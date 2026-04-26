#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
ROOT_DIR="$(cd -- "${SCRIPT_DIR}/.." && pwd)"

BASE_DOMAIN="${BASE_DOMAIN:-backendglitch.com}"
STATION_SUBDOMAIN_ROOT="${STATION_SUBDOMAIN_ROOT:-station}"
PUBLIC_SERVICE_URL="${PUBLIC_SERVICE_URL:-http://localhost:8007}"
INGRESS_ORIGIN_URL="${INGRESS_ORIGIN_URL:-http://localhost:8007}"
STATE_FILE="${STATE_FILE:-${ROOT_DIR}/configs/station-cloudflare-state.json}"
PID_FILE="${PID_FILE:-${ROOT_DIR}/station-cloudflared.pid}"
LOG_FILE="${LOG_FILE:-${ROOT_DIR}/station-cloudflared.log}"
CLOUDFLARED_PROTOCOL="${CLOUDFLARED_PROTOCOL:-quic}"

RUN_TUNNEL=1
DRY_RUN=0
FORCE_NEW_TUNNEL=0
STATION_NAME=""
STATION_LOCATION=""
CUSTOM_SLUG=""

PUBLIC_BODY=""
PUBLIC_CODE=""

usage() {
  cat <<'USAGE'
Register station in local public-service and Cloudflare automatically.

Usage:
  scripts/register-station-cloudflare.sh [options]

Options:
  --name <station_name>           Station name for /info init.
  --location <station_location>   Station location for /info init.
  --slug <custom_slug>            Optional custom slug base.
  --no-run-tunnel                 Do not start cloudflared process.
  --force-new-tunnel              Ignore saved state and create a new tunnel.
  --dry-run                       Print actions without calling APIs.
  -h, --help                      Show this help.

Required env vars:
  CF_API_TOKEN   Cloudflare API token (Zone DNS Edit + Tunnel Edit)
  CF_ACCOUNT_ID  Cloudflare account id
  CF_ZONE_ID     Cloudflare zone id for backendglitch.com

Optional env vars:
  BASE_DOMAIN=backendglitch.com
  STATION_SUBDOMAIN_ROOT=station
  PUBLIC_SERVICE_URL=http://localhost:8007
  INGRESS_ORIGIN_URL=http://localhost:8007
  STATE_FILE=./configs/station-cloudflare-state.json
  PID_FILE=./station-cloudflared.pid
  LOG_FILE=./station-cloudflared.log
  CLOUDFLARED_PROTOCOL=quic
USAGE
}

require_bin() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    echo "Missing required env var: $key" >&2
    exit 1
  fi
}

slugify() {
  local raw="$1"
  local slug
  slug="$(
    printf '%s' "$raw" \
      | tr '[:upper:]' '[:lower:]' \
      | sed -E 's/[^a-z0-9]+/-/g; s/^-+//; s/-+$//; s/-{2,}/-/g'
  )"
  if [[ -z "$slug" ]]; then
    slug="station"
  fi
  printf '%s' "$slug"
}

public_call() {
  local method="$1"
  local path="$2"
  local payload="${3:-}"
  local url="${PUBLIC_SERVICE_URL%/}${path}"
  local response

  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] ${method} ${url}" >&2
    if [[ -n "$payload" ]]; then
      echo "[dry-run] payload: ${payload}" >&2
    fi
    PUBLIC_BODY='{}'
    PUBLIC_CODE='200'
    return 0
  fi

  if [[ -n "$payload" ]]; then
    response="$(
      curl -sS -X "$method" \
        -H "Content-Type: application/json" \
        -d "$payload" \
        -w $'\n%{http_code}' \
        "$url"
    )"
  else
    response="$(
      curl -sS -X "$method" \
        -w $'\n%{http_code}' \
        "$url"
    )"
  fi

  PUBLIC_BODY="${response%$'\n'*}"
  PUBLIC_CODE="${response##*$'\n'}"
}

