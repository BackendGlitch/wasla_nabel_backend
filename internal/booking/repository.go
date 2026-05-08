package booking

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"station-backend/internal/pricing"
)

type Repository interface {
	CreateBookingByDestination(ctx context.Context, req CreateBookingByDestinationRequest) (*Booking, error)
	CreateBookingByQueueEntry(ctx context.Context, req CreateBookingByQueueEntryRequest) (*CreateBookingByQueueEntryResponse, error)
	CancelBooking(ctx context.Context, bookingID string, staffID string, reason *string) (*Booking, error)
	CancelLastBookingForStaff(ctx context.Context, staffID string) (*Booking, error)
	ListQueueSnapshot(ctx context.Context, destinationID string) ([]QueueEntry, error)
	GetDestinationByQueueEntry(ctx context.Context, queueEntryID string) (string, error)
	HasTripForQueue(ctx context.Context, queueID string) (bool, error)
	ListTrips(ctx context.Context, limit int) ([]Trip, error)
	CancelOneBookingByQueueEntry(ctx context.Context, queueEntryID string, staffID string) (*Booking, error)
	ListTodayTrips(ctx context.Context, search string, limit int) ([]Trip, error)
	GetTodayTripsCount(ctx context.Context, destinationID *string) (int, error)
	// GetTodayBookedTicketsByDestination returns regular + ghost booked ticket totals for today,
	// grouped by destination. If destinationID is provided, filters to one destination.
	GetTodayBookedTicketsByDestination(ctx context.Context, destinationID *string) ([]TodayBookedTicketsByDestination, error)
	// Ghost booking methods
	CreateGhostBooking(ctx context.Context, req CreateGhostBookingRequest) ([]*GhostBooking, error)
	GetGhostBookingCount(ctx context.Context, destinationID string) (int, error)
	// Trip count by license plate
	GetTodayTripsCountByLicensePlate(ctx context.Context, licensePlate string) (int, error)

	// GetServiceFeeTNDByDestination returns routes.service_fee for station_id (destination); default if missing.
	GetServiceFeeTNDByDestination(ctx context.Context, destinationID string) (float64, error)
}

type RepositoryImpl struct {
	db *pgxpool.Pool
}

const verificationAttempts = 32

// bookingDestinationTxAdvLockK1 is the first key for pg_advisory_xact_lock(int, int) so concurrent
// seat bookings for the same destination serialize without deadlocking on vehicle_queue updates.
const bookingDestinationTxAdvLockK1 = 918237401

// ErrCancelLastNoBooking means there is no eligible ACTIVE queued booking for the operator.
var ErrCancelLastNoBooking = errors.New("no active queued booking found for this operator")

// ErrCancelLastQueueMissing means vehicle_queue row is gone and trips offer no usable exit audit within policy.
var ErrCancelLastQueueMissing = errors.New("cannot cancel: vehicle_queue row missing; no qualifying exit-trip audit for restore")

// ErrCancelLastExitWindowExpired rejects undo after vehicle exited queue (time limit exceeded).
var ErrCancelLastExitWindowExpired = errors.New("cannot cancel: more than 5 minutes since vehicle exited queue; refusing restore")

// ErrCancelLastIncompleteAudit means we refuse to guess queue or vehicle linkage (consistency-first).
var ErrCancelLastIncompleteAudit = errors.New("cannot cancel: missing vehicle linkage for exited queue restore")

const cancelLastExitRestoreWindow = 5 * time.Minute

func nullableText(v string) interface{} {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// normalizeLicensePlateKey collapses casing and spaces so the same physical plate matches across queue re-entries (station 1 vs 2).
func normalizeLicensePlateKey(lp string) string {
	s := strings.TrimSpace(strings.ToUpper(lp))
	return strings.ReplaceAll(s, " ", "")
}

func tunisiaLocationBooking() *time.Location {
	loc, err := time.LoadLocation("Africa/Tunis")
	if err != nil {
		return time.UTC
	}
	return loc
}

// naiveTSTunisFromPg rebuilds TIMESTAMP WITHOUT TIME ZONE wall clock in Africa/Tunis (session TZ matches database/postgres.go).
// pgx unmarshals naive timestamps using UTC clocks; converting with Time.In(Africa/Tunis) shifts wall time (~+1h for Tunisia vs UTC-labelled scan).
func naiveTSTunisFromPg(t time.Time) time.Time {
	if t.IsZero() {
		return t
	}
	return time.Date(
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		tunisiaLocationBooking(),
	)
}

// firstTripOfDayReplayTx uses license plate, not queue/vehicle_queue: after a vehicle leaves, its queue row is often deleted
// and JOIN vehicle_queue would drop historical trips — same LP at another station must still count as "already had a trip today".
func firstTripOfDayReplayTx(ctx context.Context, tx pgx.Tx, licensePlate string, refTime time.Time) (bool, error) {
	key := normalizeLicensePlateKey(licensePlate)
	if key == "" {
		return false, nil
	}
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM trips t
		WHERE REPLACE(UPPER(TRIM(COALESCE(t.license_plate, ''))), ' ', '') = $1
		  AND CAST(t.start_time AS date) = CAST($2::timestamptz AS date)
		  AND t.start_time <= $2
	`, key, refTime).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// vehicleCompletedTripsTodayTx counts today's trips for this plate (survives queue row deletion).
func vehicleCompletedTripsTodayTx(ctx context.Context, tx pgx.Tx, licensePlate string) (int, error) {
	key := normalizeLicensePlateKey(licensePlate)
	if key == "" {
		return 0, nil
	}
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM trips t
		WHERE REPLACE(UPPER(TRIM(COALESCE(t.license_plate, ''))), ' ', '') = $1
		  AND CAST(t.start_time AS date) = CURRENT_DATE
	`, key).Scan(&n)
	return n, err
}

func (r *RepositoryImpl) getBookingByIdempotencyTx(ctx context.Context, tx pgx.Tx, key string) (*Booking, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("empty idempotency key")
	}

	var b Booking
	// Keep it minimal: just enough for UI to treat it as the same booking.
	err := tx.QueryRow(ctx, `
		SELECT
			b.id,
			COALESCE(b.queue_id, ''),
			COALESCE(q.vehicle_id, ''),
			COALESCE(v.license_plate, ''),
			b.seats_booked,
			COALESCE(b.seat_number, 0),
			b.total_amount,
			b.booking_status,
			b.payment_status,
			b.verification_code,
			COALESCE(b.created_by, ''),
			COALESCE(CONCAT(s.first_name, ' ', s.last_name), '') AS created_by_name,
			b.created_at
		FROM bookings b
		LEFT JOIN vehicle_queue q ON q.id = b.queue_id
		LEFT JOIN vehicles v ON v.id = q.vehicle_id
		LEFT JOIN staff s ON s.id = b.created_by
		WHERE b.idempotency_key = $1
		LIMIT 1
	`, key).Scan(
		&b.ID,
		&b.QueueID,
		&b.VehicleID,
		&b.LicensePlate,
		&b.SeatsBooked,
		&b.SeatNumber,
		&b.TotalAmount,
		&b.BookingStatus,
		&b.PaymentStatus,
		&b.VerificationCode,
		&b.CreatedBy,
		&b.CreatedByName,
		&b.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	b.CreatedAt = naiveTSTunisFromPg(b.CreatedAt)
	ft, err := firstTripOfDayReplayTx(ctx, tx, b.LicensePlate, b.CreatedAt)
	if err != nil {
		return nil, err
	}
	b.FirstTripOfDay = ft
	return &b, nil
}

