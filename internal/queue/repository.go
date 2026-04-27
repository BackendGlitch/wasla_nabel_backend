package queue

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Repository interface {
	// Routes (includeInactive: list inactive rows too, for management CRUD)
	ListRoutes(ctx context.Context, includeInactive bool) ([]Route, error)
	CreateRoute(ctx context.Context, r CreateRouteRequest) (*Route, error)
	UpdateRoute(ctx context.Context, id string, r UpdateRouteRequest) (*Route, error)
	DeleteRoute(ctx context.Context, id string) error

	// Vehicles
	ListVehicles(ctx context.Context, searchQuery string) ([]Vehicle, error)
	CreateVehicle(ctx context.Context, v CreateVehicleRequest) (*Vehicle, error)
	UpdateVehicle(ctx context.Context, id string, v UpdateVehicleRequest) (*Vehicle, error)
	DeleteVehicle(ctx context.Context, id string) error

	// Authorized routes
	ListAuthorizedRoutes(ctx context.Context, vehicleID string) ([]VehicleAuthorizedStation, error)
	AddAuthorizedRoute(ctx context.Context, vehicleID string, req AddAuthorizedRouteRequest) (*VehicleAuthorizedStation, error)
	UpdateAuthorizedRoute(ctx context.Context, authID string, req UpdateAuthorizedRouteRequest) (*VehicleAuthorizedStation, error)
	DeleteAuthorizedRoute(ctx context.Context, authID string) error

	// Queue entries
	ListQueue(ctx context.Context, destinationID string, subRoute *string) ([]QueueEntry, error)
	AddQueueEntry(ctx context.Context, req AddQueueEntryRequest) (*QueueEntry, *DayPassCreatedEvent, *DayPassCreatedEvent, string, error)
	GetVehicleDayPass(ctx context.Context, vehicleID string) (*DayPassCreatedEvent, error)
	UpdateQueueEntry(ctx context.Context, id string, req UpdateQueueEntryRequest) (*QueueEntry, error)
	DeleteQueueEntry(ctx context.Context, id string) error
	ReorderQueue(ctx context.Context, destinationID string, entryIDs []string) error

	// Move & Transfer
	MoveEntry(ctx context.Context, entryID string, destinationID string, newPos int) error
	TransferSeats(ctx context.Context, fromEntryID, toEntryID string, seats int) error

	ChangeDestination(ctx context.Context, entryID, newDestID, newDestName string) error
	ListDayPasses(ctx context.Context, limit int) ([]DayPass, error)
	ExpireDayPassesAndClearRegularQueues(ctx context.Context) (expiredPasses int64, clearedQueueEntries int64, err error)
	// Clear queues
	ClearQueue(ctx context.Context, destinationID string) error
	ClearAllQueues(ctx context.Context) error
	// Aggregates
	ListQueueSummaries(ctx context.Context, station string) ([]QueueSummary, error)
	ListAllDestinations(ctx context.Context) ([]Destination, error)
	ListRouteSummaries(ctx context.Context) ([]RouteSummary, error)

	// Trips
	CreateTripFromExit(ctx context.Context, queueEntryID string, licensePlate string, destinationName string, seatsBooked int, totalSeats int, basePrice float64) (string, error)
}

type RepositoryImpl struct {
	db *pgxpool.Pool
}

func tunisLocation() *time.Location {
	loc, err := time.LoadLocation("Africa/Tunis")
	if err != nil {
		// Keep a safe fallback if tz database is not available in runtime.
		return time.FixedZone("Africa/Tunis", 3600)
	}
	return loc
}

func tunisDayBounds(t time.Time) (start time.Time, end time.Time) {
	loc := tunisLocation()
	tt := t.In(loc)
	start = time.Date(tt.Year(), tt.Month(), tt.Day(), 0, 0, 0, 0, loc)
	end = start.Add(24*time.Hour - time.Second)
	return start, end
}

// Keep wall-clock fields while switching location to avoid timezone shifts
// when DB timestamps are stored without timezone information.
func asTunisWall(t time.Time) time.Time {
	loc := tunisLocation()
	return time.Date(
		t.Year(), t.Month(), t.Day(),
		t.Hour(), t.Minute(), t.Second(), t.Nanosecond(),
		loc,
	)
}

func NewRepository(db *pgxpool.Pool) Repository {
	return &RepositoryImpl{db: db}
}

// ===== Routes =====

func (r *RepositoryImpl) ListRoutes(ctx context.Context, includeInactive bool) ([]Route, error) {
	q := `
		SELECT id, station_id, station_name, base_price, COALESCE(service_fee, 0.200), governorate, governorate_ar,
		       delegation, delegation_ar, is_active, updated_at
		FROM routes`
	if !includeInactive {
		q += ` WHERE is_active = true`
	}
	q += ` ORDER BY station_name ASC`
	rows, err := r.db.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Route
	for rows.Next() {
		var rt Route
		if err := rows.Scan(&rt.ID, &rt.StationID, &rt.StationName, &rt.BasePrice, &rt.ServiceFee, &rt.Governorate,
			&rt.GovernorateAr, &rt.Delegation, &rt.DelegationAr, &rt.IsActive, &rt.UpdatedAt); err != nil {
			return nil, err
		}
		list = append(list, rt)
	}
	return list, nil
}

