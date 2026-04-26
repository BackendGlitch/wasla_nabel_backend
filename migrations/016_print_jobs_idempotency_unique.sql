-- Fix ON CONFLICT target: use a non-partial unique index so ON CONFLICT (printer_id, idempotency_key) works.

DROP INDEX IF EXISTS idx_print_jobs_printer_idempotency_unique;

CREATE UNIQUE INDEX IF NOT EXISTS idx_print_jobs_printer_idempotency_unique
ON print_jobs (printer_id, idempotency_key);