func (r *RepositoryImpl) getBookingsByIdempotencyTx(ctx context.Context, tx pgx.Tx, key string) (*CreateBookingByQueueEntryResponse, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, fmt.Errorf("empty idempotency key")
	}

	rows, err := tx.Query(ctx, `
		SELECT
			b.id,
			COALESCE(b.queue_id, ''),
			COALESCE(q.vehicle_id, ''),
			COALESCE(v.license_plate, ''),
			b.seats_booked,
			COALESCE(b.seat_number, 0),
			b.total_amount,
			b.booking_status,
			b.payment_status,
			b.verification_code,
			COALESCE(b.created_by, ''),
			COALESCE(CONCAT(s.first_name, ' ', s.last_name), '') AS created_by_name,
			b.created_at
		FROM bookings b
		LEFT JOIN vehicle_queue q ON q.id = b.queue_id
		LEFT JOIN vehicles v ON v.id = q.vehicle_id
		LEFT JOIN staff s ON s.id = b.created_by
		WHERE b.idempotency_key = $1
		ORDER BY b.created_at ASC
	`, key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var bookings []Booking
	for rows.Next() {
		var b Booking
		if err := rows.Scan(
			&b.ID,
			&b.QueueID,
			&b.VehicleID,
			&b.LicensePlate,
			&b.SeatsBooked,
			&b.SeatNumber,
			&b.TotalAmount,
			&b.BookingStatus,
			&b.PaymentStatus,
			&b.VerificationCode,
			&b.CreatedBy,
			&b.CreatedByName,
			&b.CreatedAt,
		); err != nil {
			return nil, err
		}
		b.CreatedAt = naiveTSTunisFromPg(b.CreatedAt)
		bookings = append(bookings, b)
	}
	if len(bookings) == 0 {
		return nil, pgx.ErrNoRows
	}

	minT := bookings[0].CreatedAt
	for _, b := range bookings {
		if b.CreatedAt.Before(minT) {
			minT = b.CreatedAt
		}
	}
	ft, err := firstTripOfDayReplayTx(ctx, tx, bookings[0].LicensePlate, minT)
	if err != nil {
		return nil, err
	}
	for i := range bookings {
		bookings[i].FirstTripOfDay = ft
	}

	return &CreateBookingByQueueEntryResponse{
		Bookings:    bookings,
		ExitPass:    nil,
		HasExitPass: false,
	}, nil
}

// resolveGhostDestination normalizes the incoming destination identifier into a concrete active route station_id.
// It accepts direct station_id, exact station_name, and resolves the grouped "moknin-tboulba" alias.
func (r *RepositoryImpl) resolveGhostDestination(ctx context.Context, tx pgx.Tx, requestedDestinationID, staffID string) (string, string, float64, float64, error) {
	normalized := strings.ToLower(strings.TrimSpace(requestedDestinationID))
	normalized = strings.ReplaceAll(normalized, "_", "-")

	candidateIDs := []string{strings.TrimSpace(requestedDestinationID)}
	switch normalized {
	case "moknin-tboulba", "tboulba-moknin", "moknin-teboulba", "teboulba-moknin":
		// Prefer the destination most recently used by this staff member, then fail closed if still ambiguous.
		// This avoids silently assigning both counters to the same route when UI sends grouped keys.
		var recentID string
		err := tx.QueryRow(ctx, `
			SELECT destination_id
			FROM bookings
			WHERE created_by = $1
			  AND destination_id IN ('station-moknin', 'station-teboulba')
			ORDER BY created_at DESC
			LIMIT 1`, staffID).Scan(&recentID)
		if err == nil {
			candidateIDs = []string{recentID}
		} else if err != pgx.ErrNoRows {
			return "", "", 0, 0, err
		} else {
			return "", "", 0, 0, fmt.Errorf("ambiguous destination %q; send station-moknin or station-teboulba", requestedDestinationID)
		}
	default:
		// Also allow exact station_name payloads (e.g. "MOKNIN", "TEBOULBA") from legacy clients.
		candidateIDs = append(candidateIDs, strings.ToUpper(strings.TrimSpace(requestedDestinationID)))
	}

	for _, candidate := range candidateIDs {
		if candidate == "" {
			continue
		}
		var destinationID string
		var destinationName string
		var basePrice float64
		var serviceFee float64

		err := tx.QueryRow(ctx, `
			SELECT station_id, station_name, base_price, COALESCE(service_fee, $2)
			FROM routes
			WHERE is_active = true
			  AND (station_id = $1 OR UPPER(station_name) = UPPER($1))
			LIMIT 1`, candidate, pricing.ServiceFeePerSeatTND).Scan(&destinationID, &destinationName, &basePrice, &serviceFee)
		if err == nil {
			return destinationID, destinationName, basePrice, serviceFee, nil
		}
		if err != pgx.ErrNoRows {
			return "", "", 0, 0, err
		}
	}

	return "", "", 0, 0, fmt.Errorf("destination not found: %s", requestedDestinationID)
}

func NewRepository(db *pgxpool.Pool) Repository { return &RepositoryImpl{db: db} }

