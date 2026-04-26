package publicapi

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrStationNotInitialized    = errors.New("STATION_NOT_INITIALIZED")
	ErrStationAlreadyConfigured = errors.New("STATION_ALREADY_CONFIGURED")
	ErrInsufficientSeats        = errors.New("INSUFFICIENT_SEATS")
	ErrBookingNotFound          = errors.New("BOOKING_NOT_FOUND")
	ErrBookingExpired           = errors.New("BOOKING_EXPIRED")
	ErrBookingStateConflict     = errors.New("BOOKING_STATUS_CONFLICT")
)

type Repository interface {
	InitializeStationInfo(ctx context.Context, name string, location string) (*StationInfoResponse, error)
	GetStationInfo(ctx context.Context) (*StationInfoResponse, error)
	ListRouteAvailability(ctx context.Context) ([]RouteAvailability, error)
	GetRouteDetails(ctx context.Context, destinationID string) (*RouteDetailsResponse, error)
	CreateBookingHold(ctx context.Context, req CreateBookingRequest, holdTTL time.Duration) (*BookingResponse, bool, error)
	GetBooking(ctx context.Context, bookingID string) (*BookingResponse, error)
	ConfirmBooking(ctx context.Context, bookingID string, req ConfirmBookingRequest) (*BookingResponse, error)
	CancelBooking(ctx context.Context, bookingID string, req CancelBookingRequest) (*BookingResponse, error)
	ExpireHeldBookings(ctx context.Context) (int, error)
}

type RepositoryImpl struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &RepositoryImpl{db: db}
}

func randomID(prefix string) string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("%s_%d", prefix, time.Now().UnixNano())
	}
	return fmt.Sprintf("%s_%s", prefix, hex.EncodeToString(buf))
}

func randomVerificationCode() string {
	buf := make([]byte, 3)
	if _, err := rand.Read(buf); err != nil {
		return "000000"
	}
	n := int(buf[0])<<16 | int(buf[1])<<8 | int(buf[2])
	return fmt.Sprintf("%06d", n%1000000)
}

func (r *RepositoryImpl) InitializeStationInfo(ctx context.Context, name string, location string) (*StationInfoResponse, error) {
	var stationID string
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	// Serialize initialization so concurrent first-run requests cannot create multiple configs.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(884001)); err != nil {
		return nil, err
	}

	err = tx.QueryRow(ctx, `
		SELECT station_id
		FROM station_config
		ORDER BY created_at DESC
		LIMIT 1`).Scan(&stationID)
	switch {
	case err == nil:
		return nil, ErrStationAlreadyConfigured
	case !errors.Is(err, pgx.ErrNoRows):
		return nil, err
	}

	id := randomID("cfg")
	stationID = randomID("st")
	if _, err = tx.Exec(ctx, `
		INSERT INTO station_config (
			id, station_id, station_name, governorate, delegation, address, updated_at
		) VALUES ($1, $2, $3, $4, $4, $4, NOW())`,
		id, stationID, name, location); err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}

	return &StationInfoResponse{
		StationID: stationID,
		Name:      name,
		Location:  location,
	}, nil
}

func (r *RepositoryImpl) GetStationInfo(ctx context.Context) (*StationInfoResponse, error) {
	var info StationInfoResponse
	err := r.db.QueryRow(ctx, `
		SELECT station_id,
		       station_name,
		       COALESCE(NULLIF(address, ''), delegation, governorate, '')
		FROM station_config
		ORDER BY created_at DESC
		LIMIT 1`).Scan(&info.StationID, &info.Name, &info.Location)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStationNotInitialized
	}
	if err != nil {
		return nil, err
	}
	return &info, nil
}