cf_api() {
  local method="$1"
  local path="$2"
  local payload="${3:-}"
  local url="https://api.cloudflare.com/client/v4${path}"
  local response

  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "[dry-run] ${method} ${url}" >&2
    if [[ -n "$payload" ]]; then
      echo "[dry-run] payload: ${payload}" >&2
    fi
    printf '{"success":true,"result":[],"result_info":{"total_pages":1}}'
    return 0
  fi

  if [[ -n "$payload" ]]; then
    response="$(
      curl -sS -X "$method" "$url" \
        -H "Authorization: Bearer ${CF_API_TOKEN}" \
        -H "Content-Type: application/json" \
        --data "$payload"
    )"
  else
    response="$(
      curl -sS -X "$method" "$url" \
        -H "Authorization: Bearer ${CF_API_TOKEN}"
    )"
  fi

  if [[ "$(echo "$response" | jq -r '.success // false')" != "true" ]]; then
    echo "Cloudflare API error for ${method} ${path}" >&2
    echo "$response" | jq -r '.errors // .'
    return 1
  fi

  printf '%s' "$response"
}

dns_record_by_name() {
  local fqdn="$1"
  cf_api "GET" "/zones/${CF_ZONE_ID}/dns_records?type=CNAME&name=${fqdn}"
}

find_unique_slug() {
  local base_slug="$1"
  local candidate="$base_slug"
  local suffix=2

  while true; do
    local fqdn="${candidate}.${STATION_SUBDOMAIN_ROOT}.${BASE_DOMAIN}"
    local exists_count
    exists_count="$(dns_record_by_name "$fqdn" | jq -r '.result | length')"
    if [[ "$exists_count" == "0" ]]; then
      printf '%s' "$candidate"
      return 0
    fi
    candidate="${base_slug}-${suffix}"
    suffix=$((suffix + 1))
    if [[ "$suffix" -gt 500 ]]; then
      echo "Unable to find available slug after 500 attempts." >&2
      return 1
    fi
  done
}

upsert_dns_record() {
  local fqdn="$1"
  local target="$2"
  local existing
  existing="$(dns_record_by_name "$fqdn")"

  local count
  count="$(echo "$existing" | jq -r '.result | length')"
  local payload
  payload="$(jq -nc --arg n "$fqdn" --arg c "$target" '{"type":"CNAME","name":$n,"content":$c,"proxied":true}')"

  if [[ "$count" == "0" ]]; then
    cf_api "POST" "/zones/${CF_ZONE_ID}/dns_records" "$payload" >/dev/null
    echo "Created DNS CNAME: ${fqdn} -> ${target}"
    return 0
  fi

  local record_id
  record_id="$(echo "$existing" | jq -r '.result[0].id')"
  cf_api "PUT" "/zones/${CF_ZONE_ID}/dns_records/${record_id}" "$payload" >/dev/null
  echo "Updated DNS CNAME: ${fqdn} -> ${target}"
}

create_tunnel() {
  local tunnel_name="$1"

  if [[ "$DRY_RUN" -eq 1 ]]; then
    printf 'dry-run-tunnel-id\ndry-run-tunnel-token\n'
    return 0
  fi

  local tunnel_secret
  tunnel_secret="$(head -c 32 /dev/urandom | base64 | tr -d '\n')"

  local create_payload
  create_payload="$(jq -nc --arg n "$tunnel_name" --arg s "$tunnel_secret" '{"name":$n,"tunnel_secret":$s,"config_src":"cloudflare"}')"

  local created
  created="$(cf_api "POST" "/accounts/${CF_ACCOUNT_ID}/cfd_tunnel" "$create_payload")"

  local tunnel_id tunnel_token
  tunnel_id="$(echo "$created" | jq -r '.result.id')"
  tunnel_token="$(echo "$created" | jq -r '.result.token // empty')"

  if [[ -z "$tunnel_token" ]]; then
    tunnel_token="$(cf_api "GET" "/accounts/${CF_ACCOUNT_ID}/cfd_tunnel/${tunnel_id}/token" | jq -r '.result // empty')"
  fi

  if [[ -z "$tunnel_id" || -z "$tunnel_token" ]]; then
    echo "Failed to create tunnel or fetch token." >&2
    return 1
  fi

  printf '%s\n%s\n' "$tunnel_id" "$tunnel_token"
}

configure_tunnel_ingress() {
  local tunnel_id="$1"
  local fqdn="$2"
  local config_payload
  config_payload="$(jq -nc --arg host "$fqdn" --arg svc "$INGRESS_ORIGIN_URL" '{"config":{"ingress":[{"hostname":$host,"service":$svc},{"service":"http_status:404"}]}}')"

  cf_api "PUT" "/accounts/${CF_ACCOUNT_ID}/cfd_tunnel/${tunnel_id}/configurations" "$config_payload" >/dev/null
}