// CreateBookingByDestination allocates seats on a single vehicle.
// When PreferExactFit=true, it first searches an exact-seat match, then falls back to first-eligible.
// When PreferExactFit=false, it directly picks the first-eligible candidate.
// Eligible ordering: garage-unblocked rows with seats, then stable tie-break —
// serving row / resume-after-unblocked / queue_position — then positions are compacted (blocked slots keep indices).
func (r *RepositoryImpl) CreateBookingByDestination(ctx context.Context, req CreateBookingByDestinationRequest) (*Booking, error) {
	if req.Seats <= 0 {
		return nil, fmt.Errorf("seats must be > 0")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2::text)::int)`, bookingDestinationTxAdvLockK1, req.DestinationID); err != nil {
		return nil, err
	}

	// Idempotency: if a booking was already created for this key, return it.
	if strings.TrimSpace(req.IdempotencyKey) != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, req.IdempotencyKey); err != nil {
			return nil, err
		}
		if existing, getErr := r.getBookingByIdempotencyTx(ctx, tx, req.IdempotencyKey); getErr == nil && existing != nil {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return existing, nil
		}
	}

	// Try exact-fit only when requested, then fall back to first-eligible.
	var row pgx.Row
	var queueID, vehicleID string
	var pricePerSeat float64
	var serviceFeePerSeat float64
	if req.PreferExactFit {
		if req.SubRoute != nil && *req.SubRoute != "" {
			row = tx.QueryRow(ctx, `
                WITH candidate AS (
                    SELECT q.id, q.destination_id
                    FROM vehicle_queue q
                    LEFT JOIN queue_destination_booking_state _qbs ON _qbs.destination_id = q.destination_id
                    WHERE q.destination_id=$1 AND q.queue_type='REGULAR' AND q.status IN ('WAITING','LOADING','READY')
                      AND NOT COALESCE(q.is_garage_blocked, false)
                      AND q.sub_route=$3 AND q.available_seats = $2
                    ORDER BY
                      CASE WHEN q.id IS NOT DISTINCT FROM _qbs.serving_queue_entry_id THEN 0 ELSE 1 END,
                      CASE WHEN q.prioritize_after_blocked_unblock THEN 0 ELSE 1 END,
                      q.queue_position ASC
                    LIMIT 1
                    FOR UPDATE OF q
                )
                UPDATE vehicle_queue q
                SET available_seats = q.available_seats - $2
                FROM candidate c
                LEFT JOIN routes r ON r.station_id = c.destination_id
                WHERE q.id = c.id
                RETURNING q.id, q.vehicle_id, COALESCE(r.base_price, q.base_price), COALESCE(r.service_fee, $4)`, req.DestinationID, req.Seats, *req.SubRoute, pricing.ServiceFeePerSeatTND)
		} else {
			row = tx.QueryRow(ctx, `
                WITH candidate AS (
                    SELECT q.id, q.destination_id
                    FROM vehicle_queue q
                    LEFT JOIN queue_destination_booking_state _qbs ON _qbs.destination_id = q.destination_id
                    WHERE q.destination_id=$1 AND q.queue_type='REGULAR' AND q.status IN ('WAITING','LOADING','READY')
                      AND NOT COALESCE(q.is_garage_blocked, false)
                      AND q.available_seats = $2
                    ORDER BY
                      CASE WHEN q.id IS NOT DISTINCT FROM _qbs.serving_queue_entry_id THEN 0 ELSE 1 END,
                      CASE WHEN q.prioritize_after_blocked_unblock THEN 0 ELSE 1 END,
                      q.queue_position ASC
                    LIMIT 1
                    FOR UPDATE OF q
                )
                UPDATE vehicle_queue q
                SET available_seats = q.available_seats - $2
                FROM candidate c
                LEFT JOIN routes r ON r.station_id = c.destination_id
                WHERE q.id = c.id
                RETURNING q.id, q.vehicle_id, COALESCE(r.base_price, q.base_price), COALESCE(r.service_fee, $3)`, req.DestinationID, req.Seats, pricing.ServiceFeePerSeatTND)
		}
		var tmpQ, tmpV string
		var tmpP float64
		var tmpF float64
		if err := row.Scan(&tmpQ, &tmpV, &tmpP, &tmpF); err != nil {
			if err != pgx.ErrNoRows {
				return nil, err
			}
		} else {
			// exact fit success; set for downstream use
			queueID, vehicleID, pricePerSeat, serviceFeePerSeat = tmpQ, tmpV, tmpP, tmpF
		}
	}

	// If exact-fit wasn't used or didn't find a candidate, fall back to first-eligible.
	if queueID == "" {
		if req.SubRoute != nil && *req.SubRoute != "" {
			row = tx.QueryRow(ctx, `
                WITH candidate AS (
                    SELECT q.id, q.destination_id
                    FROM vehicle_queue q
                    LEFT JOIN queue_destination_booking_state _qbs ON _qbs.destination_id = q.destination_id
                    WHERE q.destination_id=$1 AND q.queue_type='REGULAR' AND q.status IN ('WAITING','LOADING','READY')
                      AND NOT COALESCE(q.is_garage_blocked, false)
                      AND q.sub_route=$3 AND q.available_seats >= $2
                    ORDER BY
                      CASE WHEN q.id IS NOT DISTINCT FROM _qbs.serving_queue_entry_id THEN 0 ELSE 1 END,
                      CASE WHEN q.prioritize_after_blocked_unblock THEN 0 ELSE 1 END,
                      q.queue_position ASC
                    LIMIT 1
                    FOR UPDATE OF q
                )
                UPDATE vehicle_queue q
                SET available_seats = q.available_seats - $2
                FROM candidate c
                LEFT JOIN routes r ON r.station_id = c.destination_id
                WHERE q.id = c.id
                RETURNING q.id, q.vehicle_id, COALESCE(r.base_price, q.base_price), COALESCE(r.service_fee, $4)`, req.DestinationID, req.Seats, *req.SubRoute, pricing.ServiceFeePerSeatTND)
		} else {
			row = tx.QueryRow(ctx, `
                WITH candidate AS (
                    SELECT q.id, q.destination_id
                    FROM vehicle_queue q
                    LEFT JOIN queue_destination_booking_state _qbs ON _qbs.destination_id = q.destination_id
                    WHERE q.destination_id=$1 AND q.queue_type='REGULAR' AND q.status IN ('WAITING','LOADING','READY')
                      AND NOT COALESCE(q.is_garage_blocked, false)
                      AND q.available_seats >= $2
                    ORDER BY
                      CASE WHEN q.id IS NOT DISTINCT FROM _qbs.serving_queue_entry_id THEN 0 ELSE 1 END,
                      CASE WHEN q.prioritize_after_blocked_unblock THEN 0 ELSE 1 END,
                      q.queue_position ASC
                    LIMIT 1
                    FOR UPDATE OF q
                )
                UPDATE vehicle_queue q
                SET available_seats = q.available_seats - $2
                FROM candidate c
                LEFT JOIN routes r ON r.station_id = c.destination_id
                WHERE q.id = c.id
                RETURNING q.id, q.vehicle_id, COALESCE(r.base_price, q.base_price), COALESCE(r.service_fee, $3)`, req.DestinationID, req.Seats, pricing.ServiceFeePerSeatTND)
		}

		if err := row.Scan(&queueID, &vehicleID, &pricePerSeat, &serviceFeePerSeat); err != nil {
			if err == pgx.ErrNoRows {
				return nil, fmt.Errorf("no vehicle with enough seats available for this destination")
			}
			return nil, err
		}
	}

	// Update vehicle status based on seats after deduction
	if _, err := tx.Exec(ctx, `
        UPDATE vehicle_queue SET status = CASE
            WHEN available_seats = 0 THEN 'READY'
            WHEN available_seats < total_seats THEN 'LOADING'
            ELSE 'WAITING'
        END WHERE id = $1`, queueID); err != nil {
		return nil, err
	}

	// Fetch license plate for response (not strictly necessary for booking creation)
	var licensePlate string
	if err := tx.QueryRow(ctx, `SELECT license_plate FROM vehicles WHERE id=$1`, vehicleID).Scan(&licensePlate); err != nil {
		return nil, err
	}

	tripsToday, err := vehicleCompletedTripsTodayTx(ctx, tx, licensePlate)
	if err != nil {
		return nil, err
	}
	firstTripOfDay := tripsToday == 0

	var isReady bool
	var destID, destName string
	var totalSeats, availableSeats int
	if err := tx.QueryRow(ctx, `
        SELECT (available_seats = 0) AS ready, destination_id, destination_name, total_seats, available_seats
        FROM vehicle_queue WHERE id = $1`, queueID).Scan(&isReady, &destID, &destName, &totalSeats, &availableSeats); err != nil {
		return nil, err
	}

	var b Booking
	b.VehicleID = vehicleID
	b.LicensePlate = licensePlate
	b.SeatsBooked = req.Seats
	// Seat index is per-car and derived from occupancy (booked vs free seats).
	// availableSeats here is after this booking deduction.
	seatStart := (totalSeats - availableSeats) - req.Seats + 1
	if seatStart < 1 {
		seatStart = 1
	}
	b.SeatNumber = seatStart
	seatPrice := pricePerSeat + serviceFeePerSeat // base + per-destination service fee
	b.TotalAmount = float64(req.Seats) * seatPrice
	b.BookingStatus = "ACTIVE"
	b.PaymentStatus = "PAID"
	b.CreatedBy = req.StaffID
	b.QueueID = queueID

	// Create booking (local schema). verification_code is unique, so we must handle rare collisions safely.
	// Use ON CONFLICT + retry (like ghost bookings) to avoid transaction aborts.
	for attempt := 0; attempt < verificationAttempts; attempt++ {
		row = tx.QueryRow(ctx, `
			INSERT INTO bookings (
				id, queue_id, destination_id, seats_booked, seat_number, total_amount, booking_source, booking_type,
				booking_status, payment_status, payment_method, verification_code,
				is_verified, created_by, idempotency_key
			) VALUES (
				substr(md5(random()::text || clock_timestamp()::text),1,24),
				$1, $7, $2, $3, $4, 'CASH_STATION', 'CASH', 'ACTIVE', 'PAID', 'CASH',
				LPAD(CAST(FLOOR(random()*1000000) AS TEXT), 6, '0'), false, $5, $6
			)
			ON CONFLICT (verification_code) DO NOTHING
			RETURNING id, verification_code, created_at`, queueID, req.Seats, b.SeatNumber, float64(req.Seats)*seatPrice, req.StaffID, nullableText(req.IdempotencyKey), req.DestinationID)

		if err := row.Scan(&b.ID, &b.VerificationCode, &b.CreatedAt); err != nil {
			if err == pgx.ErrNoRows {
				continue
			}
			return nil, err
		}
		b.CreatedAt = naiveTSTunisFromPg(b.CreatedAt)
		break
	}

	if b.ID == "" {
		return nil, fmt.Errorf("failed to generate unique verification code after %d attempts", verificationAttempts)
	}

	b.FirstTripOfDay = firstTripOfDay

	// Trip row after booking so talon replay (firstTripOfDay) matches created_at ordering.
	if isReady {
		seatsForTrip := totalSeats
		if _, err := tx.Exec(ctx, `
            INSERT INTO trips (
                id, queue_id, destination_id, destination_name, license_plate,
                start_time, created_at, total_seats, booked_seats, created_by
            ) VALUES (
                substr(md5(random()::text || clock_timestamp()::text),1,24),
                $1, $2, $3, $4, NOW(), NOW(), $5, $6, $7
            )`, queueID, destID, destName, licensePlate, totalSeats, seatsForTrip, req.StaffID); err != nil {
			return nil, err
		}
	}

	if err := r.applyBookingQueueServingAndCompactTx(ctx, tx, destID, queueID, availableSeats); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &b, nil
}

func (r *RepositoryImpl) CancelBooking(ctx context.Context, bookingID string, staffID string, reason *string) (*Booking, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var vehicleID string
	var seats int
	var status string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(queue_id,''), seats_booked, booking_status FROM bookings WHERE id=$1 FOR UPDATE`, bookingID).Scan(&vehicleID, &seats, &status); err != nil {
		return nil, err
	}
	if status != "ACTIVE" {
		return nil, fmt.Errorf("booking already %s", status)
	}

	// Restore seats to that queue entry
	if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET available_seats = LEAST(total_seats, available_seats + $2) WHERE id=$1`, vehicleID, seats); err != nil {
		return nil, err
	}

	// Update status after cancellation
	if _, err := tx.Exec(ctx, `
        UPDATE vehicle_queue SET status = CASE
            WHEN available_seats = 0 THEN 'READY'
            WHEN available_seats < total_seats THEN 'LOADING'
            ELSE 'WAITING'
        END WHERE id = $1`, vehicleID); err != nil {
		return nil, err
	}

	// If the queue entry is no longer READY after restoration, remove any existing trip tied to this queue entry
	var nowReady bool
	if err := tx.QueryRow(ctx, `SELECT (available_seats = 0) FROM vehicle_queue WHERE id=$1`, vehicleID).Scan(&nowReady); err == nil {
		if !nowReady {
			if _, derr := tx.Exec(ctx, `DELETE FROM trips WHERE queue_id = $1`, vehicleID); derr != nil {
				return nil, derr
			}
			var exitPassesTable bool
			if err := tx.QueryRow(ctx, `SELECT to_regclass('public.exit_passes') IS NOT NULL`).Scan(&exitPassesTable); err != nil {
				return nil, err
			}
			if exitPassesTable {
				if _, err := tx.Exec(ctx, `DELETE FROM exit_passes WHERE queue_id = $1`, vehicleID); err != nil {
					return nil, err
				}
			}
		}
	}

	// Mark booking cancelled
	ct, err := tx.Exec(ctx, `UPDATE bookings SET booking_status='CANCELLED', cancelled_at=NOW(), cancelled_by=$2, cancellation_reason=$3 WHERE id=$1`, bookingID, staffID, reason)
	if err != nil {
		return nil, err
	}
	if ct.RowsAffected() == 0 {
		return nil, fmt.Errorf("booking not updated")
	}

	var b Booking
	if err := tx.QueryRow(ctx, `SELECT id, seats_booked, total_amount, booking_status, payment_status, verification_code, created_by, created_at FROM bookings WHERE id=$1`, bookingID).Scan(
		&b.ID, &b.SeatsBooked, &b.TotalAmount, &b.BookingStatus, &b.PaymentStatus, &b.VerificationCode, &b.CreatedBy, &b.CreatedAt,
	); err != nil {
		return nil, err
	}
	b.CreatedAt = naiveTSTunisFromPg(b.CreatedAt)
	b.QueueID = vehicleID

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &b, nil
}

// CancelOneBookingByQueueEntry finds the most recent ACTIVE booking for a queue entry and cancels it
func (r *RepositoryImpl) CancelOneBookingByQueueEntry(ctx context.Context, queueEntryID string, staffID string) (*Booking, error) {
	// Safety guard: do not allow cancel after the vehicle/entry has left the queue.
	var exists bool
	if err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1
			FROM vehicle_queue
			WHERE id = $1
			  AND status IN ('WAITING','LOADING','READY')
		)
	`, queueEntryID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("cannot cancel booking: vehicle already left queue")
	}

	var bookingID string
	if err := r.db.QueryRow(ctx, `SELECT id FROM bookings WHERE queue_id=$1 AND booking_status='ACTIVE' ORDER BY created_at DESC LIMIT 1`, queueEntryID).Scan(&bookingID); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("no active bookings to cancel for this queue entry")
		}
		return nil, err
	}
	return r.CancelBooking(ctx, bookingID, staffID, nil)
}

