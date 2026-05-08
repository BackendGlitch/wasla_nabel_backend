-- Local dev: role + DB for postgresql://wasla:Lost2409@localhost:5432/wasla_db
-- Run as Postgres superuser, e.g.:  sudo -u postgres psql -v ON_ERROR_STOP=1 -f scripts/create_wasla_db.sql
-- Then from repo root with env loaded:  ./scripts/apply-migrations.sh

DO $$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'wasla') THEN
    CREATE ROLE wasla LOGIN PASSWORD 'Lost2409';
  ELSE
    ALTER ROLE wasla WITH PASSWORD 'Lost2409';
  END IF;
END
$$;

-- If DB already exists this emits no rows and \gexec is a no-op.
SELECT format(
  'CREATE DATABASE %I OWNER %I ENCODING %L TEMPLATE template0',
  'wasla_db',
  'wasla',
  'UTF8'
)
WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname = 'wasla_db');

\gexec

\c wasla_db
GRANT ALL ON SCHEMA public TO wasla;
ALTER DATABASE wasla_db SET timezone TO 'Africa/Tunis';
