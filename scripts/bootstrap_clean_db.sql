-- Wasla backend clean bootstrap (pre-migration baseline)
-- ------------------------------------------------------
-- Purpose:
-- 1) Reset DB to a clean state.
-- 2) Create the baseline tables required before migration 009+.
-- 3) Seed exactly one supervisor user and one test station.
--
-- Usage:
--   psql "$DATABASE_URL" -f scripts/bootstrap_clean_db.sql
--   bash scripts/apply-migrations.sh

DROP SCHEMA IF EXISTS public CASCADE;
CREATE SCHEMA public;

-- Keep default privileges simple for now.
GRANT ALL ON SCHEMA public TO PUBLIC;

-- ---------------------------------------------------------------------------
-- Core auth table (required by auth service + migration 009 index)
-- ---------------------------------------------------------------------------
CREATE TABLE staff (
  id TEXT PRIMARY KEY,
  cin TEXT NOT NULL UNIQUE,
  phone_number TEXT NOT NULL DEFAULT '',
  first_name TEXT NOT NULL,
  last_name TEXT NOT NULL,
  role TEXT NOT NULL CHECK (role IN ('WORKER', 'SUPERVISOR')),
  is_active BOOLEAN NOT NULL DEFAULT true,
  last_login TIMESTAMP WITHOUT TIME ZONE NULL,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_staff_active ON staff (is_active);

-- ---------------------------------------------------------------------------
-- Station/runtime config consumed by /api/v1/init and public-service /info
-- ---------------------------------------------------------------------------
CREATE TABLE station_config (
  id TEXT PRIMARY KEY,
  station_id TEXT NOT NULL UNIQUE,
  station_name TEXT NOT NULL,
  governorate TEXT NOT NULL DEFAULT '',
  delegation TEXT NOT NULL DEFAULT '',
  address TEXT NOT NULL DEFAULT '',
  opening_time TEXT NOT NULL DEFAULT '05:00',
  closing_time TEXT NOT NULL DEFAULT '23:00',
  is_operational BOOLEAN NOT NULL DEFAULT true,
  service_fee NUMERIC(10,2) NOT NULL DEFAULT 0,
  company_name TEXT NOT NULL DEFAULT '',
  company_logo_url TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- Routes table (queue/public init reads from this table)
-- ---------------------------------------------------------------------------
CREATE TABLE routes (
  id TEXT PRIMARY KEY,
  station_id TEXT NOT NULL,
  station_name TEXT NOT NULL,
  base_price NUMERIC(10,2) NOT NULL DEFAULT 0,
  governorate TEXT NULL,
  governorate_ar TEXT NULL,
  delegation TEXT NULL,
  delegation_ar TEXT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_routes_is_active_name ON routes (is_active, station_name);

-- ---------------------------------------------------------------------------
-- Vehicle + queue + booking + trips baseline tables
-- These exist before migrations 009+ and are extended by later migrations.
-- ---------------------------------------------------------------------------
CREATE TABLE vehicles (
  id TEXT PRIMARY KEY,
  license_plate TEXT NOT NULL UNIQUE,
  capacity INTEGER NOT NULL CHECK (capacity > 0),
  phone_number TEXT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  is_available BOOLEAN NOT NULL DEFAULT true,
  is_banned BOOLEAN NOT NULL DEFAULT false,
  default_destination_id TEXT NULL,
  default_destination_name TEXT NULL,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE TABLE vehicle_authorized_routes (
  id TEXT PRIMARY KEY,
  vehicle_id TEXT NOT NULL,
  station_id TEXT NOT NULL,
  station_name TEXT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_vehicle_authorized_routes_vehicle ON vehicle_authorized_routes (vehicle_id);
CREATE INDEX idx_vehicle_authorized_routes_station ON vehicle_authorized_routes (station_id);

CREATE TABLE vehicle_queue (
  id TEXT PRIMARY KEY,
  vehicle_id TEXT NOT NULL,
  license_plate TEXT NOT NULL,
  destination_id TEXT NOT NULL,
  destination_name TEXT NOT NULL,
  sub_route TEXT NULL,
  sub_route_name TEXT NULL,
  queue_type TEXT NOT NULL DEFAULT 'REGULAR',
  queue_position INTEGER NOT NULL DEFAULT 1,
  status TEXT NOT NULL DEFAULT 'WAITING',
  entered_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  available_seats INTEGER NOT NULL DEFAULT 0,
  total_seats INTEGER NOT NULL DEFAULT 0,
  booked_seats INTEGER NOT NULL DEFAULT 0,
  base_price NUMERIC(10,2) NOT NULL DEFAULT 0,
  estimated_departure TIMESTAMP WITHOUT TIME ZONE NULL,
  actual_departure TIMESTAMP WITHOUT TIME ZONE NULL,
  created_by TEXT NULL
);

CREATE INDEX idx_vehicle_queue_destination ON vehicle_queue (destination_id);
CREATE INDEX idx_vehicle_queue_vehicle_id ON vehicle_queue (vehicle_id);

CREATE TABLE bookings (
  id TEXT PRIMARY KEY,
  queue_id TEXT NULL,
  destination_id TEXT NULL,
  seats_booked INTEGER NOT NULL CHECK (seats_booked > 0),
  total_amount NUMERIC(10,2) NOT NULL DEFAULT 0,
  booking_source TEXT NOT NULL DEFAULT 'STAFF',
  booking_type TEXT NOT NULL DEFAULT 'OFFLINE',
  booking_status TEXT NOT NULL DEFAULT 'ACTIVE',
  payment_status TEXT NOT NULL DEFAULT 'UNPAID',
  payment_method TEXT NOT NULL DEFAULT 'CASH',
  verification_code TEXT NOT NULL UNIQUE,
  is_verified BOOLEAN NOT NULL DEFAULT false,
  verified_at TIMESTAMP WITHOUT TIME ZONE NULL,
  verified_by_id TEXT NULL,
  created_by TEXT NULL,
  cancelled_at TIMESTAMP WITHOUT TIME ZONE NULL,
  cancelled_by TEXT NULL,
  cancellation_reason TEXT NULL,
  refund_amount NUMERIC(10,2) NULL,
  is_ghost_booking BOOLEAN NOT NULL DEFAULT false,
  idempotency_key TEXT NULL,
  seat_number INTEGER NULL,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_bookings_queue ON bookings (queue_id);
CREATE INDEX idx_bookings_created_at ON bookings (created_at DESC);

CREATE TABLE trips (
  id TEXT PRIMARY KEY,
  queue_id TEXT NOT NULL,
  destination_id TEXT NOT NULL,
  destination_name TEXT NOT NULL,
  license_plate TEXT NOT NULL,
  start_time TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  departed_at TIMESTAMP WITHOUT TIME ZONE NULL,
  completed_at TIMESTAMP WITHOUT TIME ZONE NULL,
  total_seats INTEGER NOT NULL DEFAULT 0,
  booked_seats INTEGER NOT NULL DEFAULT 0,
  created_by TEXT NULL,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_trips_queue_id ON trips (queue_id);

CREATE TABLE day_passes (
  id TEXT PRIMARY KEY,
  vehicle_id TEXT NOT NULL,
  license_plate TEXT NOT NULL,
  price NUMERIC(10,2) NOT NULL DEFAULT 0,
  purchase_date TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  valid_from TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  valid_until TIMESTAMP WITHOUT TIME ZONE NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_by TEXT NULL
);

CREATE TABLE exit_passes (
  id TEXT PRIMARY KEY,
  queue_id TEXT NULL,
  vehicle_id TEXT NULL,
  license_plate TEXT NULL,
  staff_id TEXT NULL,
  amount NUMERIC(10,2) NOT NULL DEFAULT 0,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

-- ---------------------------------------------------------------------------
-- Seed data (exactly one supervisor + one test station + one test route)
-- ---------------------------------------------------------------------------
INSERT INTO staff (
  id, cin, phone_number, first_name, last_name, role, is_active
) VALUES (
  'staff_supervisor_001', '12345678', '00000000', 'Test', 'Supervisor', 'SUPERVISOR', true
);

INSERT INTO station_config (
  id,
  station_id,
  station_name,
  governorate,
  delegation,
  address,
  opening_time,
  closing_time,
  is_operational,
  service_fee,
  company_name,
  company_logo_url
) VALUES (
  'cfg_test_station_001',
  'st_test_001',
  'Test Station',
  'Nabeul',
  'Nabeul',
  'Test Address',
  '05:00',
  '23:00',
  true,
  0,
  'FATMA ZAHRA Services Transport',
  '/assets/company-logo.png'
);

INSERT INTO routes (
  id, station_id, station_name, base_price, governorate, delegation, is_active
) VALUES (
  'route_test_001', 'dest_test_001', 'Test Destination', 5.00, 'Nabeul', 'Nabeul', true
);

-- Leave schema_migrations empty on purpose.
-- scripts/apply-migrations.sh will apply 009..latest after this bootstrap.