func (r *RepositoryImpl) CreateRoute(ctx context.Context, req CreateRouteRequest) (*Route, error) {
	serviceFee := 0.2
	if req.ServiceFee != nil {
		serviceFee = *req.ServiceFee
	}
	row := r.db.QueryRow(ctx, `
		INSERT INTO routes (id, station_id, station_name, base_price, service_fee, governorate, governorate_ar, delegation, delegation_ar, is_active)
		VALUES (gen_random_uuid(), $1, $2, $3, $4, $5, $6, $7, $8, true)
		RETURNING id, station_id, station_name, base_price, COALESCE(service_fee, 0.200), governorate, governorate_ar, delegation, delegation_ar, is_active, updated_at`,
		req.StationID, req.StationName, req.BasePrice, serviceFee, req.Governorate, req.GovernorateAr, req.Delegation, req.DelegationAr)
	var rt Route
	if err := row.Scan(&rt.ID, &rt.StationID, &rt.StationName, &rt.BasePrice, &rt.ServiceFee, &rt.Governorate,
		&rt.GovernorateAr, &rt.Delegation, &rt.DelegationAr, &rt.IsActive, &rt.UpdatedAt); err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *RepositoryImpl) UpdateRoute(ctx context.Context, id string, req UpdateRouteRequest) (*Route, error) {
	// simple approach: update fields that are non-nil
	_, err := r.db.Exec(ctx, `
		UPDATE routes SET 
		  station_name = COALESCE($2, station_name),
		  base_price   = COALESCE($3, base_price),
		  service_fee  = COALESCE($4, service_fee),
		  governorate  = COALESCE($5, governorate),
		  governorate_ar = COALESCE($6, governorate_ar),
		  delegation   = COALESCE($7, delegation),
		  delegation_ar = COALESCE($8, delegation_ar),
		  is_active    = COALESCE($9, is_active),
		  updated_at   = NOW()
		WHERE id = $1`, id, req.StationName, req.BasePrice, req.ServiceFee, req.Governorate, req.GovernorateAr, req.Delegation, req.DelegationAr, req.IsActive)
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, station_id, station_name, base_price, COALESCE(service_fee, 0.200), governorate, governorate_ar,
		       delegation, delegation_ar, is_active, updated_at
		FROM routes WHERE id = $1`, id)
	var rt Route
	if err := row.Scan(&rt.ID, &rt.StationID, &rt.StationName, &rt.BasePrice, &rt.ServiceFee, &rt.Governorate,
		&rt.GovernorateAr, &rt.Delegation, &rt.DelegationAr, &rt.IsActive, &rt.UpdatedAt); err != nil {
		return nil, err
	}
	return &rt, nil
}

func (r *RepositoryImpl) DeleteRoute(ctx context.Context, id string) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM routes WHERE id = $1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("route not found")
	}
	return nil
}

// ===== Vehicles =====

func (r *RepositoryImpl) ListVehicles(ctx context.Context, searchQuery string) ([]Vehicle, error) {
	var query string
	var args []interface{}

	if searchQuery != "" {
		// Enhanced search focusing on the right part of license plate (after TUN)
		// Also supports full license plate search as fallback
		query = `
			SELECT id, license_plate, capacity, phone_number, is_active, is_available, is_banned,
			       default_destination_id, default_destination_name, available_seats, total_seats, base_price,
			       destination_id, destination_name, created_at, updated_at
			FROM vehicles 
			WHERE (
				-- Search by right part of license plate (after TUN)
				LOWER(SUBSTRING(license_plate FROM 'TUN\s*(.*)$')) LIKE LOWER($1) OR
				-- Fallback: full license plate search
				LOWER(license_plate) LIKE LOWER($2)
			)
			ORDER BY 
				CASE 
					WHEN LOWER(SUBSTRING(license_plate FROM 'TUN\s*(.*)$')) LIKE LOWER($1) THEN 1
					ELSE 2
				END,
				license_plate ASC`
		args = []interface{}{"%" + searchQuery + "%", "%" + searchQuery + "%"}
	} else {
		// Return all vehicles if no search query
		query = `
			SELECT id, license_plate, capacity, phone_number, is_active, is_available, is_banned,
			       default_destination_id, default_destination_name, available_seats, total_seats, base_price,
			       destination_id, destination_name, created_at, updated_at
			FROM vehicles 
			ORDER BY license_plate ASC`
		args = []interface{}{}
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []Vehicle
	for rows.Next() {
		var v Vehicle
		if err := rows.Scan(&v.ID, &v.LicensePlate, &v.Capacity, &v.PhoneNumber, &v.IsActive, &v.IsAvailable, &v.IsBanned,
			&v.DefaultDestID, &v.DefaultDestName, &v.AvailableSeats, &v.TotalSeats, &v.BasePrice,
			&v.DestinationID, &v.DestinationName, &v.CreatedAt, &v.UpdatedAt); err != nil {
			return nil, err
		}
		// fetch authorized stations for each vehicle
		authRows, err := r.db.Query(ctx, `SELECT id, vehicle_id, station_id, station_name, priority, is_default, created_at FROM vehicle_authorized_stations WHERE vehicle_id = $1 ORDER BY priority ASC, created_at ASC`, v.ID)
		if err == nil {
			var auths []VehicleAuthorizedStation
			for authRows.Next() {
				var as VehicleAuthorizedStation
				if err := authRows.Scan(&as.ID, &as.VehicleID, &as.StationID, &as.StationName, &as.Priority, &as.IsDefault, &as.CreatedAt); err == nil {
					auths = append(auths, as)
				}
			}
			authRows.Close()
			v.AuthorizedStations = auths
		}
		list = append(list, v)
	}
	return list, nil
}

func (r *RepositoryImpl) CreateVehicle(ctx context.Context, req CreateVehicleRequest) (*Vehicle, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO vehicles (id, license_plate, capacity, phone_number, is_active, is_available, is_banned,
		       available_seats, total_seats, base_price)
		VALUES (gen_random_uuid(), $1, $2, $3, true, true, false, $2, $2, 2.00)
		RETURNING id, license_plate, capacity, phone_number, is_active, is_available, is_banned,
		       default_destination_id, default_destination_name, available_seats, total_seats, base_price,
		       destination_id, destination_name, created_at, updated_at`, req.LicensePlate, req.Capacity, req.PhoneNumber)
	var v Vehicle
	if err := row.Scan(&v.ID, &v.LicensePlate, &v.Capacity, &v.PhoneNumber, &v.IsActive, &v.IsAvailable, &v.IsBanned,
		&v.DefaultDestID, &v.DefaultDestName, &v.AvailableSeats, &v.TotalSeats, &v.BasePrice,
		&v.DestinationID, &v.DestinationName, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *RepositoryImpl) UpdateVehicle(ctx context.Context, id string, req UpdateVehicleRequest) (*Vehicle, error) {
	_, err := r.db.Exec(ctx, `
		UPDATE vehicles SET
		  license_plate = COALESCE($2, license_plate),
		  capacity = COALESCE($3, capacity),
		  phone_number = COALESCE($4, phone_number),
		  is_active = COALESCE($5, is_active),
		  is_available = COALESCE($6, is_available),
		  is_banned = COALESCE($7, is_banned),
		  default_destination_id = COALESCE($8, default_destination_id),
		  default_destination_name = COALESCE($9, default_destination_name),
		  updated_at = NOW()
		WHERE id = $1`, id, req.LicensePlate, req.Capacity, req.PhoneNumber, req.IsActive, req.IsAvailable, req.IsBanned, req.DefaultDestID, req.DefaultDestName)
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRow(ctx, `
		SELECT id, license_plate, capacity, phone_number, is_active, is_available, is_banned,
		       default_destination_id, default_destination_name, available_seats, total_seats, base_price,
		       destination_id, destination_name, created_at, updated_at
		FROM vehicles WHERE id = $1`, id)
	var v Vehicle
	if err := row.Scan(&v.ID, &v.LicensePlate, &v.Capacity, &v.PhoneNumber, &v.IsActive, &v.IsAvailable, &v.IsBanned,
		&v.DefaultDestID, &v.DefaultDestName, &v.AvailableSeats, &v.TotalSeats, &v.BasePrice,
		&v.DestinationID, &v.DestinationName, &v.CreatedAt, &v.UpdatedAt); err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *RepositoryImpl) DeleteVehicle(ctx context.Context, id string) error {
	// Delete operational data first
	// Historical records (trips, exit_passes, day_passes) will have vehicle_id set to NULL automatically
	// via ON DELETE SET NULL foreign key constraints
	var vehicleSchedulesExists bool
	if err := r.db.QueryRow(ctx, `SELECT to_regclass('public.vehicle_schedules') IS NOT NULL`).Scan(&vehicleSchedulesExists); err != nil {
		return fmt.Errorf("failed to check vehicle schedules table: %w", err)
	}
	if vehicleSchedulesExists {
		if _, err := r.db.Exec(ctx, `DELETE FROM vehicle_schedules WHERE vehicle_id = $1`, id); err != nil {
			return fmt.Errorf("failed to delete vehicle schedules: %w", err)
		}
	}

	// Delete authorized stations
	_, err := r.db.Exec(ctx, `DELETE FROM vehicle_authorized_stations WHERE vehicle_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete authorized stations: %w", err)
	}

	// Delete queue entries
	_, err = r.db.Exec(ctx, `DELETE FROM vehicle_queue WHERE vehicle_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete queue entries: %w", err)
	}

	// Delete the vehicle - historical records will be preserved with vehicle_id = NULL
	ct, err := r.db.Exec(ctx, `DELETE FROM vehicles WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete vehicle: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("vehicle not found")
	}
	return nil
}

// ===== Authorized Routes =====

func (r *RepositoryImpl) ListAuthorizedRoutes(ctx context.Context, vehicleID string) ([]VehicleAuthorizedStation, error) {
	rows, err := r.db.Query(ctx, `
        SELECT id, vehicle_id, station_id, station_name, priority, is_default, created_at
        FROM vehicle_authorized_stations
        WHERE vehicle_id = $1
        ORDER BY priority ASC, created_at ASC`, vehicleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []VehicleAuthorizedStation
	for rows.Next() {
		var as VehicleAuthorizedStation
		if err := rows.Scan(&as.ID, &as.VehicleID, &as.StationID, &as.StationName, &as.Priority, &as.IsDefault, &as.CreatedAt); err != nil {
			return nil, err
		}
		list = append(list, as)
	}
	return list, nil
}

func (r *RepositoryImpl) AddAuthorizedRoute(ctx context.Context, vehicleID string, req AddAuthorizedRouteRequest) (*VehicleAuthorizedStation, error) {
	row := r.db.QueryRow(ctx, `
        INSERT INTO vehicle_authorized_stations (id, vehicle_id, station_id, station_name, priority, is_default)
        VALUES (gen_random_uuid(), $1, $2, $3, $4, $5)
        RETURNING id, vehicle_id, station_id, station_name, priority, is_default, created_at`,
		vehicleID, req.StationID, req.StationName, req.Priority, req.IsDefault,
	)
	var as VehicleAuthorizedStation
	if err := row.Scan(&as.ID, &as.VehicleID, &as.StationID, &as.StationName, &as.Priority, &as.IsDefault, &as.CreatedAt); err != nil {
		return nil, err
	}
	return &as, nil
}

func (r *RepositoryImpl) UpdateAuthorizedRoute(ctx context.Context, authID string, req UpdateAuthorizedRouteRequest) (*VehicleAuthorizedStation, error) {
	_, err := r.db.Exec(ctx, `
        UPDATE vehicle_authorized_stations SET
          priority = COALESCE($2, priority),
          is_default = COALESCE($3, is_default)
        WHERE id = $1`, authID, req.Priority, req.IsDefault)
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRow(ctx, `
        SELECT id, vehicle_id, station_id, station_name, priority, is_default, created_at
        FROM vehicle_authorized_stations WHERE id = $1`, authID)
	var as VehicleAuthorizedStation
	if err := row.Scan(&as.ID, &as.VehicleID, &as.StationID, &as.StationName, &as.Priority, &as.IsDefault, &as.CreatedAt); err != nil {
		return nil, err
	}
	return &as, nil
}

func (r *RepositoryImpl) DeleteAuthorizedRoute(ctx context.Context, authID string) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM vehicle_authorized_stations WHERE id = $1`, authID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("authorized route not found")
	}
	return nil
}