save_state() {
  local station_id="$1"
  local station_name="$2"
  local station_location="$3"
  local slug="$4"
  local fqdn="$5"
  local tunnel_id="$6"
  local tunnel_token="$7"

  mkdir -p "$(dirname "$STATE_FILE")"

  jq -nc \
    --arg station_id "$station_id" \
    --arg station_name "$station_name" \
    --arg station_location "$station_location" \
    --arg slug "$slug" \
    --arg fqdn "$fqdn" \
    --arg tunnel_id "$tunnel_id" \
    --arg tunnel_token "$tunnel_token" \
    --arg base_domain "$BASE_DOMAIN" \
    --arg subdomain_root "$STATION_SUBDOMAIN_ROOT" \
    --arg public_service_url "$PUBLIC_SERVICE_URL" \
    --arg ingress_origin_url "$INGRESS_ORIGIN_URL" \
    --arg updated_at "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    '{
      station_id: $station_id,
      station_name: $station_name,
      station_location: $station_location,
      slug: $slug,
      fqdn: $fqdn,
      tunnel_id: $tunnel_id,
      tunnel_token: $tunnel_token,
      base_domain: $base_domain,
      subdomain_root: $subdomain_root,
      public_service_url: $public_service_url,
      ingress_origin_url: $ingress_origin_url,
      updated_at: $updated_at
    }' >"$STATE_FILE"

  chmod 600 "$STATE_FILE" || true
}

start_tunnel_process() {
  local tunnel_token="$1"

  if ! command -v cloudflared >/dev/null 2>&1; then
    echo "cloudflared not found; tunnel not started. Install cloudflared and rerun." >&2
    return 0
  fi

  if [[ -f "$PID_FILE" ]]; then
    local old_pid
    old_pid="$(cat "$PID_FILE" 2>/dev/null || true)"
    if [[ -n "$old_pid" ]] && ps -p "$old_pid" >/dev/null 2>&1; then
      kill "$old_pid" >/dev/null 2>&1 || true
      sleep 1
    fi
    rm -f "$PID_FILE"
  fi

  nohup cloudflared tunnel --no-autoupdate --protocol "$CLOUDFLARED_PROTOCOL" run --token "$tunnel_token" >"$LOG_FILE" 2>&1 &
  local pid=$!
  echo "$pid" >"$PID_FILE"
  sleep 2

  if ps -p "$pid" >/dev/null 2>&1; then
    echo "cloudflared started (PID ${pid})"
  else
    echo "cloudflared failed to start; check ${LOG_FILE}" >&2
    return 1
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name)
      STATION_NAME="${2:-}"
      shift 2
      ;;
    --location)
      STATION_LOCATION="${2:-}"
      shift 2
      ;;
    --slug)
      CUSTOM_SLUG="${2:-}"
      shift 2
      ;;
    --no-run-tunnel)
      RUN_TUNNEL=0
      shift
      ;;
    --force-new-tunnel)
      FORCE_NEW_TUNNEL=1
      shift
      ;;
    --dry-run)
      DRY_RUN=1
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    *)
      echo "Unknown option: $1" >&2
      usage
      exit 1
      ;;
  esac
done

require_bin curl
require_bin jq
require_env CF_API_TOKEN
require_env CF_ACCOUNT_ID
require_env CF_ZONE_ID

if [[ -n "$STATION_NAME" || -n "$STATION_LOCATION" ]]; then
  if [[ -z "$STATION_NAME" || -z "$STATION_LOCATION" ]]; then
    echo "--name and --location must be provided together." >&2
    exit 1
  fi
fi

if [[ -n "$STATION_NAME" ]]; then
  init_payload="$(jq -nc --arg n "$STATION_NAME" --arg l "$STATION_LOCATION" '{"name":$n,"location":$l}')"
  public_call "POST" "/info" "$init_payload"
  if [[ "$PUBLIC_CODE" != "200" ]]; then
    echo "Failed to initialize station via ${PUBLIC_SERVICE_URL}/info (HTTP ${PUBLIC_CODE})" >&2
    echo "$PUBLIC_BODY" >&2
    exit 1
  fi
else
  public_call "GET" "/info"
  if [[ "$PUBLIC_CODE" == "404" ]]; then
    echo "Station is not initialized yet. Run with --name and --location first." >&2
    exit 1
  fi
  if [[ "$PUBLIC_CODE" != "200" ]]; then
    echo "Failed to read station info via ${PUBLIC_SERVICE_URL}/info (HTTP ${PUBLIC_CODE})" >&2
    echo "$PUBLIC_BODY" >&2
    exit 1
  fi