func (r *RepositoryImpl) ListRouteAvailability(ctx context.Context) ([]RouteAvailability, error) {
	rows, err := r.db.Query(ctx, `
		SELECT q.destination_id,
		       q.destination_name,
		       GREATEST(SUM(q.available_seats)::int - COALESCE(wib.seats_booked, 0), 0) AS available_seats_total,
		       COUNT(*)::int AS vehicles_count
		FROM vehicle_queue q
		LEFT JOIN (
			SELECT destination_id, SUM(seats_booked)::int AS seats_booked
			FROM wasla_intern_booking
			WHERE booking_status IN ('HELD', 'ACTIVE')
			GROUP BY destination_id
		) wib ON wib.destination_id = q.destination_id
		WHERE q.queue_type = 'REGULAR'
		  AND q.status IN ('WAITING', 'LOADING', 'READY')
		  AND q.available_seats > 0
		GROUP BY q.destination_id, q.destination_name, wib.seats_booked
		ORDER BY q.destination_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]RouteAvailability, 0)
	for rows.Next() {
		var item RouteAvailability
		if err := rows.Scan(
			&item.DestinationID,
			&item.DestinationName,
			&item.AvailableSeatsTotal,
			&item.VehiclesCount,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func (r *RepositoryImpl) GetRouteDetails(ctx context.Context, destinationID string) (*RouteDetailsResponse, error) {
	rows, err := r.db.Query(ctx, `
		SELECT q.id,
		       q.vehicle_id,
		       GREATEST(q.available_seats - COALESCE(wib.seats_booked, 0), 0) AS available_seats
		FROM vehicle_queue q
		LEFT JOIN (
			SELECT queue_id, SUM(seats_booked)::int AS seats_booked
			FROM wasla_intern_booking
			WHERE booking_status IN ('HELD', 'ACTIVE')
			GROUP BY queue_id
		) wib ON wib.queue_id = q.id
		WHERE q.destination_id = $1
		  AND q.queue_type = 'REGULAR'
		  AND q.status IN ('WAITING', 'LOADING', 'READY')
		  AND (q.available_seats - COALESCE(wib.seats_booked, 0)) > 0
		ORDER BY q.queue_position ASC`, destinationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	res := &RouteDetailsResponse{
		DestinationID: destinationID,
		Vehicles:      make([]RouteVehicle, 0),
	}

	for rows.Next() {
		var v RouteVehicle
		if err := rows.Scan(&v.QueueID, &v.VehicleID, &v.AvailableSeats); err != nil {
			return nil, err
		}
		v.Status = "OPEN"
		res.Vehicles = append(res.Vehicles, v)
	}

	return res, nil
}

func (r *RepositoryImpl) CreateBookingHold(ctx context.Context, req CreateBookingRequest, holdTTL time.Duration) (*BookingResponse, bool, error) {
	if req.SeatsBooked <= 0 {
		return nil, false, fmt.Errorf("seats_booked must be greater than 0")
	}

	if req.IdempotencyKey != "" {
		if existing, err := r.getBookingByIdempotency(ctx, req.IdempotencyKey); err == nil {
			return existing, true, nil
		} else if !errors.Is(err, ErrBookingNotFound) {
			return nil, false, err
		}
	}

	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer tx.Rollback(ctx)

	if req.IdempotencyKey != "" {
		if existing, err := r.getBookingByIdempotencyTx(ctx, tx, req.IdempotencyKey); err == nil {
			return existing, true, nil
		} else if !errors.Is(err, ErrBookingNotFound) {
			return nil, false, err
		}
	}

	var queueID string
	var destinationID string
	var basePrice float64
	err = tx.QueryRow(ctx, `
		SELECT q.id,
		       q.destination_id,
		       COALESCE(r.base_price, q.base_price)
		FROM vehicle_queue q
		LEFT JOIN routes r ON r.station_id = q.destination_id
		LEFT JOIN (
			SELECT queue_id, SUM(seats_booked)::int AS seats_booked
			FROM wasla_intern_booking
			WHERE booking_status IN ('HELD', 'ACTIVE')
			GROUP BY queue_id
		) wib ON wib.queue_id = q.id
		WHERE q.destination_id = $1
		  AND q.queue_type = 'REGULAR'
		  AND q.status IN ('WAITING', 'LOADING', 'READY')
		  AND (q.available_seats - COALESCE(wib.seats_booked, 0)) >= $2
		ORDER BY q.queue_position ASC
		LIMIT 1`,
		req.DestinationID, req.SeatsBooked,
	).Scan(&queueID, &destinationID, &basePrice)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, ErrInsufficientSeats
	}
	if err != nil {
		return nil, false, err
	}

	source := strings.TrimSpace(req.BookingSource)
	if source == "" {
		source = "CENTRAL"
	}
	bookingType := strings.TrimSpace(req.BookingType)
	if bookingType == "" {
		bookingType = "ONLINE"
	}

	bookingID := randomID("bkg")
	expiresAt := time.Now().Add(holdTTL)
	totalAmount := float64(req.SeatsBooked) * basePrice

	for attempt := 0; attempt < 8; attempt++ {
		verificationCode := randomVerificationCode()
		_, err = tx.Exec(ctx, `
			INSERT INTO wasla_intern_booking (
				id, queue_id, destination_id, seats_booked, total_amount,
				booking_source, booking_type, booking_status, payment_status,
				payment_method, verification_code, is_verified,
				user_ref, idempotency_key, expires_at, created_at, updated_at
			) VALUES (
				$1, $2, $3, $4, $5,
				$6, $7, 'HELD', 'UNPAID',
				'ONLINE', $8, false,
				$9, $10, $11, NOW(), NOW()
			)`,
			bookingID, queueID, destinationID, req.SeatsBooked, totalAmount,
			source, bookingType, verificationCode,
			nullableText(req.UserRef), nullableText(req.IdempotencyKey), expiresAt,
		)
		if err == nil {
			break
		}

		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
			return nil, false, err
		}

		// Idempotency replay race: return previously created booking.
		if req.IdempotencyKey != "" {
			if existing, getErr := r.getBookingByIdempotency(ctx, req.IdempotencyKey); getErr == nil {
				return existing, true, nil
			}
		}

		// Retry only verification_code unique conflicts; fail fast otherwise.
		conflict := strings.ToLower(pgErr.ConstraintName + " " + pgErr.Detail + " " + pgErr.Message)
		if !strings.Contains(conflict, "verification") {
			return nil, false, err
		}

		// Last retry exhausted.
		if attempt == 7 {
			return nil, false, err
		}
	}

	res, err := r.getBookingTx(ctx, tx, bookingID)
	if err != nil {
		return nil, false, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}

	return res, false, nil
}

func (r *RepositoryImpl) GetBooking(ctx context.Context, bookingID string) (*BookingResponse, error) {
	row := r.db.QueryRow(ctx, bookingSelectByID, bookingID)
	booking, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func (r *RepositoryImpl) ConfirmBooking(ctx context.Context, bookingID string, req ConfirmBookingRequest) (*BookingResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	lockData, err := getLockedBooking(ctx, tx, bookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}

	if lockData.BookingStatus == "ACTIVE" && lockData.PaymentStatus == "PAID" {
		booking, err := r.getBookingTx(ctx, tx, bookingID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return booking, nil
	}

	if lockData.BookingStatus != "HELD" {
		return nil, ErrBookingStateConflict
	}

	if lockData.ExpiresAt != nil && time.Now().After(*lockData.ExpiresAt) {
		if _, err := tx.Exec(ctx, `
			UPDATE wasla_intern_booking
			SET booking_status='EXPIRED',
			    payment_status=CASE WHEN payment_status='UNPAID' THEN 'FAILED' ELSE payment_status END,
			    cancelled_at=COALESCE(cancelled_at, NOW()),
			    cancellation_reason=COALESCE(cancellation_reason, 'HOLD_EXPIRED'),
			    updated_at=NOW()
			WHERE id=$1`, bookingID); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return nil, ErrBookingExpired
	}

	paymentProcessedAt := req.PaymentProcessedAt
	if paymentProcessedAt == nil {
		now := time.Now()
		paymentProcessedAt = &now
	}

	if _, err := tx.Exec(ctx, `
		UPDATE wasla_intern_booking
		SET booking_status='ACTIVE',
		    payment_status='PAID',
		    payment_method=$2,
		    payment_processed_at=$3,
		    updated_at=NOW()
		WHERE id=$1`,
		bookingID, req.PaymentMethod, *paymentProcessedAt); err != nil {
		return nil, err
	}

	booking, err := r.getBookingTx(ctx, tx, bookingID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return booking, nil
}

func (r *RepositoryImpl) CancelBooking(ctx context.Context, bookingID string, req CancelBookingRequest) (*BookingResponse, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	lockData, err := getLockedBooking(ctx, tx, bookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}

	if lockData.BookingStatus == "CANCELLED" || lockData.BookingStatus == "EXPIRED" {
		booking, err := r.getBookingTx(ctx, tx, bookingID)
		if err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return booking, nil
	}

	paymentStatus := lockData.PaymentStatus
	if strings.Contains(strings.ToUpper(req.CancellationReason), "PAYMENT") {
		paymentStatus = "FAILED"
	}

	reason := strings.TrimSpace(req.CancellationReason)
	cancelledByActor := strings.TrimSpace(req.CancelledBy)
	if cancelledByActor != "" {
		if reason == "" {
			reason = fmt.Sprintf("cancelled_by=%s", cancelledByActor)
		} else {
			reason = fmt.Sprintf("%s | cancelled_by=%s", reason, cancelledByActor)
		}
	}

	var cancelledByStaff *string
	if cancelledByActor != "" {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM staff WHERE id=$1)`, cancelledByActor).Scan(&exists); err == nil && exists {
			cancelledByStaff = &cancelledByActor
		}
	}

	if _, err := tx.Exec(ctx, `
		UPDATE wasla_intern_booking
		SET booking_status='CANCELLED',
		    payment_status=$2,
		    cancelled_at=NOW(),
		    cancelled_by=$3,
		    cancellation_reason=$4,
		    updated_at=NOW()
		WHERE id=$1`,
		bookingID, paymentStatus, cancelledByStaff, nullableText(reason)); err != nil {
		return nil, err
	}

	booking, err := r.getBookingTx(ctx, tx, bookingID)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return booking, nil
}