// ===== Queue entries =====

func (r *RepositoryImpl) ListQueue(ctx context.Context, destinationID string, subRoute *string) ([]QueueEntry, error) {
	var rows pgx.Rows
	var err error
	if subRoute != nil && *subRoute != "" {
		rows, err = r.db.Query(ctx, `
            SELECT q.id, q.vehicle_id, v.license_plate, q.destination_id, q.destination_name,
                   q.sub_route, q.sub_route_name, q.queue_type, q.queue_position,
                   CASE
                     WHEN q.available_seats = 0 THEN 'READY'
                     WHEN q.available_seats < q.total_seats THEN 'LOADING'
                     ELSE q.status
                   END AS status,
                   q.entered_at, q.available_seats, q.total_seats, q.base_price,
                   q.estimated_departure, q.actual_departure,
                   -- Calculate booked seats
                   (q.total_seats - q.available_seats) as booked_seats,
                   -- Day pass status
                   COALESCE(dp.has_pass, false) as has_day_pass,
                   CASE 
                     WHEN dp.has_pass IS NULL THEN 'no_pass'
                     WHEN dp.has_pass = true AND dp.has_trips_today = true THEN 'has_pass'
                     WHEN dp.has_pass = true AND dp.has_trips_today = false AND q.entered_at > (now() AT TIME ZONE 'Africa/Tunis' - interval '30 minutes') THEN 'recent_pass'
                     WHEN dp.has_pass = true AND dp.has_trips_today = false THEN 'has_pass'
                     ELSE 'no_pass'
                   END as day_pass_status,
                   dp.purchase_date as day_pass_purchased_at,
                   COALESCE(dp.has_trips_today, false) as has_trips_today
            FROM vehicle_queue q
            JOIN vehicles v ON v.id = q.vehicle_id
            LEFT JOIN (
                SELECT 
                    dp.vehicle_id,
                    true as has_pass,
                    dp.purchase_date,
                    EXISTS(
                        SELECT 1 FROM trips t 
                        WHERE t.vehicle_id = dp.vehicle_id 
                        AND t.start_time::date = CURRENT_DATE
                    ) as has_trips_today
                FROM day_passes dp
                WHERE dp.is_active = true 
                AND (now() AT TIME ZONE 'Africa/Tunis') BETWEEN dp.valid_from AND dp.valid_until
            ) dp ON dp.vehicle_id = q.vehicle_id
            WHERE q.destination_id = $1 AND q.sub_route = $2
            ORDER BY q.queue_position ASC`, destinationID, *subRoute)
	} else {
		rows, err = r.db.Query(ctx, `
            SELECT q.id, q.vehicle_id, v.license_plate, q.destination_id, q.destination_name,
                   q.sub_route, q.sub_route_name, q.queue_type, q.queue_position,
                   CASE
                     WHEN q.available_seats = 0 THEN 'READY'
                     WHEN q.available_seats < q.total_seats THEN 'LOADING'
                     ELSE q.status
                   END AS status,
                   q.entered_at, q.available_seats, q.total_seats, q.base_price,
                   q.estimated_departure, q.actual_departure,
                   -- Calculate booked seats
                   (q.total_seats - q.available_seats) as booked_seats,
                   -- Day pass status
                   COALESCE(dp.has_pass, false) as has_day_pass,
                   CASE 
                     WHEN dp.has_pass IS NULL THEN 'no_pass'
                     WHEN dp.has_pass = true AND dp.has_trips_today = true THEN 'has_pass'
                     WHEN dp.has_pass = true AND dp.has_trips_today = false AND q.entered_at > (now() AT TIME ZONE 'Africa/Tunis' - interval '30 minutes') THEN 'recent_pass'
                     WHEN dp.has_pass = true AND dp.has_trips_today = false THEN 'has_pass'
                     ELSE 'no_pass'
                   END as day_pass_status,
                   dp.purchase_date as day_pass_purchased_at,
                   COALESCE(dp.has_trips_today, false) as has_trips_today
            FROM vehicle_queue q
            JOIN vehicles v ON v.id = q.vehicle_id
            LEFT JOIN (
                SELECT 
                    dp.vehicle_id,
                    true as has_pass,
                    dp.purchase_date,
                    EXISTS(
                        SELECT 1 FROM trips t 
                        WHERE t.vehicle_id = dp.vehicle_id 
                        AND t.start_time::date = CURRENT_DATE
                    ) as has_trips_today
                FROM day_passes dp
                WHERE dp.is_active = true 
                AND (now() AT TIME ZONE 'Africa/Tunis') BETWEEN dp.valid_from AND dp.valid_until
            ) dp ON dp.vehicle_id = q.vehicle_id
            WHERE q.destination_id = $1
            ORDER BY q.queue_position ASC`, destinationID)
	}
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
			&e.BookedSeats, &e.HasDayPass, &e.DayPassStatus, &e.DayPassPurchasedAt, &e.HasTripsToday); err != nil {
			return nil, err
		}
		list = append(list, e)
	}
	return list, nil
}