// CancelLastBookingForStaff resolves the operator's latest regular (queued) seat booking.
//
// Constraints (consistency-first):
//   - is_ghost_booking rows are skipped (separate undo flow later).
//   - If vehicle_queue exists: delegates to CancelBooking (seat restore + trip/exit-pass cleanup).
//   - If vehicle_queue row is missing: only when a trips row exists for queue_id (exit audit).
//     Reinsert uses trip fields + restores available_seats from remaining ACTIVE bookings (after cancelling this row).
//     Queue position is append-only (destination MAX+1), not reconstructed — consistency over original slot.
//     Exit must be within cancelLastExitRestoreWindow since trip.start_time; otherwise ErrCancelLastExitWindowExpired (no DB mutations).
//
// Revenue / analytics are not reversed here by design.
func (r *RepositoryImpl) CancelLastBookingForStaff(ctx context.Context, staffID string) (*Booking, error) {
	if strings.TrimSpace(staffID) == "" {
		return nil, fmt.Errorf("staff id required")
	}

	var bookingID, queueID string
	err := r.db.QueryRow(ctx, `
		SELECT id, TRIM(queue_id)
		FROM bookings
		WHERE created_by = $1
		  AND booking_status = 'ACTIVE'
		  AND COALESCE(is_ghost_booking, false) = false
		  AND queue_id IS NOT NULL
		  AND TRIM(queue_id) <> ''
		ORDER BY created_at DESC
		LIMIT 1
	`, staffID).Scan(&bookingID, &queueID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCancelLastNoBooking
		}
		return nil, err
	}

	var queueExists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM vehicle_queue WHERE id = $1)`, queueID).Scan(&queueExists); err != nil {
		return nil, err
	}
	if queueExists {
		reason := "cancel_last_operator"
		return r.CancelBooking(ctx, bookingID, staffID, &reason)
	}

	return r.cancelLastBookingAfterQueueRemoved(ctx, staffID, bookingID, queueID)
}

// cancelLastBookingAfterQueueRemoved reinserts vehicle_queue only when trips prove a recent exit.
func (r *RepositoryImpl) cancelLastBookingAfterQueueRemoved(ctx context.Context, staffID string, bookingID, queueID string) (*Booking, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var scanID string
	var status string
	var createdBy string
	var ghost bool
	var qref string
	if err := tx.QueryRow(ctx, `
		SELECT id, booking_status, created_by,
		       COALESCE(is_ghost_booking, false), TRIM(queue_id)
		FROM bookings WHERE id=$1 FOR UPDATE`,
		bookingID).Scan(&scanID, &status, &createdBy, &ghost, &qref); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCancelLastNoBooking
		}
		return nil, err
	}
	if status != "ACTIVE" {
		return nil, fmt.Errorf("booking already %s", status)
	}
	if ghost {
		return nil, ErrCancelLastNoBooking
	}
	if createdBy != staffID {
		return nil, ErrCancelLastNoBooking
	}
	if qref != queueID {
		return nil, ErrCancelLastNoBooking
	}

	var destID, destName, licensePlate string
	var startTime time.Time
	var totalSeats int
	var tripVehicleID sql.NullString

	err = tx.QueryRow(ctx, `
		SELECT destination_id, destination_name, license_plate, start_time, total_seats, vehicle_id
		FROM trips
		WHERE queue_id = $1
		ORDER BY start_time DESC, created_at DESC
		LIMIT 1`,
		queueID).Scan(&destID, &destName, &licensePlate, &startTime, &totalSeats, &tripVehicleID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrCancelLastQueueMissing
		}
		return nil, err
	}
	startTime = naiveTSTunisFromPg(startTime)

	exitAt := startTime.UTC()
	if time.Now().UTC().Sub(exitAt) > cancelLastExitRestoreWindow {
		return nil, ErrCancelLastExitWindowExpired
	}

	vehicleIDResolved := ""
	if tripVehicleID.Valid && strings.TrimSpace(tripVehicleID.String) != "" {
		vehicleIDResolved = strings.TrimSpace(tripVehicleID.String)
	}
	if vehicleIDResolved == "" {
		key := normalizeLicensePlateKey(licensePlate)
		if key == "" {
			return nil, ErrCancelLastIncompleteAudit
		}
		if err := tx.QueryRow(ctx, `
			SELECT id::text FROM vehicles
			WHERE REPLACE(UPPER(TRIM(COALESCE(license_plate, ''))), ' ', '') = $1
			LIMIT 1`, key).Scan(&vehicleIDResolved); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrCancelLastIncompleteAudit
			}
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2::text)::int)`, bookingDestinationTxAdvLockK1, destID); err != nil {
		return nil, err
	}

	var nextPos int
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(queue_position), 0) + 1
		FROM vehicle_queue WHERE destination_id = $1`, destID).Scan(&nextPos); err != nil {
		return nil, err
	}

	var basePrice float64
	qErr := tx.QueryRow(ctx, `SELECT COALESCE(r.base_price, 0) FROM routes r WHERE r.station_id = $1 LIMIT 1`, destID).Scan(&basePrice)
	if qErr != nil && errors.Is(qErr, pgx.ErrNoRows) {
		basePrice = 15
		qErr = nil
	}
	if qErr != nil {
		return nil, qErr
	}
	if basePrice <= 0 {
		basePrice = 15
	}

	reason := "cancel_last_operator"
	if _, err := tx.Exec(ctx, `
		UPDATE bookings SET booking_status='CANCELLED', cancelled_at=NOW(), cancelled_by=$2, cancellation_reason=$3
		WHERE id=$1 AND booking_status='ACTIVE'`, bookingID, staffID, reason); err != nil {
		return nil, err
	}

	var remainingActiveSeatSum int64
	if err := tx.QueryRow(ctx, `
		SELECT COALESCE(SUM(seats_booked), 0)::bigint
		FROM bookings
		WHERE queue_id = $1 AND booking_status = 'ACTIVE'`,
		queueID).Scan(&remainingActiveSeatSum); err != nil {
		return nil, err
	}

	availInserted := totalSeats - int(remainingActiveSeatSum)
	if availInserted < 0 {
		availInserted = 0
	}
	if availInserted > totalSeats {
		availInserted = totalSeats
	}

	var queueStatus string
	switch {
	case availInserted == 0:
		queueStatus = "READY"
	case availInserted < totalSeats:
		queueStatus = "LOADING"
	default:
		queueStatus = "WAITING"
	}

	if _, err := tx.Exec(ctx, `DELETE FROM trips WHERE queue_id = $1`, queueID); err != nil {
		return nil, err
	}

	var exitPassesTable bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.exit_passes') IS NOT NULL`).Scan(&exitPassesTable); err != nil {
		return nil, err
	}
	if exitPassesTable {
		if _, err := tx.Exec(ctx, `DELETE FROM exit_passes WHERE queue_id = $1`, queueID); err != nil {
			return nil, err
		}
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO vehicle_queue (
			id, vehicle_id, license_plate, destination_id, destination_name,
			sub_route, sub_route_name, queue_type, queue_position, status,
			entered_at, available_seats, total_seats, base_price
		) VALUES (
			$1, $2, $3, $4, $5, NULL, NULL, 'REGULAR', $6, $7, NOW(),
			$8, $9, $10
		)`,
		queueID, vehicleIDResolved, licensePlate, destID, destName, nextPos,
		queueStatus, availInserted, totalSeats, basePrice); err != nil {
		return nil, err
	}

	var b Booking
	if err := tx.QueryRow(ctx, `
		SELECT id, seats_booked, total_amount, booking_status, payment_status, verification_code, created_by, created_at
		FROM bookings WHERE id=$1`, bookingID).Scan(
		&b.ID, &b.SeatsBooked, &b.TotalAmount, &b.BookingStatus, &b.PaymentStatus, &b.VerificationCode, &b.CreatedBy, &b.CreatedAt,
	); err != nil {
		return nil, err
	}
	b.CreatedAt = naiveTSTunisFromPg(b.CreatedAt)
	b.QueueID = queueID

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &b, nil
}

// ListQueueSnapshot returns the current queue for a destination (minimal columns for UI refresh)
func (r *RepositoryImpl) ListQueueSnapshot(ctx context.Context, destinationID string) ([]QueueEntry, error) {
	rows, err := r.db.Query(ctx, `
        SELECT q.id, q.vehicle_id, COALESCE(NULLIF(v.license_plate, ''), NULLIF(q.license_plate, ''), '[UNKNOWN]'), q.destination_id, q.destination_name,
               q.sub_route, q.sub_route_name, q.queue_type, q.queue_position, q.status,
               q.entered_at, q.available_seats, q.total_seats, q.base_price,
               q.estimated_departure, q.actual_departure,
               COALESCE(q.is_garage_blocked, false)
        FROM vehicle_queue q
        LEFT JOIN vehicles v ON v.id = q.vehicle_id
        WHERE q.destination_id = $1
        ORDER BY q.queue_position ASC`, destinationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QueueEntry
	for rows.Next() {
		var e QueueEntry
		if err := rows.Scan(&e.ID, &e.VehicleID, &e.LicensePlate, &e.DestinationID, &e.DestinationName,
			&e.SubRoute, &e.SubRouteName, &e.QueueType, &e.QueuePosition, &e.Status,
			&e.EnteredAt, &e.AvailableSeats, &e.TotalSeats, &e.BasePrice, &e.EstimatedDeparture, &e.ActualDeparture,
			&e.IsGarageBlocked); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, nil
}

// HasTripForQueue returns whether a trip exists for the given queue entry
func (r *RepositoryImpl) HasTripForQueue(ctx context.Context, queueID string) (bool, error) {
	var exists bool
	if err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM trips WHERE queue_id = $1)`, queueID).Scan(&exists); err != nil {
		return false, err
	}
	return exists, nil
}

