-- Migration 010: Wasla intern bookings table for sandboxed public-service flow

CREATE TABLE IF NOT EXISTS wasla_intern_booking (
    id VARCHAR(64) PRIMARY KEY,
    queue_id TEXT,
    destination_id VARCHAR(255),
    seats_booked INTEGER NOT NULL,
    total_amount DECIMAL(10,2) NOT NULL,
    booking_source VARCHAR(50) DEFAULT 'CENTRAL',
    booking_type VARCHAR(20) DEFAULT 'ONLINE',
    booking_status VARCHAR(20) DEFAULT 'HELD',
    payment_status VARCHAR(20) DEFAULT 'UNPAID',
    payment_method VARCHAR(20) DEFAULT 'ONLINE',
    verification_code VARCHAR(10) UNIQUE NOT NULL,
    is_verified BOOLEAN DEFAULT false,
    user_ref TEXT,
    idempotency_key TEXT,
    expires_at TIMESTAMP(3) WITHOUT TIME ZONE,
    payment_processed_at TIMESTAMP(3) WITHOUT TIME ZONE,
    cancelled_at TIMESTAMP(3) WITHOUT TIME ZONE,
    cancelled_by UUID,
    cancellation_reason TEXT,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_wasla_intern_booking_idempotency_key_unique
ON wasla_intern_booking (idempotency_key)
WHERE idempotency_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_wasla_intern_booking_held_expires_at
ON wasla_intern_booking (booking_status, expires_at)
WHERE booking_status = 'HELD';

CREATE INDEX IF NOT EXISTS idx_wasla_intern_booking_queue_id
ON wasla_intern_booking (queue_id);

CREATE INDEX IF NOT EXISTS idx_wasla_intern_booking_destination_id
ON wasla_intern_booking (destination_id);

CREATE INDEX IF NOT EXISTS idx_wasla_intern_booking_status
ON wasla_intern_booking (booking_status);

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'wasla_intern_booking'::regclass
          AND conname = 'wasla_intern_booking_booking_status_check'
    ) THEN
        ALTER TABLE wasla_intern_booking
        ADD CONSTRAINT wasla_intern_booking_booking_status_check
        CHECK (
            booking_status IN (
                'HELD',
                'ACTIVE',
                'CANCELLED',
                'EXPIRED',
                'COMPLETED',
                'REFUNDED'
            )
        );
    END IF;

    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conrelid = 'wasla_intern_booking'::regclass
          AND conname = 'wasla_intern_booking_payment_status_check'
    ) THEN
        ALTER TABLE wasla_intern_booking
        ADD CONSTRAINT wasla_intern_booking_payment_status_check
        CHECK (payment_status IN ('UNPAID', 'PAID', 'FAILED'));
    END IF;
END $$;