func (r *RepositoryImpl) ExpireHeldBookings(ctx context.Context) (int, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)

	rows, err := tx.Query(ctx, `
		SELECT id
		FROM wasla_intern_booking
		WHERE booking_status='HELD'
		  AND expires_at IS NOT NULL
		  AND expires_at < NOW()
		FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	bookingIDs := make([]string, 0)

	for rows.Next() {
		var bookingID string
		if err := rows.Scan(&bookingID); err != nil {
			return 0, err
		}
		bookingIDs = append(bookingIDs, bookingID)
	}

	if len(bookingIDs) == 0 {
		if err := tx.Commit(ctx); err != nil {
			return 0, err
		}
		return 0, nil
	}

	if _, err := tx.Exec(ctx, `
		UPDATE wasla_intern_booking
		SET booking_status='EXPIRED',
		    payment_status=CASE WHEN payment_status='UNPAID' THEN 'FAILED' ELSE payment_status END,
		    cancelled_at=COALESCE(cancelled_at, NOW()),
		    cancellation_reason=COALESCE(cancellation_reason, 'HOLD_EXPIRED'),
		    updated_at=NOW()
		WHERE id = ANY($1)`, bookingIDs); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return len(bookingIDs), nil
}

type lockedBooking struct {
	BookingStatus string
	PaymentStatus string
	ExpiresAt     *time.Time
}

func getLockedBooking(ctx context.Context, tx pgx.Tx, bookingID string) (*lockedBooking, error) {
	var lb lockedBooking
	err := tx.QueryRow(ctx, `
		SELECT booking_status, payment_status, expires_at
		FROM wasla_intern_booking
		WHERE id=$1
		FOR UPDATE`, bookingID).Scan(
		&lb.BookingStatus,
		&lb.PaymentStatus,
		&lb.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}
	return &lb, nil
}

const bookingSelectByID = `
	SELECT b.id, b.queue_id, b.destination_id, b.seats_booked, b.booking_status, b.payment_status,
	       b.payment_method, b.payment_processed_at, b.expires_at, b.created_at, v.license_plate
	FROM wasla_intern_booking b
	LEFT JOIN vehicle_queue q ON q.id = b.queue_id
	LEFT JOIN vehicles v ON v.id = q.vehicle_id
	WHERE b.id=$1`

func scanBooking(row pgx.Row) (*BookingResponse, error) {
	var b BookingResponse
	var queueID *string
	var destinationID *string
	var vehicleLicensePlate *string
	err := row.Scan(
		&b.BookingID,
		&queueID,
		&destinationID,
		&b.SeatsBooked,
		&b.BookingStatus,
		&b.PaymentStatus,
		&b.PaymentMethod,
		&b.PaymentProcessedAt,
		&b.ExpiresAt,
		&b.CreatedAt,
		&vehicleLicensePlate,
	)
	if err != nil {
		return nil, err
	}
	if queueID != nil {
		b.QueueID = *queueID
	}
	if destinationID != nil {
		b.DestinationID = *destinationID
	}
	if vehicleLicensePlate != nil {
		b.VehicleLicensePlate = *vehicleLicensePlate
	}
	return &b, nil
}

func (r *RepositoryImpl) getBookingTx(ctx context.Context, tx pgx.Tx, bookingID string) (*BookingResponse, error) {
	row := tx.QueryRow(ctx, bookingSelectByID, bookingID)
	booking, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func (r *RepositoryImpl) getBookingByIdempotency(ctx context.Context, key string) (*BookingResponse, error) {
	row := r.db.QueryRow(ctx, `
		SELECT b.id, b.queue_id, b.destination_id, b.seats_booked, b.booking_status, b.payment_status,
		       b.payment_method, b.payment_processed_at, b.expires_at, b.created_at, v.license_plate
		FROM wasla_intern_booking b
		LEFT JOIN vehicle_queue q ON q.id = b.queue_id
		LEFT JOIN vehicles v ON v.id = q.vehicle_id
		WHERE b.idempotency_key=$1
		ORDER BY b.created_at DESC
		LIMIT 1`, key)
	booking, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func (r *RepositoryImpl) getBookingByIdempotencyTx(ctx context.Context, tx pgx.Tx, key string) (*BookingResponse, error) {
	row := tx.QueryRow(ctx, `
		SELECT b.id, b.queue_id, b.destination_id, b.seats_booked, b.booking_status, b.payment_status,
		       b.payment_method, b.payment_processed_at, b.expires_at, b.created_at, v.license_plate
		FROM wasla_intern_booking b
		LEFT JOIN vehicle_queue q ON q.id = b.queue_id
		LEFT JOIN vehicles v ON v.id = q.vehicle_id
		WHERE b.idempotency_key=$1
		ORDER BY b.created_at DESC
		LIMIT 1
		FOR UPDATE OF b`, key)
	booking, err := scanBooking(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBookingNotFound
	}
	if err != nil {
		return nil, err
	}
	return booking, nil
}

func nullableText(v string) *string {
	v = strings.TrimSpace(v)
	if v == "" {
		return nil
	}
	return &v
}
