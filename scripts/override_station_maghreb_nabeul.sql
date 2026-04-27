-- Full station override for:
--   Company: Societe Regionale Privee Fatma Ezzahra de Services de Transport a Nabeul
--   Station: Station Maghreb Arabi - Nabeul
--
-- This script is DESTRUCTIVE for operational/master data in this DB.
-- It keeps schema objects, but clears and reseeds station data.
--
-- Usage:
--   psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f scripts/override_station_maghreb_nabeul.sql

BEGIN;

-- ---------------------------------------------------------------------------
-- 0) Hardening: ensure core columns/tables expected by the current backend
-- ---------------------------------------------------------------------------

-- Vehicles seat-capacity columns (used by CreateVehicle / queue logic).
ALTER TABLE IF EXISTS vehicles ADD COLUMN IF NOT EXISTS available_seats INTEGER NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS vehicles ADD COLUMN IF NOT EXISTS total_seats INTEGER NOT NULL DEFAULT 0;
ALTER TABLE IF EXISTS vehicles ADD COLUMN IF NOT EXISTS base_price NUMERIC(10,2) NOT NULL DEFAULT 2.00;
ALTER TABLE IF EXISTS vehicles ADD COLUMN IF NOT EXISTS destination_id TEXT NULL;
ALTER TABLE IF EXISTS vehicles ADD COLUMN IF NOT EXISTS destination_name TEXT NULL;
ALTER TABLE IF EXISTS vehicles ADD COLUMN IF NOT EXISTS default_destination_id TEXT NULL;
ALTER TABLE IF EXISTS vehicles ADD COLUMN IF NOT EXISTS default_destination_name TEXT NULL;

-- If a legacy table name exists, rename to what the code uses.
DO $$
BEGIN
  IF to_regclass('public.vehicle_authorized_stations') IS NULL
     AND to_regclass('public.vehicle_authorized_routes') IS NOT NULL THEN
    ALTER TABLE vehicle_authorized_routes RENAME TO vehicle_authorized_stations;
  END IF;
END $$;

-- Ensure authorized stations table exists.
CREATE TABLE IF NOT EXISTS vehicle_authorized_stations (
  id TEXT PRIMARY KEY,
  vehicle_id TEXT NOT NULL,
  station_id TEXT NOT NULL,
  station_name TEXT NULL,
  priority INTEGER NOT NULL DEFAULT 0,
  is_default BOOLEAN NOT NULL DEFAULT false,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_vehicle_authorized_stations_vehicle ON vehicle_authorized_stations(vehicle_id);
CREATE INDEX IF NOT EXISTS idx_vehicle_authorized_stations_station ON vehicle_authorized_stations(station_id);

-- Station config fields consumed by init/public endpoints.
ALTER TABLE IF EXISTS station_config ADD COLUMN IF NOT EXISTS company_name TEXT NOT NULL DEFAULT '';
ALTER TABLE IF EXISTS station_config ADD COLUMN IF NOT EXISTS company_logo_url TEXT NOT NULL DEFAULT '';

-- Route Arabic metadata (optional; kept for bilingual display if UI uses it).
ALTER TABLE IF EXISTS routes ADD COLUMN IF NOT EXISTS governorate_ar TEXT NULL;
ALTER TABLE IF EXISTS routes ADD COLUMN IF NOT EXISTS delegation_ar TEXT NULL;
ALTER TABLE IF EXISTS routes ADD COLUMN IF NOT EXISTS service_fee NUMERIC(10,3) NOT NULL DEFAULT 0.200;

-- ---------------------------------------------------------------------------
-- 1) Clear operational + master data (schema preserved)
-- ---------------------------------------------------------------------------
TRUNCATE TABLE
  staff_transaction_log,
  print_jobs,
  wasla_intern_booking,
  bookings,
  trips,
  vehicle_queue,
  day_passes,
  exit_passes,
  vehicle_authorized_stations,
  vehicles,
  routes,
  station_config,
  staff
RESTART IDENTITY CASCADE;

-- ---------------------------------------------------------------------------
-- 2) Seed one supervisor user (CIN 12345678)
-- ---------------------------------------------------------------------------
INSERT INTO staff (
  id, cin, phone_number, first_name, last_name, role, is_active, created_at, updated_at
) VALUES (
  'staff_supervisor_001',
  '12345678',
  '',
  'Superviseur',
  'Principal',
  'SUPERVISOR',
  true,
  NOW(),
  NOW()
);

-- ---------------------------------------------------------------------------
-- 3) Seed station/company profile
-- ---------------------------------------------------------------------------
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
  company_logo_url,
  created_at,
  updated_at
) VALUES (
  'cfg_maghreb_nabeul_001',
  'st_maghreb_nabeul',
  'Station Maghreb Arabi - Nabeul',
  'Nabeul',
  'Nabeul',
  'Nabeul',
  '05:00',
  '23:00',
  true,
  0,
  'FATMA ZAHRA Services Transport',
  '/assets/company-logo.png',
  NOW(),
  NOW()
);

-- ---------------------------------------------------------------------------
-- 4) Seed routes for this station
-- NOTE: in this codebase, routes.station_id is treated as destination id.
-- ---------------------------------------------------------------------------
INSERT INTO routes (
  id, station_id, station_name, base_price, service_fee, governorate, governorate_ar, delegation, delegation_ar, is_active, updated_at
) VALUES
  ('route_grombalia',       'dest_grombalia',        'Grombalia',         3.800, 0.200, 'Nabeul', 'نابل', 'Nabeul', 'نابل', true, NOW()),
  ('route_beni_khalled',    'dest_beni_khalled',     'Beni Khalled',      3.600, 0.100, 'Nabeul', 'نابل', 'Nabeul', 'نابل', true, NOW()),
  ('route_menzel_bouzelfa', 'dest_menzel_bouzelfa',  'Menzel Bouzelfa',   3.400, 0.050, 'Nabeul', 'نابل', 'Nabeul', 'نابل', true, NOW()),
  ('route_soliman',         'dest_soliman',          'Soliman',           4.200, 0.200, 'Nabeul', 'نابل', 'Nabeul', 'نابل', true, NOW()),
  ('route_belli_mhazba',    'dest_belli_mhazba',     'Belli - El Mhazba', 4.000, 0.200, 'Nabeul', 'نابل', 'Nabeul', 'نابل', true, NOW());

COMMIT;

-- Quick post-checks
SELECT 'staff_count' AS check_name, COUNT(*)::TEXT AS value FROM staff
UNION ALL
SELECT 'station_config_count', COUNT(*)::TEXT FROM station_config
UNION ALL
SELECT 'routes_count', COUNT(*)::TEXT FROM routes
UNION ALL
SELECT 'vehicle_authorized_stations_exists',
       CASE WHEN to_regclass('public.vehicle_authorized_stations') IS NULL THEN 'no' ELSE 'yes' END;
