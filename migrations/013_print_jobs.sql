-- Durable print jobs (server-side print queue / audit trail)

CREATE TABLE IF NOT EXISTS print_jobs (
  id TEXT PRIMARY KEY,
  booking_id TEXT NULL,
  printer_id TEXT NOT NULL,
  job_type TEXT NOT NULL,
  payload_json JSONB NOT NULL,
  status TEXT NOT NULL,
  attempts INTEGER NOT NULL DEFAULT 0,
  last_error TEXT NULL,
  created_at TIMESTAMP(3) WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMP(3) WITHOUT TIME ZONE NOT NULL DEFAULT NOW(),
  printed_at TIMESTAMP(3) WITHOUT TIME ZONE NULL
);

CREATE INDEX IF NOT EXISTS idx_print_jobs_status_created_at
ON print_jobs (status, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_print_jobs_booking_id
ON print_jobs (booking_id);

CREATE INDEX IF NOT EXISTS idx_print_jobs_printer_id
ON print_jobs (printer_id);

-- Keep updated_at fresh
CREATE OR REPLACE FUNCTION set_print_jobs_updated_at() RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_print_jobs_updated_at ON print_jobs;
CREATE TRIGGER trg_print_jobs_updated_at
BEFORE UPDATE ON print_jobs
FOR EACH ROW
EXECUTE FUNCTION set_print_jobs_updated_at();