fi

station_name="$(echo "$PUBLIC_BODY" | jq -r --arg fallback "$STATION_NAME" '.name // $fallback')"
station_location="$(echo "$PUBLIC_BODY" | jq -r --arg fallback "$STATION_LOCATION" '.location // $fallback')"
station_id="$(echo "$PUBLIC_BODY" | jq -r '.station_id // empty')"

if [[ -z "$station_name" ]]; then
  echo "Could not resolve station name from /info response." >&2
  exit 1
fi

reuse_state=0
saved_slug=""
saved_fqdn=""
saved_tunnel_id=""
saved_tunnel_token=""

if [[ -f "$STATE_FILE" && "$FORCE_NEW_TUNNEL" -eq 0 ]]; then
  saved_slug="$(jq -r '.slug // empty' "$STATE_FILE")"
  saved_fqdn="$(jq -r '.fqdn // empty' "$STATE_FILE")"
  saved_tunnel_id="$(jq -r '.tunnel_id // empty' "$STATE_FILE")"
  saved_tunnel_token="$(jq -r '.tunnel_token // empty' "$STATE_FILE")"
  saved_station_id="$(jq -r '.station_id // empty' "$STATE_FILE")"
  saved_station_name="$(jq -r '.station_name // empty' "$STATE_FILE")"

  if [[ -n "$saved_slug" && -n "$saved_fqdn" && -n "$saved_tunnel_id" && -n "$saved_tunnel_token" ]]; then
    if [[ -n "$station_id" && "$saved_station_id" == "$station_id" ]]; then
      reuse_state=1
    elif [[ -n "$saved_station_name" && "$saved_station_name" == "$station_name" ]]; then
      reuse_state=1
    fi
  fi
fi

slug=""
fqdn=""
tunnel_id=""
tunnel_token=""

if [[ "$reuse_state" -eq 1 ]]; then
  slug="$saved_slug"
  fqdn="$saved_fqdn"
  tunnel_id="$saved_tunnel_id"
  tunnel_token="$saved_tunnel_token"
  echo "Reusing saved station domain state for ${station_name}: ${fqdn}"
else
  if [[ -n "$CUSTOM_SLUG" ]]; then
    base_slug="$(slugify "$CUSTOM_SLUG")"
  else
    base_slug="$(slugify "$station_name")"
  fi
  slug="$(find_unique_slug "$base_slug")"
  fqdn="${slug}.${STATION_SUBDOMAIN_ROOT}.${BASE_DOMAIN}"
fi

if [[ -z "$tunnel_id" || -z "$tunnel_token" || "$FORCE_NEW_TUNNEL" -eq 1 ]]; then
  tunnel_name="station-${slug}-$(date +%s)"
  mapfile -t tunnel_data < <(create_tunnel "$tunnel_name")
  tunnel_id="${tunnel_data[0]}"
  tunnel_token="${tunnel_data[1]}"
  echo "Created Cloudflare tunnel: ${tunnel_name} (${tunnel_id})"
  configure_tunnel_ingress "$tunnel_id" "$fqdn"
  echo "Configured tunnel ingress: ${fqdn} -> ${INGRESS_ORIGIN_URL}"
fi

dns_target="${tunnel_id}.cfargotunnel.com"
upsert_dns_record "$fqdn" "$dns_target"
if [[ "$DRY_RUN" -eq 0 ]]; then
  save_state "$station_id" "$station_name" "$station_location" "$slug" "$fqdn" "$tunnel_id" "$tunnel_token"
fi

if [[ "$RUN_TUNNEL" -eq 1 && "$DRY_RUN" -eq 0 ]]; then
  start_tunnel_process "$tunnel_token"
fi

echo ""
echo "Station Cloudflare registration complete."
echo "Station: ${station_name}"
if [[ -n "$station_id" ]]; then
  echo "Station ID: ${station_id}"
fi
echo "Slug: ${slug}"
echo "URL: https://${fqdn}"
if [[ "$DRY_RUN" -eq 0 ]]; then
  echo "State file: ${STATE_FILE}"
fi
if [[ "$RUN_TUNNEL" -eq 1 ]]; then
  echo "Tunnel log: ${LOG_FILE}"
fi
