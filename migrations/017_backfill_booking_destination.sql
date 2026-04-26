-- Backfill destination_id on bookings that have a queue_id but no destination_id.
-- After this, even if queue entries are later deleted (ON DELETE SET NULL), the
-- destination_id column persists.
UPDATE bookings b
SET destination_id = q.destination_id
FROM vehicle_queue q
WHERE b.queue_id = q.id
  AND (b.destination_id IS NULL OR b.destination_id = '')
  AND q.destination_id IS NOT NULL
  AND q.destination_id != '';
