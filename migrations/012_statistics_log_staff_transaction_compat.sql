-- Compatible staff transaction logging for deployments where IDs are mixed types:
-- - staff.id is TEXT
-- - bookings.id is TEXT
-- - stations.id is UUID
--
-- The application calls: SELECT log_staff_transaction($1,$2,$3,$4,$5,$6)
-- and parameters may arrive as "unknown" types from pgx. To avoid ambiguity and
-- "function does not exist" errors, we provide ONE unambiguous signature that
-- accepts everything as TEXT and casts internally.

BEGIN;

CREATE TABLE IF NOT EXISTS staff_transaction_log (
  id BIGSERIAL PRIMARY KEY,
  staff_id TEXT NOT NULL,
  transaction_type VARCHAR(20) NOT NULL,
  transaction_id TEXT NOT NULL,
  amount NUMERIC(10,2) NOT NULL,
  quantity INTEGER NOT NULL DEFAULT 1,
  station_id UUID NOT NULL,
  created_at TIMESTAMP WITHOUT TIME ZONE NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_staff_transaction_log_staff_id ON staff_transaction_log(staff_id);
CREATE INDEX IF NOT EXISTS idx_staff_transaction_log_station_id ON staff_transaction_log(station_id);
CREATE INDEX IF NOT EXISTS idx_staff_transaction_log_created_at ON staff_transaction_log(created_at);

-- Drop possible previous variants to prevent ambiguous resolution.
DROP FUNCTION IF EXISTS log_staff_transaction(text,character varying,text,numeric,integer,uuid);
DROP FUNCTION IF EXISTS log_staff_transaction(text,character varying,text,numeric,integer,text);
DROP FUNCTION IF EXISTS log_staff_transaction(text,character varying,text,double precision,integer,text);

CREATE OR REPLACE FUNCTION log_staff_transaction(
  p_staff_id TEXT,
  p_transaction_type TEXT,
  p_transaction_id TEXT,
  p_amount TEXT,
  p_quantity TEXT,
  p_station_id TEXT
) RETURNS VOID AS $$
DECLARE
  v_amount NUMERIC(10,2);
  v_qty INTEGER;
  v_station UUID;
BEGIN
  v_amount := ROUND(COALESCE(NULLIF(p_amount, '')::numeric, 0), 2);
  v_qty := COALESCE(NULLIF(p_quantity, '')::integer, 1);
  v_station := p_station_id::uuid;

  INSERT INTO staff_transaction_log (
    staff_id, transaction_type, transaction_id, amount, quantity, station_id
  ) VALUES (
    p_staff_id,
    LEFT(p_transaction_type, 20),
    p_transaction_id,
    v_amount,
    v_qty,
    v_station
  );
END;
$$ LANGUAGE plpgsql;

COMMIT;