func (r *RepositoryImpl) AddQueueEntry(ctx context.Context, req AddQueueEntryRequest) (*QueueEntry, *DayPassCreatedEvent, *DayPassCreatedEvent, string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return nil, nil, nil, "", err
	}
	defer tx.Rollback(ctx)

	// Check if vehicle is already in any queue
	var existingEntryID, existingDestinationID, existingDestinationName string
	err = tx.QueryRow(ctx, `SELECT id, destination_id, destination_name FROM vehicle_queue WHERE vehicle_id=$1`, req.VehicleID).Scan(&existingEntryID, &existingDestinationID, &existingDestinationName)
	if err == nil {
		return nil, nil, nil, "", fmt.Errorf("vehicle is already in queue for destination: %s", existingDestinationName)
	} else if err != pgx.ErrNoRows {
		return nil, nil, nil, "", err
	}

	// Compute next queue position for destination
	var nextPos int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(queue_position),0)+1 FROM vehicle_queue WHERE destination_id=$1`, req.DestinationID).Scan(&nextPos); err != nil {
		return nil, nil, nil, "", err
	}

	// Get vehicle license plate
	var lp string
	if err := tx.QueryRow(ctx, `SELECT license_plate FROM vehicles WHERE id=$1`, req.VehicleID).Scan(&lp); err != nil {
		return nil, nil, nil, "", err
	}

	// Check for existing valid day pass for this vehicle in Africa/Tunis timezone
	var existingDayPass *DayPassCreatedEvent
	var dayPassStatus string

	var dayPassID, vehicleID, licensePlate string
	var price float64
	var purchaseDate, validFrom, validUntil time.Time
	var createdBy string

	err = tx.QueryRow(ctx, `
		SELECT dp.id, dp.vehicle_id, dp.license_plate, dp.price, dp.purchase_date, 
		       dp.valid_from, dp.valid_until, dp.created_by
		FROM day_passes dp
		WHERE dp.vehicle_id = $1
		  AND dp.is_active = true
		  AND (now() AT TIME ZONE 'Africa/Tunis') BETWEEN dp.valid_from AND dp.valid_until
		ORDER BY dp.purchase_date DESC
		LIMIT 1`, req.VehicleID).Scan(
		&dayPassID, &vehicleID, &licensePlate, &price,
		&purchaseDate, &validFrom, &validUntil, &createdBy)

	if err != nil && err != pgx.ErrNoRows {
		return nil, nil, nil, "", err
	}

	var dayPassEvent *DayPassCreatedEvent
	if err == pgx.ErrNoRows {
		// No existing day pass, create a new one
		dayPassStatus = "created"

		// Generate day pass ID
		var newDayPassID string
		if err := tx.QueryRow(ctx, `SELECT substr(md5(random()::text || clock_timestamp()::text),1,24)`).Scan(&newDayPassID); err != nil {
			return nil, nil, nil, "", err
		}

		_, err := tx.Exec(ctx, `
            INSERT INTO day_passes (
                id, vehicle_id, license_plate, price, purchase_date,
                valid_from, valid_until, is_active, created_by, created_at, updated_at
            ) VALUES (
                $1, $2, $3, 2.0,
                (now() AT TIME ZONE 'Africa/Tunis'),
                date_trunc('day', (now() AT TIME ZONE 'Africa/Tunis')),
                (date_trunc('day', (now() AT TIME ZONE 'Africa/Tunis')) + interval '1 day' - interval '1 second'),
                true, $4,
                (now() AT TIME ZONE 'Africa/Tunis'),
                (now() AT TIME ZONE 'Africa/Tunis')
            )`, newDayPassID, req.VehicleID, lp, req.CreatedBy)
		if err != nil {
			return nil, nil, nil, "", err
		}

		// Create day pass event for WebSocket broadcasting
		nowTunis := time.Now().In(tunisLocation())
		startOfDay, endOfDay := tunisDayBounds(nowTunis)
		dayPassEvent = &DayPassCreatedEvent{
			DayPassID:       newDayPassID,
			VehicleID:       req.VehicleID,
			LicensePlate:    lp,
			DestinationID:   req.DestinationID,
			DestinationName: req.DestinationName,
			Price:           2.0,
			PurchaseDate:    nowTunis,
			ValidFrom:       startOfDay,
			ValidUntil:      endOfDay,
			CreatedBy:       req.CreatedBy,
		}
	} else {
		// Existing day pass found
		dayPassStatus = "valid"
		existingDayPass = &DayPassCreatedEvent{
			DayPassID:       dayPassID,
			VehicleID:       vehicleID,
			LicensePlate:    licensePlate,
			DestinationID:   req.DestinationID,
			DestinationName: req.DestinationName,
			Price:           price,
			PurchaseDate:    asTunisWall(purchaseDate),
			ValidFrom:       asTunisWall(validFrom),
			ValidUntil:      asTunisWall(validUntil),
			CreatedBy:       createdBy,
		}
	}

	row := tx.QueryRow(ctx, `
        INSERT INTO vehicle_queue (id, vehicle_id, destination_id, destination_name, sub_route, sub_route_name,
            queue_type, queue_position, status, entered_at, available_seats, total_seats, base_price)
        SELECT gen_random_uuid(), $1, $2, $3, $4, $5,
               COALESCE($6,'REGULAR'), $7, 'WAITING', now(), v.available_seats, v.total_seats, 
               COALESCE(r.base_price, v.base_price)
        FROM vehicles v 
        LEFT JOIN routes r ON r.station_id = $2
        WHERE v.id = $1
        RETURNING id, vehicle_id, destination_id, destination_name, sub_route, sub_route_name,
               queue_type, queue_position, status, entered_at,
               available_seats, total_seats, base_price, estimated_departure, actual_departure`,
		req.VehicleID, req.DestinationID, req.DestinationName, req.SubRoute, req.SubRouteName, req.QueueType, nextPos)

	var e QueueEntry
	if err := row.Scan(&e.ID, &e.VehicleID, &e.DestinationID, &e.DestinationName, &e.SubRoute, &e.SubRouteName,
		&e.QueueType, &e.QueuePosition, &e.Status, &e.EnteredAt, &e.AvailableSeats, &e.TotalSeats, &e.BasePrice, &e.EstimatedDeparture, &e.ActualDeparture); err != nil {
		return nil, nil, nil, "", err
	}
	if err := tx.QueryRow(ctx, `SELECT license_plate FROM vehicles WHERE id=$1`, e.VehicleID).Scan(&e.LicensePlate); err != nil {
		return nil, nil, nil, "", err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, nil, nil, "", err
	}
	return &e, dayPassEvent, existingDayPass, dayPassStatus, nil
}

func (r *RepositoryImpl) GetVehicleDayPass(ctx context.Context, vehicleID string) (*DayPassCreatedEvent, error) {
	var dayPass DayPassCreatedEvent
	var licensePlate string

	// Get vehicle license plate
	if err := r.db.QueryRow(ctx, `SELECT license_plate FROM vehicles WHERE id=$1`, vehicleID).Scan(&licensePlate); err != nil {
		return nil, err
	}

	// Get current valid day pass for the vehicle
	err := r.db.QueryRow(ctx, `
		SELECT dp.id, dp.vehicle_id, dp.license_plate, dp.price, dp.purchase_date, 
		       dp.valid_from, dp.valid_until, dp.created_by
		FROM day_passes dp
		WHERE dp.vehicle_id = $1
		  AND dp.is_active = true
		  AND (now() AT TIME ZONE 'Africa/Tunis') BETWEEN dp.valid_from AND dp.valid_until
		ORDER BY dp.purchase_date DESC
		LIMIT 1`, vehicleID).Scan(
		&dayPass.DayPassID, &dayPass.VehicleID, &dayPass.LicensePlate, &dayPass.Price,
		&dayPass.PurchaseDate, &dayPass.ValidFrom, &dayPass.ValidUntil, &dayPass.CreatedBy)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, nil // No day pass found
		}
		return nil, err
	}

	// Get destination info from current queue entry
	var destinationID, destinationName string
	err = r.db.QueryRow(ctx, `
		SELECT destination_id, destination_name 
		FROM vehicle_queue 
		WHERE vehicle_id = $1 
		ORDER BY entered_at DESC 
		LIMIT 1`, vehicleID).Scan(&destinationID, &destinationName)

	if err != nil {
		// If no queue entry found, use default values
		destinationID = "unknown"
		destinationName = "Unknown Destination"
	}

	dayPass.DestinationID = destinationID
	dayPass.DestinationName = destinationName

	// Preserve stored wall-clock values and attach Tunisia location.
	dayPass.PurchaseDate = asTunisWall(dayPass.PurchaseDate)
	dayPass.ValidFrom = asTunisWall(dayPass.ValidFrom)
	dayPass.ValidUntil = asTunisWall(dayPass.ValidUntil)

	return &dayPass, nil
}

func (r *RepositoryImpl) UpdateQueueEntry(ctx context.Context, id string, req UpdateQueueEntryRequest) (*QueueEntry, error) {
	_, err := r.db.Exec(ctx, `
        UPDATE vehicle_queue SET
          status = COALESCE($2, status),
          available_seats = COALESCE($3, available_seats),
          estimated_departure = COALESCE($4, estimated_departure),
          sub_route = COALESCE($5, sub_route),
          sub_route_name = COALESCE($6, sub_route_name)
        WHERE id=$1`, id, req.Status, req.AvailableSeats, req.EstimatedDeparture, req.SubRoute, req.SubRouteName)
	if err != nil {
		return nil, err
	}

	row := r.db.QueryRow(ctx, `
        SELECT q.id, q.vehicle_id, v.license_plate, q.destination_id, q.destination_name,
               q.sub_route, q.sub_route_name, q.queue_type, q.queue_position, q.status,
               q.entered_at, q.available_seats, q.total_seats, q.base_price,
               q.estimated_departure, q.actual_departure
        FROM vehicle_queue q JOIN vehicles v ON v.id=q.vehicle_id
        WHERE q.id=$1`, id)
	var e QueueEntry
	if err := row.Scan(&e.ID, &e.VehicleID, &e.LicensePlate, &e.DestinationID, &e.DestinationName,
		&e.SubRoute, &e.SubRouteName, &e.QueueType, &e.QueuePosition, &e.Status,
		&e.EnteredAt, &e.AvailableSeats, &e.TotalSeats, &e.BasePrice, &e.EstimatedDeparture, &e.ActualDeparture); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *RepositoryImpl) DeleteQueueEntry(ctx context.Context, id string) error {
	ct, err := r.db.Exec(ctx, `DELETE FROM vehicle_queue WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("queue entry not found")
	}
	return nil
}

