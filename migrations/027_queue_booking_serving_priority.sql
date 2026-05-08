-- Per-destination "serving" row: keep booking on same car after unblock until it is full.
ALTER TABLE vehicle_queue
  ADD COLUMN IF NOT EXISTS prioritize_after_blocked_unblock BOOLEAN NOT NULL DEFAULT FALSE;

CREATE TABLE IF NOT EXISTS queue_destination_booking_state (
  destination_id TEXT PRIMARY KEY,
  serving_queue_entry_id TEXT REFERENCES vehicle_queue (id) ON DELETE SET NULL
);
