-- Garage / blocking: frozen in queue slot; excluded from next-vehicle booking and POS queue list aggregates.

ALTER TABLE vehicle_queue ADD COLUMN IF NOT EXISTS is_garage_blocked BOOLEAN NOT NULL DEFAULT FALSE;