func (r *RepositoryImpl) ListTrips(ctx context.Context, limit int) ([]Trip, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
        SELECT
            t.id,
            COALESCE(to_jsonb(t)->>'vehicle_id', '') AS vehicle_id,
            COALESCE(NULLIF(t.license_plate, ''), '[UNKNOWN]') AS license_plate,
            t.destination_id,
            t.destination_name,
            t.queue_id,
            COALESCE(
              NULLIF(to_jsonb(t)->>'seats_booked', '')::int,
              NULLIF(to_jsonb(t)->>'booked_seats', '')::int,
              0
            ) AS seats_booked,
            NULLIF(to_jsonb(t)->>'vehicle_capacity', '')::int AS vehicle_capacity,
            NULLIF(to_jsonb(t)->>'base_price', '')::double precision AS base_price,
            t.start_time,
            t.created_at,
            (
                ROW_NUMBER() OVER (
                    PARTITION BY REPLACE(UPPER(TRIM(COALESCE(t.license_plate, ''))), ' ', ''),
                               CAST(t.start_time AS date)
                    ORDER BY t.start_time ASC NULLS LAST, t.created_at ASC NULLS LAST, t.id ASC
                ) = 1
            ) AS first_trip_of_day
        FROM trips t
        ORDER BY t.start_time DESC
        LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Trip
	for rows.Next() {
		var t Trip
		if err := rows.Scan(&t.ID, &t.VehicleID, &t.LicensePlate, &t.DestinationID, &t.DestinationName, &t.QueueID, &t.SeatsBooked, &t.VehicleCapacity, &t.BasePrice, &t.StartTime, &t.CreatedAt, &t.FirstTripOfDay); err != nil {
			return nil, err
		}
		t.StartTime = naiveTSTunisFromPg(t.StartTime)
		t.CreatedAt = naiveTSTunisFromPg(t.CreatedAt)
		list = append(list, t)
	}
	return list, nil
}