func (r *RepositoryImpl) ClearQueue(ctx context.Context, destinationID string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM vehicle_queue WHERE destination_id=$1`, destinationID)
	if err != nil {
		return err
	}
	return nil
}

func (r *RepositoryImpl) ClearAllQueues(ctx context.Context) error {
	_, err := r.db.Exec(ctx, `DELETE FROM vehicle_queue`)
	if err != nil {
		return err
	}
	return nil
}

// CreateTripFromExit inserts a trip row when a vehicle exits via print&remove
func (r *RepositoryImpl) CreateTripFromExit(ctx context.Context, queueEntryID string, licensePlate string, destinationName string, seatsBooked int, totalSeats int, basePrice float64) (string, error) {
	tripID := fmt.Sprintf("trip_%d", time.Now().UnixNano())
	// Look up queue entry details to populate vehicle_id and destination_id
	var vehicleID, destinationID string
	if err := r.db.QueryRow(ctx, `SELECT vehicle_id, destination_id FROM vehicle_queue WHERE id=$1`, queueEntryID).Scan(&vehicleID, &destinationID); err != nil {
		return "", err
	}
	// Insert into trips
	if _, err := r.db.Exec(ctx, `
        INSERT INTO trips (
            id, vehicle_id, license_plate, destination_id, destination_name, queue_id, seats_booked,
            vehicle_capacity, base_price, start_time, created_at
        ) VALUES (
            $1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), NOW()
        )`, tripID, vehicleID, licensePlate, destinationID, destinationName, queueEntryID, seatsBooked, totalSeats, basePrice); err != nil {
		return "", err
	}
	return tripID, nil
}

func (r *RepositoryImpl) ReorderQueue(ctx context.Context, destinationID string, entryIDs []string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// lock rows for destination
	if _, err := tx.Exec(ctx, `SELECT id FROM vehicle_queue WHERE destination_id=$1 FOR UPDATE`, destinationID); err != nil {
		return err
	}
	for i, id := range entryIDs {
		if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET queue_position=$2 WHERE id=$1 AND destination_id=$3`, id, i+1, destinationID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *RepositoryImpl) MoveEntry(ctx context.Context, entryID string, destinationID string, newPos int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// lock destination
	if _, err := tx.Exec(ctx, `SELECT id FROM vehicle_queue WHERE destination_id=$1 FOR UPDATE`, destinationID); err != nil {
		return err
	}

	// get current position
	var cur int
	if err := tx.QueryRow(ctx, `SELECT queue_position FROM vehicle_queue WHERE id=$1 AND destination_id=$2`, entryID, destinationID).Scan(&cur); err != nil {
		return err
	}

	if newPos == cur {
		return tx.Commit(ctx)
	}
	if newPos < cur {
		// shift down others between newPos..cur-1 up by +1
		if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET queue_position = queue_position+1 WHERE destination_id=$1 AND queue_position >= $2 AND queue_position < $3`, destinationID, newPos, cur); err != nil {
			return err
		}
	} else {
		// shift up others between cur+1..newPos down by -1
		if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET queue_position = queue_position-1 WHERE destination_id=$1 AND queue_position > $2 AND queue_position <= $3`, destinationID, cur, newPos); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET queue_position=$2 WHERE id=$1`, entryID, newPos); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *RepositoryImpl) TransferSeats(ctx context.Context, fromEntryID, toEntryID string, seats int) error {
	if seats <= 0 {
		return fmt.Errorf("seats must be > 0")
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	var fromAvail, fromTotal, toAvail, toTotal int
	if err := tx.QueryRow(ctx, `SELECT available_seats, total_seats FROM vehicle_queue WHERE id=$1 FOR UPDATE`, fromEntryID).Scan(&fromAvail, &fromTotal); err != nil {
		return err
	}
	if err := tx.QueryRow(ctx, `SELECT available_seats, total_seats FROM vehicle_queue WHERE id=$1 FOR UPDATE`, toEntryID).Scan(&toAvail, &toTotal); err != nil {
		return err
	}

	// Calculate booked seats
	fromBooked := fromTotal - fromAvail
	toBooked := toTotal - toAvail

	// Check if source vehicle has enough booked seats to transfer
	if fromBooked < seats {
		return fmt.Errorf("source vehicle does not have enough booked seats to transfer")
	}

	// Check if target vehicle can accommodate the additional passengers
	if toBooked+seats > toTotal {
		return fmt.Errorf("target vehicle cannot accommodate additional passengers (would exceed total capacity)")
	}

	// Transfer booked seats: passengers move from source to target
	// Source: reduce booked seats (increase available seats)
	// Target: increase booked seats (decrease available seats)
	if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET available_seats = available_seats + $2 WHERE id=$1`, fromEntryID, seats); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET available_seats = available_seats - $2 WHERE id=$1`, toEntryID, seats); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *RepositoryImpl) ChangeDestination(ctx context.Context, entryID, newDestID, newDestName string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// get current destination
	var oldDest string
	if err := tx.QueryRow(ctx, `SELECT destination_id FROM vehicle_queue WHERE id=$1 FOR UPDATE`, entryID).Scan(&oldDest); err != nil {
		return err
	}

	// compute next position in new destination
	var nextPos int
	if err := tx.QueryRow(ctx, `SELECT COALESCE(MAX(queue_position),0)+1 FROM vehicle_queue WHERE destination_id=$1`, newDestID).Scan(&nextPos); err != nil {
		return err
	}

	// update entry
	if _, err := tx.Exec(ctx, `UPDATE vehicle_queue SET destination_id=$2, destination_name=$3, queue_position=$4 WHERE id=$1`, entryID, newDestID, newDestName, nextPos); err != nil {
		return err
	}

	// re-sequence old destination positions
	if _, err := tx.Exec(ctx, `WITH ranked AS (
        SELECT id, ROW_NUMBER() OVER(ORDER BY queue_position ASC) AS rn
        FROM vehicle_queue WHERE destination_id=$1
    ) UPDATE vehicle_queue v SET queue_position = r.rn FROM ranked r WHERE v.id=r.id`, oldDest); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *RepositoryImpl) ListDayPasses(ctx context.Context, limit int) ([]DayPass, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, vehicle_id, license_plate, price, purchase_date, valid_from, valid_until, is_active, false AS is_expired, created_by
        FROM day_passes ORDER BY purchase_date DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []DayPass
	for rows.Next() {
		var d DayPass
		if err := rows.Scan(&d.ID, &d.VehicleID, &d.LicensePlate, &d.Price, &d.PurchaseDate, &d.ValidFrom, &d.ValidUntil, &d.IsActive, &d.IsExpired, &d.CreatedBy); err != nil {
			return nil, err
		}
		list = append(list, d)
	}
	return list, nil
}

func (r *RepositoryImpl) ExpireDayPassesAndClearRegularQueues(ctx context.Context) (expiredPasses int64, clearedQueueEntries int64, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback(ctx)

	// Mark expired day passes first.
	var hasIsExpired bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public'
			  AND table_name = 'day_passes'
			  AND column_name = 'is_expired'
		)`).Scan(&hasIsExpired); err != nil {
		return 0, 0, err
	}

	var dayPassCT pgconn.CommandTag
	if hasIsExpired {
		dayPassCT, err = tx.Exec(ctx, `
			UPDATE day_passes
			SET is_active = false,
			    is_expired = true,
			    updated_at = (now() AT TIME ZONE 'Africa/Tunis')
			WHERE is_active = true
			  AND (is_expired = false OR is_expired IS NULL)
			  AND valid_until < (now() AT TIME ZONE 'Africa/Tunis')`)
	} else {
		dayPassCT, err = tx.Exec(ctx, `
			UPDATE day_passes
			SET is_active = false,
			    updated_at = (now() AT TIME ZONE 'Africa/Tunis')
			WHERE is_active = true
			  AND valid_until < (now() AT TIME ZONE 'Africa/Tunis')`)
	}
	if err != nil {
		return 0, 0, err
	}

	// Business rule: when day passes are invalidated at midnight, clear regular queues too.
	queueCT, err := tx.Exec(ctx, `
		DELETE FROM vehicle_queue
		WHERE UPPER(COALESCE(queue_type, 'REGULAR')) = 'REGULAR'`)
	if err != nil {
		return 0, 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, 0, err
	}
	return dayPassCT.RowsAffected(), queueCT.RowsAffected(), nil
}

