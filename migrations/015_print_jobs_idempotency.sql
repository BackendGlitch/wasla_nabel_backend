-- Make print jobs idempotent (dedupe retries per printer)

ALTER TABLE print_jobs
  ADD COLUMN IF NOT EXISTS idempotency_key TEXT NULL;

-- Dedupe within a printer (same printer can receive retries for same user action)
CREATE UNIQUE INDEX IF NOT EXISTS idx_print_jobs_printer_idempotency_unique
ON print_jobs (printer_id, idempotency_key)
WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';

CREATE INDEX IF NOT EXISTS idx_print_jobs_pending_fifo
ON print_jobs (status, created_at ASC, id ASC);