// ListTodayTrips returns trips for the current day, optionally filtered by license plate
func (r *RepositoryImpl) ListTodayTrips(ctx context.Context, search string, limit int) ([]Trip, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 2000 {
		limit = 2000
	}
	var rows pgx.Rows
	var err error
	if search != "" {
		rows, err = r.db.Query(ctx, `
            SELECT
                t.id,
                COALESCE(to_jsonb(t)->>'vehicle_id', '') AS vehicle_id,
                COALESCE(NULLIF(t.license_plate, ''), '[UNKNOWN]') AS license_plate,
                t.destination_id,
                t.destination_name,
                t.queue_id,
                COALESCE(
                  NULLIF(to_jsonb(t)->>'seats_booked', '')::int,
                  NULLIF(to_jsonb(t)->>'booked_seats', '')::int,
                  0
                ) AS seats_booked,
                NULLIF(to_jsonb(t)->>'vehicle_capacity', '')::int AS vehicle_capacity,
                NULLIF(to_jsonb(t)->>'base_price', '')::double precision AS base_price,
                t.start_time,
                t.created_at,
                (
                    ROW_NUMBER() OVER (
                        PARTITION BY REPLACE(UPPER(TRIM(COALESCE(t.license_plate, ''))), ' ', ''),
                                   CAST(t.start_time AS date)
                        ORDER BY t.start_time ASC NULLS LAST, t.created_at ASC NULLS LAST, t.id ASC
                    ) = 1
                ) AS first_trip_of_day
            FROM trips t
            WHERE t.start_time::date = CURRENT_DATE AND t.license_plate ILIKE '%' || $1 || '%'
            ORDER BY t.start_time DESC
            LIMIT $2
        `, search, limit)
	} else {
		rows, err = r.db.Query(ctx, `
            SELECT
                t.id,
                COALESCE(to_jsonb(t)->>'vehicle_id', '') AS vehicle_id,
                COALESCE(NULLIF(t.license_plate, ''), '[UNKNOWN]') AS license_plate,
                t.destination_id,
                t.destination_name,
                t.queue_id,
                COALESCE(
                  NULLIF(to_jsonb(t)->>'seats_booked', '')::int,
                  NULLIF(to_jsonb(t)->>'booked_seats', '')::int,
                  0
                ) AS seats_booked,
                NULLIF(to_jsonb(t)->>'vehicle_capacity', '')::int AS vehicle_capacity,
                NULLIF(to_jsonb(t)->>'base_price', '')::double precision AS base_price,
                t.start_time,
                t.created_at,
                (
                    ROW_NUMBER() OVER (
                        PARTITION BY REPLACE(UPPER(TRIM(COALESCE(t.license_plate, ''))), ' ', ''),
                                   CAST(t.start_time AS date)
                        ORDER BY t.start_time ASC NULLS LAST, t.created_at ASC NULLS LAST, t.id ASC
                    ) = 1
                ) AS first_trip_of_day
            FROM trips t
            WHERE t.start_time::date = CURRENT_DATE
            ORDER BY t.start_time DESC
            LIMIT $1
        `, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []Trip
	for rows.Next() {
		var t Trip
		if err := rows.Scan(&t.ID, &t.VehicleID, &t.LicensePlate, &t.DestinationID, &t.DestinationName, &t.QueueID, &t.SeatsBooked, &t.VehicleCapacity, &t.BasePrice, &t.StartTime, &t.CreatedAt, &t.FirstTripOfDay); err != nil {
			return nil, err
		}
		t.StartTime = naiveTSTunisFromPg(t.StartTime)
		t.CreatedAt = naiveTSTunisFromPg(t.CreatedAt)
		list = append(list, t)
	}
	return list, nil
}

// CreateBookingByQueueEntry creates individual bookings for each seat on a specific queue entry
func (r *RepositoryImpl) CreateBookingByQueueEntry(ctx context.Context, req CreateBookingByQueueEntryRequest) (*CreateBookingByQueueEntryResponse, error) {
	if req.Seats <= 0 {
		return nil, fmt.Errorf("seats must be > 0")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var destForAdvLock string
	if err := tx.QueryRow(ctx, `SELECT destination_id FROM vehicle_queue WHERE id = $1`, req.QueueEntryID).Scan(&destForAdvLock); err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("queue entry not found")
		}
		return nil, err
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2::text)::int)`, bookingDestinationTxAdvLockK1, destForAdvLock); err != nil {
		return nil, err
	}

	// Idempotency: if a booking batch was already created for this key, return it.
	if strings.TrimSpace(req.IdempotencyKey) != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, req.IdempotencyKey); err != nil {
			return nil, err
		}
		if existing, getErr := r.getBookingsByIdempotencyTx(ctx, tx, req.IdempotencyKey); getErr == nil && existing != nil {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return existing, nil
		}
	}

	// Get staff name for display
	var staffName string
	if req.StaffID != "" {
		err = tx.QueryRow(ctx, `SELECT CONCAT(first_name, ' ', last_name) FROM staff WHERE id = $1`, req.StaffID).Scan(&staffName)
		if err != nil {
			staffName = "Unknown Staff" // Fallback if staff not found
		}
	} else {
		staffName = "System"
	}
	var queueID, vehicleID string
	var pricePerSeat float64
	var serviceFeePerSeat float64
	var availableSeats int
	err = tx.QueryRow(ctx, `
		SELECT q.id, q.vehicle_id, COALESCE(r.base_price, q.base_price), COALESCE(r.service_fee, $2), q.available_seats
		FROM vehicle_queue q
		LEFT JOIN routes r ON r.station_id = q.destination_id
		WHERE q.id = $1 AND q.queue_type='REGULAR' AND q.status IN ('WAITING','LOADING','READY')
		  AND NOT COALESCE(q.is_garage_blocked, false)
		FOR UPDATE OF q`, req.QueueEntryID, pricing.ServiceFeePerSeatTND).Scan(&queueID, &vehicleID, &pricePerSeat, &serviceFeePerSeat, &availableSeats)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("queue entry not found, not available for booking, or blocked in garage")
		}
		return nil, err
	}

	if availableSeats < req.Seats {
		return nil, fmt.Errorf("not enough seats available (requested: %d, available: %d)", req.Seats, availableSeats)
	}

	// Deduct seats from the specific queue entry
	_, err = tx.Exec(ctx, `
		UPDATE vehicle_queue 
		SET available_seats = available_seats - $2
		WHERE id = $1`, queueID, req.Seats)
	if err != nil {
		return nil, err
	}

	// Update vehicle status based on seats after deduction
	if _, err := tx.Exec(ctx, `
		UPDATE vehicle_queue SET status = CASE
			WHEN available_seats = 0 THEN 'READY'
			WHEN available_seats < total_seats THEN 'LOADING'
			ELSE 'WAITING'
		END WHERE id = $1`, queueID); err != nil {
		return nil, err
	}

	// Fetch license plate for response
	var licensePlate string
	if err := tx.QueryRow(ctx, `SELECT license_plate FROM vehicles WHERE id=$1`, vehicleID).Scan(&licensePlate); err != nil {
		return nil, err
	}

	var destID, destName string
	var totalSeats, newAvailableSeats int
	if err := tx.QueryRow(ctx, `
		SELECT destination_id, destination_name, total_seats, available_seats
		FROM vehicle_queue WHERE id = $1`, queueID).Scan(&destID, &destName, &totalSeats, &newAvailableSeats); err != nil {
		return nil, err
	}

	// Get base price from routes table (preferred) or fallback to vehicle_queue
	var basePrice float64
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(r.base_price, q.base_price)
		FROM vehicle_queue q
		LEFT JOIN routes r ON r.station_id = q.destination_id
		WHERE q.id = $1`, queueID).Scan(&basePrice)
	if err != nil {
		basePrice = 15.0 // Default price if not found
	}

	tripsToday, err := vehicleCompletedTripsTodayTx(ctx, tx, licensePlate)
	if err != nil {
		return nil, err
	}
	firstTripOfDay := tripsToday == 0

	// Seat numbering per car is derived from occupancy (booked vs free seats),
	// not from booking row count, so it stays consistent per vehicle lifecycle.
	nextSeatNumber := (totalSeats - newAvailableSeats) - req.Seats + 1
	if nextSeatNumber < 1 {
		nextSeatNumber = 1
	}

	var exitPass *ExitPass

	// Create individual bookings for each seat
	var bookings []Booking
	seatPrice := pricePerSeat + serviceFeePerSeat // base + per-destination service fee

	for i := 0; i < req.Seats; i++ {
		var bookingID string
		var verificationCode string
		var createdAt time.Time
		currentSeatNumber := nextSeatNumber + i

		var insertErr error
		for attempt := 0; attempt < verificationAttempts; attempt++ {
			insertErr = tx.QueryRow(ctx, `
				INSERT INTO bookings (
					id, queue_id, destination_id, seats_booked, total_amount, booking_source, booking_type,
					booking_status, payment_status, payment_method, verification_code,
					is_verified, created_by, idempotency_key
				) VALUES (
					substr(md5(random()::text || clock_timestamp()::text),1,24),
					$1, $5, 1, $2, 'CASH_STATION', 'CASH', 'ACTIVE', 'PAID', 'CASH',
					LPAD(CAST(FLOOR(random()*1000000) AS TEXT), 6, '0'), false, $3, $4
				)
				ON CONFLICT (verification_code) DO NOTHING
				RETURNING id, verification_code, created_at`, queueID, seatPrice, req.StaffID, nullableText(req.IdempotencyKey), destID).Scan(&bookingID, &verificationCode, &createdAt)
			if insertErr == nil {
				break
			}
			if insertErr == pgx.ErrNoRows {
				continue
			}
			return nil, insertErr
		}
		if insertErr != nil {
			return nil, fmt.Errorf("failed to generate unique verification code after %d attempts: %w", verificationAttempts, insertErr)
		}

		bookings = append(bookings, Booking{
			ID:               bookingID,
			QueueID:          queueID,
			VehicleID:        vehicleID,
			LicensePlate:     licensePlate,
			SeatsBooked:      1,                 // Each booking is for 1 seat
			SeatNumber:       currentSeatNumber, // Individual seat number based on booking order
			TotalAmount:      seatPrice,
			BookingStatus:    "ACTIVE",
			PaymentStatus:    "PAID",
			VerificationCode: verificationCode,
			CreatedBy:        req.StaffID,
			CreatedByName:    staffName, // Staff name instead of just ID
			CreatedAt:        naiveTSTunisFromPg(createdAt),
			FirstTripOfDay:   firstTripOfDay,
		})
	}

	// Trip + exit pass after booking rows so firstTripOfDay replay stays consistent with created_at.
	if newAvailableSeats == 0 {
		tripID := fmt.Sprintf("trip_%d", time.Now().UnixNano())
		currentExitTime := time.Now().In(time.FixedZone("Africa/Tunis", 3600)) // Use Tunisia timezone

		if _, err := tx.Exec(ctx, `
			INSERT INTO trips (
				id, queue_id, destination_id, destination_name, license_plate,
				start_time, created_at, total_seats, booked_seats, created_by
			) VALUES (
				$1, $2, $3, $4, $5, NOW(), NOW(), $6, $7, $8
			)`, tripID, queueID, destID, destName, licensePlate, totalSeats, totalSeats, req.StaffID); err != nil {
			return nil, err
		}

		totalPrice := basePrice * float64(totalSeats)
		exitPass = &ExitPass{
			ID:              tripID,
			QueueID:         queueID,
			VehicleID:       vehicleID,
			LicensePlate:    licensePlate,
			DestinationID:   destID,
			DestinationName: destName,
			CurrentExitTime: currentExitTime,
			TotalPrice:      totalPrice,
			CreatedBy:       req.StaffID,
			CreatedByName:   staffName,
			CreatedAt:       time.Now(),
			VehicleCapacity: totalSeats,
			BasePrice:       basePrice,
		}
	}

	if err := r.applyBookingQueueServingAndCompactTx(ctx, tx, destID, queueID, newAvailableSeats); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	// Return response with bookings and exit pass information
	response := &CreateBookingByQueueEntryResponse{
		Bookings:    bookings,
		ExitPass:    exitPass,
		HasExitPass: exitPass != nil,
	}

	return response, nil
}

