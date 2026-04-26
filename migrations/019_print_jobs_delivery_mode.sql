-- Adds delivery_mode to support client-local POS printing in addition to backend_tcp.
--
-- backend_tcp  : existing flow. printer-service worker pulls pending jobs and
--                opens raw TCP to printer.IP:printer.Port. Used by management.
-- client_local : new flow for POS staff machines. Backend renders ESC/POS bytes
--                and returns them to the client; the local Electron main writes
--                them to the USB printer and acks the job. printer_id for these
--                rows is prefixed with "client:" so they are easy to spot.
--
-- This migration is additive and backwards compatible. Default is backend_tcp,
-- so all historical and existing rows keep their semantics.

ALTER TABLE print_jobs
  ADD COLUMN IF NOT EXISTS delivery_mode TEXT NOT NULL DEFAULT 'backend_tcp';

DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'print_jobs_delivery_mode_check'
  ) THEN
    ALTER TABLE print_jobs
      ADD CONSTRAINT print_jobs_delivery_mode_check
      CHECK (delivery_mode IN ('backend_tcp', 'client_local'));
  END IF;
END$$;

CREATE INDEX IF NOT EXISTS idx_print_jobs_delivery_mode_status
ON print_jobs (delivery_mode, status, created_at DESC);

-- The application-level status column has no DB CHECK constraint today, so
-- the new logical value 'rendered' (server has produced bytes; waiting for
-- client ack) does not need a schema change. This comment documents intent.
