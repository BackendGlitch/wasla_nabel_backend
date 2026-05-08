#!/usr/bin/env bash
#
# Forgot the PostgreSQL DB password for role "postgres"? This resets it while
# the server is STOPPED using single-user mode (no pg_hba/auth).
#
# Usage (pick ONE):
#   sudo ./scripts/reset-local-postgres-superuser-password.sh 'YourNewStrongPassword'
#   sudo PGDATA=/path/to/pgdata ./scripts/reset-local-postgres-superuser-password.sh 'YourNewStrongPassword'
#
# Then create wasla_db and apply migrations from wasla_backend (same password variable):
#   POSTGRES_SUPERUSER_PASSWORD='YourNewStrongPassword' bash ./scripts/init-local-database.sh
# Or smoke-test manually:
#   PGPASSWORD='YourNewStrongPassword' psql -h localhost -U postgres -d postgres -c 'SELECT 1;'
#
set -euo pipefail

NEWPASS="${1:?Usage: sudo $0 'NewPasswordForPostgres'}"

detect_pgdata() {
	local d
	for d in \
		"${PGDATA:-}" \
		/var/lib/pgsql/data \
		/var/lib/pgsql/18/data \
		/var/lib/pgsql/17/data \
		/var/lib/pgsql/16/data; do
		[[ -z "$d" ]] && continue
		if [[ -f "$d/PG_VERSION" ]]; then
			printf '%s' "$d"
			return 0
		fi
	done
	return 1
}

sql_escape_sq() {
	printf '%s' "$1" | sed "s/'/''/g"
}

PGDATA_DETECTED="$(detect_pgdata)" || true
PGDATA="${PGDATA:-$PGDATA_DETECTED}"
if [[ -z "${PGDATA}" ]] || [[ ! -f "${PGDATA}/PG_VERSION" ]]; then
	echo "Cannot find Postgres data dir (needs PG_VERSION). Set PGDATA explicitly, e.g.:" >&2
	echo "  sudo PGDATA=/var/lib/pgsql/data $0 '$NEWPASS'" >&2
	exit 1
fi

BIN="$(command -v postgres || true)"
if [[ -z "$BIN" ]] && [[ -x /usr/bin/postgres ]]; then BIN=/usr/bin/postgres; fi
if [[ -z "$BIN" ]]; then
	echo "postgres binary not found in PATH. Install postgresql-server or set PATH." >&2
	exit 1
fi

echo "Using PGDATA=$PGDATA"
echo "Stopping PostgreSQL (try common systemd unit names)..."

stopped=""
for svc in postgresql postgresql-18 postgresql-17 postgresql-16 postgresql-15; do
	if systemctl is-active --quiet "$svc" 2>/dev/null; then
		echo "  stopping $svc"
		systemctl stop "$svc"
		stopped="$svc"
		break
	fi
done

if pg_isready -h /var/run/postgresql -p 5432 2>/dev/null; then
	echo "Server still accepting connections on port 5432 — stop PostgreSQL manually, then re-run." >&2
	exit 1
fi

PWD_SQL="$(sql_escape_sq "$NEWPASS")"
TMP="$(mktemp)"
chmod 600 "$TMP"
printf "ALTER USER postgres PASSWORD '%s';\n" "$PWD_SQL" >"$TMP"

echo "Applying ALTER USER postgres in single-user mode..."
sudo -u postgres "${BIN}" --single -D "${PGDATA}" postgres <"${TMP}"
rm -f "${TMP}"

REPO_BACKEND="$(dirname "$(readlink -f "$0")")/.."
if [[ -n "$stopped" ]]; then
	systemctl start "$stopped"
	echo "Started $stopped"
	echo "Finish DB bootstrap (reuse the SAME password argument you passed above; run as YOUR user,"
	echo "sudo often strips POSTGRES_SUPERUSER_PASSWORD):"
	echo "  cd ${REPO_BACKEND} && POSTGRES_SUPERUSER_PASSWORD='…' bash ./scripts/init-local-database.sh"
else
	echo "PostgreSQL was not detected as systemd-active; start your cluster manually, then:"
	echo "  cd ${REPO_BACKEND} && POSTGRES_SUPERUSER_PASSWORD='…' bash ./scripts/init-local-database.sh"
	echo "Or if peer/socket auth works only as OS user postgres: sudo -u postgres bash ./scripts/init-local-database.sh"
fi