// GetDestinationByQueueEntry gets the destination ID for a queue entry
func (r *RepositoryImpl) GetDestinationByQueueEntry(ctx context.Context, queueEntryID string) (string, error) {
	var destinationID string
	err := r.db.QueryRow(ctx, `SELECT destination_id FROM vehicle_queue WHERE id = $1`, queueEntryID).Scan(&destinationID)
	if err != nil {
		return "", err
	}
	return destinationID, nil
}

// GetTodayTripsCount returns the count of trips for today, optionally filtered by destination
func (r *RepositoryImpl) GetTodayTripsCount(ctx context.Context, destinationID *string) (int, error) {
	var count int
	var query string
	var args []interface{}

	if destinationID != nil && *destinationID != "" {
		query = `SELECT COUNT(*) FROM trips WHERE start_time::date = CURRENT_DATE AND destination_id = $1`
		args = []interface{}{*destinationID}
	} else {
		query = `SELECT COUNT(*) FROM trips WHERE start_time::date = CURRENT_DATE`
		args = []interface{}{}
	}

	err := r.db.QueryRow(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetTodayBookedTicketsByDestination sums booked tickets today (regular + ghost),
// grouped by destination_id.
func (r *RepositoryImpl) GetTodayBookedTicketsByDestination(ctx context.Context, destinationID *string) ([]TodayBookedTicketsByDestination, error) {
	if destinationID != nil && strings.TrimSpace(*destinationID) == "" {
		destinationID = nil
	}

	var query string
	var args []interface{}
	if destinationID != nil {
		query = `
			SELECT
				destination_id,
				COALESCE(SUM(CASE WHEN is_ghost_booking = true THEN seats_booked ELSE 0 END), 0) AS ghost_tickets,
				COALESCE(SUM(CASE WHEN is_ghost_booking = false THEN seats_booked ELSE 0 END), 0) AS regular_tickets,
				COALESCE(SUM(seats_booked), 0) AS total_tickets
			FROM bookings
			WHERE destination_id = $1
			  AND booking_status = 'ACTIVE'
			  AND created_at::date = CURRENT_DATE
			GROUP BY destination_id
		`
		args = []interface{}{*destinationID}
	} else {
		query = `
			SELECT
				destination_id,
				COALESCE(SUM(CASE WHEN is_ghost_booking = true THEN seats_booked ELSE 0 END), 0) AS ghost_tickets,
				COALESCE(SUM(CASE WHEN is_ghost_booking = false THEN seats_booked ELSE 0 END), 0) AS regular_tickets,
				COALESCE(SUM(seats_booked), 0) AS total_tickets
			FROM bookings
			WHERE booking_status = 'ACTIVE'
			  AND created_at::date = CURRENT_DATE
			GROUP BY destination_id
		`
		args = []interface{}{}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := make([]TodayBookedTicketsByDestination, 0)
	for rows.Next() {
		var destID string
		var ghostTickets int
		var regularTickets int
		var totalTickets int
		if err := rows.Scan(&destID, &ghostTickets, &regularTickets, &totalTickets); err != nil {
			return nil, err
		}
		results = append(results, TodayBookedTicketsByDestination{
			DestinationID:     destID,
			RegularCountToday: regularTickets,
			GhostCountToday:   ghostTickets,
			TotalToday:        totalTickets,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// CreateGhostBooking creates N separate ghost bookings (one per seat) with sequential seat numbers.
func (r *RepositoryImpl) CreateGhostBooking(ctx context.Context, req CreateGhostBookingRequest) ([]*GhostBooking, error) {
	if req.Seats <= 0 {
		return nil, fmt.Errorf("seats must be > 0")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Idempotency: if bookings were already created for this key batch, return them.
	baseKey := strings.TrimSpace(req.IdempotencyKey)
	if baseKey != "" {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, baseKey); err != nil {
			return nil, err
		}
		existing, getErr := r.getGhostBookingsByIdempotencyBatchTx(ctx, tx, baseKey, req.Seats)
		if getErr == nil && len(existing) > 0 {
			if err := tx.Commit(ctx); err != nil {
				return nil, err
			}
			return existing, nil
		}
	}

	resolvedDestinationID, destinationName, basePrice, serviceFeePerSeat, err := r.resolveGhostDestination(ctx, tx, req.DestinationID, req.StaffID)
	if err != nil {
		return nil, err
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1, hashtext($2::text)::int)`, bookingDestinationTxAdvLockK1, resolvedDestinationID); err != nil {
		return nil, err
	}

	var staffName string
	if req.StaffID != "" {
		err = tx.QueryRow(ctx, `SELECT CONCAT(first_name, ' ', last_name) FROM staff WHERE id = $1`, req.StaffID).Scan(&staffName)
		if err != nil {
			staffName = "Unknown Staff"
		}
	} else {
		staffName = "System"
	}

	var nextGhostNumber int
	err = tx.QueryRow(ctx, `
		SELECT COALESCE(MAX(seat_number), -1) + 1
		FROM bookings 
		WHERE is_ghost_booking = true
		  AND destination_id = $1
		  AND DATE(created_at) = CURRENT_DATE`, resolvedDestinationID).Scan(&nextGhostNumber)
	if err != nil {
		nextGhostNumber = 0
	}
	if nextGhostNumber+req.Seats > 9999 {
		return nil, fmt.Errorf("daily ghost booking index limit reached for destination %s (max 9999)", resolvedDestinationID)
	}

	var referenceDatePrefix string
	err = tx.QueryRow(ctx, `SELECT TO_CHAR(CURRENT_DATE, 'YYMMDD')`).Scan(&referenceDatePrefix)
	if err != nil {
		return nil, err
	}

	seatPrice := basePrice + serviceFeePerSeat
	const verificationCodeSpace = 10000
	const verificationCodeStep = 9973

	results := make([]*GhostBooking, 0, req.Seats)

	for i := 0; i < req.Seats; i++ {
		seatNum := nextGhostNumber + i
		seatTotal := seatPrice

		// Per-seat idempotency key derived from the batch key
		var perSeatKey *string
		if baseKey != "" {
			k := fmt.Sprintf("%s:%d", baseKey, i)
			perSeatKey = &k
		}

		var bookingID, verificationCode string
		var createdAt time.Time
		var inserted bool

		for attempt := 0; attempt < verificationCodeSpace; attempt++ {
			suffix := (seatNum + (attempt * verificationCodeStep)) % verificationCodeSpace
			verificationCode = fmt.Sprintf("%s%04d", referenceDatePrefix, suffix)

			err = tx.QueryRow(ctx, `
				INSERT INTO bookings (
					id, destination_id, seats_booked, seat_number, total_amount, booking_source, booking_type,
					booking_status, payment_status, payment_method, verification_code,
					is_verified, is_ghost_booking, created_by, idempotency_key
				) VALUES (
					substr(md5(random()::text || clock_timestamp()::text),1,24),
					$1, 1, $2, $3, 'CASH_STATION', 'CASH', 'ACTIVE', 'PAID', 'CASH',
					$4, false, true, $5, $6
				)
				ON CONFLICT (verification_code) DO NOTHING
				RETURNING id, verification_code, created_at`,
				resolvedDestinationID, seatNum, seatTotal, verificationCode, req.StaffID, perSeatKey).Scan(&bookingID, &verificationCode, &createdAt)
			if err == nil {
				inserted = true
				break
			}
			if err == pgx.ErrNoRows {
				continue
			}
			return nil, err
		}
		if !inserted {
			return nil, fmt.Errorf("failed to generate unique ghost verification code for seat %d", i)
		}

		results = append(results, &GhostBooking{
			ID:               bookingID,
			DestinationID:    resolvedDestinationID,
			DestinationName:  destinationName,
			SeatsBooked:      1,
			SeatNumber:       seatNum,
			TotalAmount:      seatTotal,
			BookingStatus:    "ACTIVE",
			PaymentStatus:    "PAID",
			VerificationCode: verificationCode,
			CreatedBy:        req.StaffID,
			CreatedByName:    staffName,
			CreatedAt:        naiveTSTunisFromPg(createdAt),
			IsGhostBooking:   true,
			BasePrice:        basePrice,
		})
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return results, nil
}

// getGhostBookingsByIdempotencyBatchTx returns all ghost bookings whose idempotency_key
// matches the batch pattern "{baseKey}:{i}" for i in [0..seats).
func (r *RepositoryImpl) getGhostBookingsByIdempotencyBatchTx(ctx context.Context, tx pgx.Tx, baseKey string, seats int) ([]*GhostBooking, error) {
	baseKey = strings.TrimSpace(baseKey)
	if baseKey == "" {
		return nil, fmt.Errorf("empty idempotency key")
	}

	// Check if the first seat's key exists — if not, the batch was never created.
	firstKey := fmt.Sprintf("%s:0", baseKey)
	rows, err := tx.Query(ctx, `
		SELECT
			b.id,
			b.destination_id,
			COALESCE(r.station_name, ''),
			b.seats_booked,
			b.seat_number,
			b.total_amount,
			b.booking_status,
			b.payment_status,
			b.verification_code,
			COALESCE(b.created_by, ''),
			COALESCE(CONCAT(s.first_name, ' ', s.last_name), 'Unknown Staff') AS created_by_name,
			b.created_at,
			TRUE AS is_ghost_booking,
			COALESCE(r.base_price, 0) AS base_price
		FROM bookings b
		LEFT JOIN routes r ON r.station_id = b.destination_id
		LEFT JOIN staff s ON s.id = b.created_by
		WHERE b.idempotency_key LIKE $1
		  AND b.is_ghost_booking = TRUE
		ORDER BY b.seat_number ASC
	`, firstKey[:len(baseKey)+1]+"%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []*GhostBooking
	for rows.Next() {
		var gb GhostBooking
		var createdAt time.Time
		if err := rows.Scan(
			&gb.ID, &gb.DestinationID, &gb.DestinationName,
			&gb.SeatsBooked, &gb.SeatNumber, &gb.TotalAmount,
			&gb.BookingStatus, &gb.PaymentStatus, &gb.VerificationCode,
			&gb.CreatedBy, &gb.CreatedByName, &createdAt,
			&gb.IsGhostBooking, &gb.BasePrice,
		); err != nil {
			return nil, err
		}
		gb.CreatedAt = naiveTSTunisFromPg(createdAt)
		results = append(results, &gb)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no existing ghost bookings for key batch %s", baseKey)
	}
	return results, nil
}

// GetGhostBookingCount returns the count of active ghost bookings for a destination
func (r *RepositoryImpl) GetGhostBookingCount(ctx context.Context, destinationID string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) 
		FROM bookings 
		WHERE destination_id = $1
		  AND is_ghost_booking = true
		  AND booking_status = 'ACTIVE'
		  AND DATE(created_at) = CURRENT_DATE`, destinationID).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetTodayTripsCountByLicensePlate returns the count of trips for a specific license plate today
func (r *RepositoryImpl) GetTodayTripsCountByLicensePlate(ctx context.Context, licensePlate string) (int, error) {
	var count int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*)
		FROM trips
		WHERE start_time::date = CURRENT_DATE AND license_plate = $1
	`, licensePlate).Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// GetServiceFeeTNDByDestination returns per-seat station fee from routes.service_fee (millimes stored as TND fraction).
func (r *RepositoryImpl) GetServiceFeeTNDByDestination(ctx context.Context, destinationID string) (float64, error) {
	if strings.TrimSpace(destinationID) == "" {
		return pricing.ServiceFeePerSeatTND, nil
	}
	var fee float64
	err := r.db.QueryRow(ctx, `
		SELECT COALESCE(service_fee, $2) FROM routes WHERE station_id = $1
	`, destinationID, pricing.ServiceFeePerSeatTND).Scan(&fee)
	if errors.Is(err, pgx.ErrNoRows) {
		return pricing.ServiceFeePerSeatTND, nil
	}
	if err != nil {
		return 0, err
	}
	return fee, nil
}
