-- Legacy schema compatibility pack.
-- Goal: avoid runtime SQL errors when older DBs miss columns/tables expected by current services.
-- Safe to run multiple times.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------------------
-- Core reference table used by statistics queries
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS stations (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  station_id VARCHAR(50) NOT NULL UNIQUE,
  station_name VARCHAR(100) NOT NULL,
  governorate VARCHAR(100),
  delegation VARCHAR(100),
  address TEXT,
  opening_time VARCHAR(10) DEFAULT '06:00',
  closing_time VARCHAR(10) DEFAULT '22:00',
  is_operational BOOLEAN DEFAULT true,
  service_fee NUMERIC(10,3) DEFAULT 0.200,
  created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

INSERT INTO stations (station_id, station_name, governorate, delegation, address, opening_time, closing_time, is_operational, service_fee)
SELECT sc.station_id, sc.station_name, sc.governorate, sc.delegation, sc.address, sc.opening_time, sc.closing_time, sc.is_operational, COALESCE(sc.service_fee, 0.200)
FROM station_config sc
WHERE NOT EXISTS (
  SELECT 1 FROM stations s WHERE s.station_id = sc.station_id
);

-- ---------------------------------------------------------------------------
-- Statistics aggregate tables referenced by repository queries
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS staff_daily_statistics (
  staff_id TEXT NOT NULL,
  date DATE NOT NULL,
  total_seats_booked INTEGER NOT NULL DEFAULT 0,
  total_seat_income NUMERIC(10,2) NOT NULL DEFAULT 0,
  total_day_passes_sold INTEGER NOT NULL DEFAULT 0,
  total_day_pass_income NUMERIC(10,2) NOT NULL DEFAULT 0,
  total_income NUMERIC(10,2) NOT NULL DEFAULT 0,
  total_transactions INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (staff_id, date)
);

CREATE TABLE IF NOT EXISTS station_daily_statistics (
  station_id UUID NOT NULL,
  date DATE NOT NULL,
  total_seats_booked INTEGER NOT NULL DEFAULT 0,
  total_seat_income NUMERIC(10,2) NOT NULL DEFAULT 0,
  total_day_passes_sold INTEGER NOT NULL DEFAULT 0,
  total_day_pass_income NUMERIC(10,2) NOT NULL DEFAULT 0,
  total_income NUMERIC(10,2) NOT NULL DEFAULT 0,
  total_transactions INTEGER NOT NULL DEFAULT 0,
  active_staff_count INTEGER NOT NULL DEFAULT 0,
  created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW(),
  PRIMARY KEY (station_id, date)
);

-- ---------------------------------------------------------------------------
-- day_passes compatibility (old/new code paths)
-- ---------------------------------------------------------------------------
ALTER TABLE day_passes ADD COLUMN IF NOT EXISTS is_expired BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE day_passes ADD COLUMN IF NOT EXISTS created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW();
ALTER TABLE day_passes ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW();

-- ---------------------------------------------------------------------------
-- vehicles compatibility (columns used in queue/booking code)
-- ---------------------------------------------------------------------------
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS available_seats INTEGER DEFAULT 8;
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS total_seats INTEGER DEFAULT 8;
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS base_price NUMERIC(10,2) DEFAULT 2.00;
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS destination_id TEXT;
ALTER TABLE vehicles ADD COLUMN IF NOT EXISTS destination_name TEXT;

UPDATE vehicles
SET available_seats = COALESCE(available_seats, capacity, 8),
    total_seats = COALESCE(total_seats, capacity, 8)
WHERE available_seats IS NULL OR total_seats IS NULL;

-- ---------------------------------------------------------------------------
-- vehicle_queue compatibility
-- ---------------------------------------------------------------------------
ALTER TABLE vehicle_queue ADD COLUMN IF NOT EXISTS queue_type TEXT DEFAULT 'REGULAR';
ALTER TABLE vehicle_queue ADD COLUMN IF NOT EXISTS booked_seats INTEGER DEFAULT 0;

-- ---------------------------------------------------------------------------
-- bookings compatibility (fields used by booking/statistics logic)
-- ---------------------------------------------------------------------------
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS destination_id TEXT;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS is_ghost_booking BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS idempotency_key TEXT;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS seat_number INTEGER;
ALTER TABLE bookings ADD COLUMN IF NOT EXISTS created_by TEXT;

-- ---------------------------------------------------------------------------
-- vehicle authorized stations naming compatibility
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS vehicle_authorized_stations (
  id TEXT PRIMARY KEY,
  vehicle_id TEXT NOT NULL,
  station_id TEXT NOT NULL,
  station_name TEXT,
  priority INTEGER NOT NULL DEFAULT 1,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMP WITHOUT TIME ZONE DEFAULT NOW()
);

DO $$
BEGIN
  IF EXISTS (
    SELECT 1
    FROM information_schema.tables
    WHERE table_schema = 'public' AND table_name = 'vehicle_authorized_routes'
  ) THEN
    EXECUTE '
      INSERT INTO vehicle_authorized_stations (id, vehicle_id, station_id, station_name, priority, is_default, created_at)
      SELECT var.id, var.vehicle_id, var.station_id, var.station_name, COALESCE(var.priority, 1), COALESCE(var.is_default, false), COALESCE(var.created_at, NOW())
      FROM vehicle_authorized_routes var
      WHERE NOT EXISTS (
        SELECT 1 FROM vehicle_authorized_stations vas WHERE vas.id = var.id
      )
    ';
  END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS vehicle_authorized_stations_vehicle_id_station_id_key
ON vehicle_authorized_stations (vehicle_id, station_id);

-- ---------------------------------------------------------------------------
-- vehicle schedules table expected by some delete flows
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS vehicle_schedules (
  id TEXT PRIMARY KEY,
  vehicle_id TEXT NOT NULL,
  route_id TEXT NOT NULL,
  departure_time TIMESTAMP WITHOUT TIME ZONE NOT NULL,
  available_seats INTEGER NOT NULL,
  total_seats INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'SCHEDULED',
  actual_departure TIMESTAMP WITHOUT TIME ZONE
);

