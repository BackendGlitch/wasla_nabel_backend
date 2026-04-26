-- Performance indexes for on-prem station workload

-- Bookings: common filters by destination + day + status
CREATE INDEX IF NOT EXISTS idx_bookings_destination_created_at
ON bookings (destination_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_bookings_queue_id_status
ON bookings (queue_id, booking_status);

-- Ghost booking daily allocations and counts
CREATE INDEX IF NOT EXISTS idx_bookings_ghost_destination_day
ON bookings (destination_id, created_at)
WHERE is_ghost_booking = true;

-- Vehicle queue: destination ordering
CREATE INDEX IF NOT EXISTS idx_vehicle_queue_destination_position
ON vehicle_queue (destination_id, queue_position);

-- Trips: lookups by day/destination/license
CREATE INDEX IF NOT EXISTS idx_trips_start_date_destination
ON trips ((start_time::date), destination_id);

CREATE INDEX IF NOT EXISTS idx_trips_start_date_license
ON trips ((start_time::date), license_plate);

