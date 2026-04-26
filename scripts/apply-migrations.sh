#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd "${SCRIPT_DIR}/.." && pwd)"

ENV_FILE="${ENV_FILE:-${ROOT_DIR}/configs/environment.env}"
if [[ -f "${ENV_FILE}" ]]; then
  # environment.env is systemd-style (not bash); parse without eval.
  while IFS= read -r line || [[ -n "${line}" ]]; do
    [[ -z "${line}" ]] && continue
    [[ "${line}" =~ ^[[:space:]]*# ]] && continue
    if [[ "${line}" != *"="* ]]; then
      continue
    fi
    key="${line%%=*}"
    value="${line#*=}"
    key="$(echo "${key}" | xargs)"
    export "${key}=${value}"
  done < "${ENV_FILE}"
fi

DATABASE_URL="${DATABASE_URL:-}"
if [[ -z "${DATABASE_URL}" ]]; then
  echo "DATABASE_URL is not set (export it or provide configs/environment.env)" >&2
  exit 1
fi

# psql rejects some driver-only URI params (e.g. timezone=...).
PSQL_URL="${DATABASE_URL}"
if [[ "${PSQL_URL}" == *"?"* ]]; then
  base="${PSQL_URL%%\?*}"
  query="${PSQL_URL#*\?}"
  filtered=()
  IFS='&' read -r -a parts <<< "${query}"
  for part in "${parts[@]}"; do
    [[ -z "${part}" ]] && continue
    if [[ "${part}" == timezone=* ]]; then
      continue
    fi
    filtered+=("${part}")
  done
  if (( ${#filtered[@]} > 0 )); then
    PSQL_URL="${base}?$(IFS='&'; echo "${filtered[*]}")"
  else
    PSQL_URL="${base}"
  fi
fi

MIGRATIONS_DIR="${MIGRATIONS_DIR:-${ROOT_DIR}/migrations}"

if [[ ! -d "${MIGRATIONS_DIR}" ]]; then
  echo "Migrations directory not found: ${MIGRATIONS_DIR}" >&2
  exit 1
fi

psql "${PSQL_URL}" -v ON_ERROR_STOP=1 <<'SQL'
CREATE TABLE IF NOT EXISTS schema_migrations (
  filename text PRIMARY KEY,
  applied_at timestamptz NOT NULL DEFAULT now()
);
SQL

mapfile -t migration_files < <(ls -1 "${MIGRATIONS_DIR}"/*.sql 2>/dev/null | sort)

if (( ${#migration_files[@]} == 0 )); then
  echo "No migrations found in ${MIGRATIONS_DIR}"
  exit 0
fi

applied_count="$(psql "${PSQL_URL}" -tA -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM schema_migrations;")"
baseline_version="${BASELINE_VERSION:-}"
if [[ "${applied_count}" == "0" && -z "${baseline_version}" ]]; then
  # Heuristic bootstrap for existing databases that predate schema_migrations.
  # We infer a baseline from presence of newer tables, then mark older migrations as applied.
  IFS='|' read -r has_print_jobs has_staff_tx has_bookings <<<"$(psql "${PSQL_URL}" -tA -v ON_ERROR_STOP=1 -c "SELECT (to_regclass('public.print_jobs') IS NOT NULL)::int, (to_regclass('public.staff_transaction_log') IS NOT NULL)::int, (to_regclass('public.bookings') IS NOT NULL)::int;")"
  if [[ "${has_print_jobs}" == "1" ]]; then
    baseline_version="013"
  elif [[ "${has_staff_tx}" == "1" ]]; then
    baseline_version="012"
  elif [[ "${has_bookings}" == "1" ]]; then
    baseline_version="001"
  fi
fi

if [[ "${applied_count}" == "0" && -n "${baseline_version}" ]]; then
  echo "Bootstrapping schema_migrations (baseline ${baseline_version})"
  for f in "${migration_files[@]}"; do
    base="$(basename "${f}")"
    prefix="${base%%_*}"
    if [[ "${prefix}" =~ ^[0-9]{3}$ ]] && (( 10#"${prefix}" <= 10#"${baseline_version}" )); then
      psql "${PSQL_URL}" -v ON_ERROR_STOP=1 -c "INSERT INTO schema_migrations(filename) VALUES ('${base}') ON CONFLICT DO NOTHING;"
    fi
  done
  applied_count="$(psql "${PSQL_URL}" -tA -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM schema_migrations;")"
fi

echo "Applying migrations from ${MIGRATIONS_DIR}"

for f in "${migration_files[@]}"; do
  base="$(basename "${f}")"
  already_applied="$(psql "${PSQL_URL}" -tA -v ON_ERROR_STOP=1 -c "SELECT 1 FROM schema_migrations WHERE filename='${base}' LIMIT 1;")"
  if [[ "${already_applied}" == "1" ]]; then
    echo " - skip ${base}"
    continue
  fi

  echo " - apply ${base}"
  psql "${PSQL_URL}" -v ON_ERROR_STOP=1 -f "${f}"
  psql "${PSQL_URL}" -v ON_ERROR_STOP=1 -c "INSERT INTO schema_migrations(filename) VALUES ('${base}');"
done

echo "Migrations up to date."

