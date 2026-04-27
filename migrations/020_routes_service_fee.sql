-- Per-destination service fee (TND) used in ticket pricing.
-- Default stays 0.200 (200 millimes) when not specified.
ALTER TABLE routes
  ADD COLUMN IF NOT EXISTS service_fee NUMERIC(10,3) NOT NULL DEFAULT 0.200;

