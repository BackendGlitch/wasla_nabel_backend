-- Normalize trips table across legacy/current station schemas.
-- Safe to run repeatedly.

ALTER TABLE trips
  ADD COLUMN IF NOT EXISTS vehicle_id TEXT;

ALTER TABLE trips
  ADD COLUMN IF NOT EXISTS seats_booked INTEGER;

ALTER TABLE trips
  ADD COLUMN IF NOT EXISTS vehicle_capacity INTEGER;

ALTER TABLE trips
  ADD COLUMN IF NOT EXISTS base_price DOUBLE PRECISION;

-- Backfill seats_booked from legacy booked_seats when available.
UPDATE trips
SET seats_booked = COALESCE(seats_booked, booked_seats)
WHERE seats_booked IS NULL;

-- Keep not-null semantics expected by newer code paths.
UPDATE trips
SET seats_booked = 0
WHERE seats_booked IS NULL;

-- Backfill vehicle_id from queue rows when possible.
UPDATE trips t
SET vehicle_id = q.vehicle_id
FROM vehicle_queue q
WHERE t.vehicle_id IS NULL
  AND t.queue_id IS NOT NULL
  AND q.id = t.queue_id;

-- Backfill capacity/base price from queue/routes where possible.
UPDATE trips t
SET vehicle_capacity = q.total_seats
FROM vehicle_queue q
WHERE t.vehicle_capacity IS NULL
  AND t.queue_id IS NOT NULL
  AND q.id = t.queue_id;

UPDATE trips t
SET base_price = COALESCE(r.base_price, q.base_price)
FROM vehicle_queue q
LEFT JOIN routes r ON r.station_id = q.destination_id
WHERE t.base_price IS NULL
  AND t.queue_id IS NOT NULL
  AND q.id = t.queue_id;

-- Optional performance helpers for current access patterns.
CREATE INDEX IF NOT EXISTS idx_trips_vehicle_id ON trips(vehicle_id);
CREATE INDEX IF NOT EXISTS idx_trips_start_time ON trips(start_time DESC);
