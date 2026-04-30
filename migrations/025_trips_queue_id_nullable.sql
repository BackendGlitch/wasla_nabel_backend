-- Allow unlinking trips from removed queue rows (management removes vehicle from queue
-- after recording an exit-trip row). Nullable queue_id is required for UPDATE ... SET queue_id = NULL.

ALTER TABLE trips
  ALTER COLUMN queue_id DROP NOT NULL;
