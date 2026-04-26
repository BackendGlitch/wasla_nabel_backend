-- Migration 011: Align wasla_intern_booking.queue_id with vehicle_queue.id (text)

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'wasla_intern_booking'
          AND column_name = 'queue_id'
          AND data_type <> 'text'
    ) THEN
        ALTER TABLE wasla_intern_booking
        ALTER COLUMN queue_id TYPE text
        USING queue_id::text;
    END IF;
END $$;