// ListQueueSummaries returns aggregated per-destination stats from vehicle_queue table
func (r *RepositoryImpl) ListQueueSummaries(ctx context.Context, station string) ([]QueueSummary, error) {
	// Define station-to-destination mapping
	stationDestinations := map[string][]string{
		"jammel":         {"JEMMAL"},
		"moknin-tboulba": {"MOKNIN", "TEBOULBA"},
		"kasra-hlele":    {"KSAR HLEL"},
		"all":            {"JEMMAL", "MOKNIN", "TEBOULBA", "KSAR HLEL"},
	}

	var query string
	var args []interface{}

	if station == "" || station == "all" {
		// No filtering - return all destinations
		query = `
			SELECT q.destination_id,
			       COALESCE(r.station_name, q.destination_name) as destination_name,
			       COUNT(*) as total_vehicles,
			       COALESCE(SUM(q.total_seats),0) as total_seats,
			       COALESCE(SUM(q.available_seats),0) as available_seats,
			       COALESCE(r.base_price, 0) as base_price
			FROM vehicle_queue q
			LEFT JOIN routes r ON r.station_id = q.destination_id
			GROUP BY q.destination_id, r.station_name, q.destination_name, r.base_price
			ORDER BY COALESCE(r.station_name, q.destination_name) ASC`
	} else {
		// Filter by station destinations
		destinations, exists := stationDestinations[station]
		if !exists {
			// Unknown station, return empty result
			return []QueueSummary{}, nil
		}

		// Build IN clause for destinations
		placeholders := make([]string, len(destinations))
		for i, dest := range destinations {
			placeholders[i] = fmt.Sprintf("$%d", i+1)
			args = append(args, dest)
		}

		query = fmt.Sprintf(`
			SELECT q.destination_id,
			       COALESCE(r.station_name, q.destination_name) as destination_name,
			       COUNT(*) as total_vehicles,
			       COALESCE(SUM(q.total_seats),0) as total_seats,
			       COALESCE(SUM(q.available_seats),0) as available_seats,
			       COALESCE(r.base_price, 0) as base_price
			FROM vehicle_queue q
			LEFT JOIN routes r ON r.station_id = q.destination_id
			WHERE q.destination_name IN (%s)
			GROUP BY q.destination_id, r.station_name, q.destination_name, r.base_price
			ORDER BY COALESCE(r.station_name, q.destination_name) ASC`, strings.Join(placeholders, ","))
	}

	rows, err := r.db.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var list []QueueSummary
	for rows.Next() {
		var s QueueSummary
		if err := rows.Scan(&s.DestinationID, &s.DestinationName, &s.TotalVehicles, &s.TotalSeats, &s.AvailableSeats, &s.BasePrice); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}

func (r *RepositoryImpl) ListAllDestinations(ctx context.Context) ([]Destination, error) {
	rows, err := r.db.Query(ctx, `
		SELECT station_id, station_name, base_price, COALESCE(service_fee, 0.200), is_active
		FROM routes
		WHERE is_active = true
		ORDER BY station_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var destinations []Destination
	for rows.Next() {
		var dest Destination
		if err := rows.Scan(&dest.ID, &dest.Name, &dest.BasePrice, &dest.ServiceFee, &dest.IsActive); err != nil {
			return nil, err
		}
		destinations = append(destinations, dest)
	}
	return destinations, nil
}

// ListRouteSummaries joins active routes with queue aggregation
func (r *RepositoryImpl) ListRouteSummaries(ctx context.Context) ([]RouteSummary, error) {
	rows, err := r.db.Query(ctx, `
        WITH q AS (
            SELECT destination_id, destination_name,
                   COUNT(*) AS total_vehicles,
                   COALESCE(SUM(total_seats),0) AS total_seats,
                   COALESCE(SUM(available_seats),0) AS available_seats
            FROM vehicle_queue
            GROUP BY destination_id, destination_name
        )
        SELECT rt.id AS route_id,
               rt.station_name AS route_name,
               COALESCE(q.total_vehicles,0) AS total_vehicles,
               COALESCE(q.total_seats,0) AS total_seats,
               COALESCE(q.available_seats,0) AS available_seats
        FROM routes rt
        LEFT JOIN q ON q.destination_id = rt.id
        WHERE rt.is_active = true
        ORDER BY rt.station_name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []RouteSummary
	for rows.Next() {
		var s RouteSummary
		if err := rows.Scan(&s.RouteID, &s.RouteName, &s.TotalVehicles, &s.TotalSeats, &s.AvailableSeats); err != nil {
			return nil, err
		}
		list = append(list, s)
	}
	return list, nil
}
