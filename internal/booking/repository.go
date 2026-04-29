package booking

import (
	"context"
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
}

type RepositoryImpl struct {
	db *pgxpool.Pool
}

const verificationAttempts = 32

func nullableText(v string) interface{} {
	if strings.TrimSpace(v) == "" {
		return nil
	}
	return v
}

// firstTripOfDayReplayTx is true when no trip was recorded yet for this vehicle on the same calendar day at or before refTime (destination-agnostic). Trips are inserted after booking rows so start_time lines up with idempotent replay.
func firstTripOfDayReplayTx(ctx context.Context, tx pgx.Tx, vehicleID string, refTime time.Time) (bool, error) {
	if strings.TrimSpace(vehicleID) == "" {
		return false, nil
	}
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM trips t
		INNER JOIN vehicle_queue q ON q.id = t.queue_id
		WHERE q.vehicle_id = $1
		  AND CAST(t.start_time AS date) = CAST($2::timestamptz AS date)
		  AND t.start_time <= $2
	`, vehicleID, refTime).Scan(&n)
	if err != nil {
		return false, err
	}
	return n == 0, nil
}

// vehicleCompletedTripsTodayTx counts trips already started today for this vehicle (any destination).
func vehicleCompletedTripsTodayTx(ctx context.Context, tx pgx.Tx, vehicleID string) (int, error) {
	if strings.TrimSpace(vehicleID) == "" {
		return 0, nil
	}
	var n int
	err := tx.QueryRow(ctx, `
		SELECT COUNT(*)::int
		FROM trips t
		INNER JOIN vehicle_queue q ON q.id = t.queue_id
		WHERE q.vehicle_id = $1
		  AND CAST(t.start_time AS date) = CURRENT_DATE
	`, vehicleID).Scan(&n)
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
	ft, err := firstTripOfDayReplayTx(ctx, tx, b.VehicleID, b.CreatedAt)
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
		bookings = append(bookings, b)
	}
	if len(bookings) == 0 {
		return nil, pgx.ErrNoRows
	}

	minT := bookings[0].CreatedAt
	vid := bookings[0].VehicleID
	for _, b := range bookings {
		if b.CreatedAt.Before(minT) {
			minT = b.CreatedAt
		}
	}
	ft, err := firstTripOfDayReplayTx(ctx, tx, vid, minT)
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

// CreateBookingByDestination allocates seats from the first queue entry with sufficient seats, else tries next entries in order
func (r *RepositoryImpl) CreateBookingByDestination(ctx context.Context, req CreateBookingByDestinationRequest) (*Booking, error) {
	if req.Seats <= 0 {
		return nil, fmt.Errorf("seats must be > 0")
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

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

	// Always try exact-fit first, then fall back to first-eligible
	var row pgx.Row
	var queueID, vehicleID string
	var pricePerSeat float64
	var serviceFeePerSeat float64
	{
		if req.SubRoute != nil && *req.SubRoute != "" {
			row = tx.QueryRow(ctx, `
                WITH candidate AS (
                    SELECT id, destination_id
                    FROM vehicle_queue
                    WHERE destination_id=$1 AND queue_type='REGULAR' AND status IN ('WAITING','LOADING','READY')
                      AND sub_route=$3 AND available_seats = $2
                    ORDER BY queue_position ASC
                    LIMIT 1
                    FOR UPDATE
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
                    SELECT id, destination_id
                    FROM vehicle_queue
                    WHERE destination_id=$1 AND queue_type='REGULAR' AND status IN ('WAITING','LOADING','READY')
                      AND available_seats = $2
                    ORDER BY queue_position ASC
                    LIMIT 1
                    FOR UPDATE
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

	// If exact-fit not requested or not found, fall back to first-eligible
	if queueID == "" {
		if req.SubRoute != nil && *req.SubRoute != "" {
			row = tx.QueryRow(ctx, `
                WITH candidate AS (
                    SELECT id, destination_id
                    FROM vehicle_queue
                    WHERE destination_id=$1 AND queue_type='REGULAR' AND status IN ('WAITING','LOADING','READY')
                      AND sub_route=$3 AND available_seats >= $2
                    ORDER BY queue_position ASC
                    LIMIT 1
                    FOR UPDATE
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
                    SELECT id, destination_id
                    FROM vehicle_queue
                    WHERE destination_id=$1 AND queue_type='REGULAR' AND status IN ('WAITING','LOADING','READY')
                      AND available_seats >= $2
                    ORDER BY queue_position ASC
                    LIMIT 1
                    FOR UPDATE
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

	tripsToday, err := vehicleCompletedTripsTodayTx(ctx, tx, vehicleID)
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

// ListQueueSnapshot returns the current queue for a destination (minimal columns for UI refresh)
func (r *RepositoryImpl) ListQueueSnapshot(ctx context.Context, destinationID string) ([]QueueEntry, error) {
	rows, err := r.db.Query(ctx, `
        SELECT q.id, q.vehicle_id, COALESCE(NULLIF(v.license_plate, ''), NULLIF(q.license_plate, ''), '[UNKNOWN]'), q.destination_id, q.destination_name,
               q.sub_route, q.sub_route_name, q.queue_type, q.queue_position, q.status,
               q.entered_at, q.available_seats, q.total_seats, q.base_price,
               q.estimated_departure, q.actual_departure
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
			&e.EnteredAt, &e.AvailableSeats, &e.TotalSeats, &e.BasePrice, &e.EstimatedDeparture, &e.ActualDeparture); err != nil {
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
            t.created_at
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
		if err := rows.Scan(&t.ID, &t.VehicleID, &t.LicensePlate, &t.DestinationID, &t.DestinationName, &t.QueueID, &t.SeatsBooked, &t.VehicleCapacity, &t.BasePrice, &t.StartTime, &t.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, t)
	}
	return list, nil
}

// ListTodayTrips returns trips for the current day, optionally filtered by license plate
func (r *RepositoryImpl) ListTodayTrips(ctx context.Context, search string, limit int) ([]Trip, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
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
                t.created_at
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
                t.created_at
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
		if err := rows.Scan(&t.ID, &t.VehicleID, &t.LicensePlate, &t.DestinationID, &t.DestinationName, &t.QueueID, &t.SeatsBooked, &t.VehicleCapacity, &t.BasePrice, &t.StartTime, &t.CreatedAt); err != nil {
			return nil, err
		}
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
		FOR UPDATE OF q`, req.QueueEntryID, pricing.ServiceFeePerSeatTND).Scan(&queueID, &vehicleID, &pricePerSeat, &serviceFeePerSeat, &availableSeats)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, fmt.Errorf("queue entry not found or not available for booking")
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

	tripsToday, err := vehicleCompletedTripsTodayTx(ctx, tx, vehicleID)
	if err != nil {
		return nil, err
	}
	firstTripOfDay := tripsToday == 0

	fmt.Printf("DEBUG: Checking if vehicle becomes fully booked - newAvailableSeats: %d\n", newAvailableSeats)
	if newAvailableSeats == 0 {
		fmt.Printf("DEBUG: Vehicle fully booked after this booking; trip/exitPass created after seats\n")
	} else {
		fmt.Printf("DEBUG: Vehicle not fully booked yet - available seats: %d\n", newAvailableSeats)
	}

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
			CreatedAt:        createdAt,
			FirstTripOfDay:   firstTripOfDay,
		})
	}

	// Trip + exit pass after booking rows so firstTripOfDay replay stays consistent with created_at.
	if newAvailableSeats == 0 {
		fmt.Printf("DEBUG: Vehicle is now fully booked! Creating trip record...\n")
		tripID := fmt.Sprintf("trip_%d", time.Now().UnixNano())
		currentExitTime := time.Now().In(time.FixedZone("Africa/Tunis", 3600)) // Use Tunisia timezone

		fmt.Printf("DEBUG: Inserting trip record with ID: %s, Vehicle: %s, Destination: %s\n", tripID, licensePlate, destName)
		if _, err := tx.Exec(ctx, `
			INSERT INTO trips (
				id, queue_id, destination_id, destination_name, license_plate,
				start_time, created_at, total_seats, booked_seats, created_by
			) VALUES (
				$1, $2, $3, $4, $5, NOW(), NOW(), $6, $7, $8
			)`, tripID, queueID, destID, destName, licensePlate, totalSeats, totalSeats, req.StaffID); err != nil {
			fmt.Printf("DEBUG: Error creating trip record: %v\n", err)
			return nil, err
		}
		fmt.Printf("DEBUG: Trip record created successfully!\n")

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
		fmt.Printf("DEBUG: Exit pass created for frontend\n")
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

	var staffName string
	if req.StaffID != "" {
		err = tx.QueryRow(ctx, `SELECT CONCAT(first_name, ' ', last_name) FROM staff WHERE id = $1`, req.StaffID).Scan(&staffName)
		if err != nil {
			staffName = "Unknown Staff"
		}
	} else {
		staffName = "System"
	}

	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, resolvedDestinationID); err != nil {
		return nil, err
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
			CreatedAt:        createdAt,
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
		gb.CreatedAt = createdAt
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
