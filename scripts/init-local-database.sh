#!/usr/bin/env bash
# Create Postgres role `wasla` and database `wasla_db` (see configs/environment.env).
#
# Forgot the DB password for PostgreSQL role "postgres"? Reset it without knowing the old one:
#   sudo ./scripts/reset-local-postgres-superuser-password.sh 'your_new_pw'
#
# Typical paths:
# • pg_hba permits peer/trust on the local Unix socket — run as OS user postgres:
#     sudo -u postgres bash ./scripts/init-local-database.sh
# • pg_hba wants a password even on the socket — use the superuser password (run as YOUR user;
#     do not rely on sudo passing env vars; run without sudo unless you pass env explicitly):
#     POSTGRES_SUPERUSER_PASSWORD='your_new_pw' bash ./scripts/init-local-database.sh

set -euo pipefail
unset PGUSER PGHOST PGPORT
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
SQL_FILE="${ROOT_DIR}/scripts/create_wasla_db.sql"

echo "Applying ${SQL_FILE} ..."
if [[ -n "${POSTGRES_SUPERUSER_PASSWORD:-}" ]]; then
	# pg_hba uses password auth (e.g. scram) — use role postgres on localhost
	export PGPASSWORD="${POSTGRES_SUPERUSER_PASSWORD}"
	psql -h localhost -U postgres -d postgres -v ON_ERROR_STOP=1 -f "${SQL_FILE}"
	unset PGPASSWORD
else
	# Expects peer/trust for local socket as OS user postgres (no DB password)
	unset PGPASSWORD
	psql -d postgres -v ON_ERROR_STOP=1 -f "${SQL_FILE}"
fi

echo "Testing connection as wasla → wasla_db ..."
export PGPASSWORD="${PGPASSWORD:-Lost2409}"
psql -h localhost -U wasla -d wasla_db -v ON_ERROR_STOP=1 -c "SELECT current_database(), current_user, version();"

# Migrations in-repo start at 009; earlier DDL lives in bootstrap_clean_db.sql (see that file header).
BOOTSTRAP_FILE="${ROOT_DIR}/scripts/bootstrap_clean_db.sql"
WASLA_PSQL_URL="postgresql://wasla:Lost2409@localhost:5432/wasla_db?sslmode=disable"
has_staff="$(PGPASSWORD="${PGPASSWORD:-Lost2409}" psql "${WASLA_PSQL_URL}" -tAv ON_ERROR_STOP=1 -c "SELECT (to_regclass('public.staff') IS NOT NULL)::text;")"
if [[ "${has_staff}" != "t" ]]; then
	echo "Applying baseline schema (${BOOTSTRAP_FILE}) ..."
	PGPASSWORD="${PGPASSWORD:-Lost2409}" psql "${WASLA_PSQL_URL}" -v ON_ERROR_STOP=1 -f "${BOOTSTRAP_FILE}"
fi

echo "Applying migrations ..."
cd "${ROOT_DIR}"
export DATABASE_URL="postgresql://wasla:Lost2409@localhost:5432/wasla_db?sslmode=disable&timezone=Africa/Tunis"
chmod +x "${ROOT_DIR}/scripts/apply-migrations.sh"
"${ROOT_DIR}/scripts/apply-migrations.sh"

echo "Done. Postgres is ready; DATABASE_URL matches configs/environment.env (~wasla_backend)."
