#!/usr/bin/env bash
set -euo pipefail

BASE_DOMAIN="${BASE_DOMAIN:-backendglitch.com}"
STATION_SUBDOMAIN_ROOT="${STATION_SUBDOMAIN_ROOT:-station}"
CHECK_HEALTH=0

usage() {
  cat <<'USAGE'
List station subdomains registered in Cloudflare DNS.

Usage:
  scripts/list-stations-cloudflare.sh [options]

Options:
  --check-health   Also call https://<station>/health and print status.
  -h, --help       Show this help.

Required env vars:
  CF_API_TOKEN
  CF_ZONE_ID

Optional env vars:
  BASE_DOMAIN=backendglitch.com
  STATION_SUBDOMAIN_ROOT=station
USAGE
}

require_env() {
  local key="$1"
  if [[ -z "${!key:-}" ]]; then
    echo "Missing required env var: $key" >&2
    exit 1
  fi
}

require_bin() {
  local cmd="$1"
  if ! command -v "$cmd" >/dev/null 2>&1; then
    echo "Missing required command: $cmd" >&2
    exit 1
  fi
}

cf_api() {
  local method="$1"
  local path="$2"
  local url="https://api.cloudflare.com/client/v4${path}"
  local response

  response="$(
    curl -sS -X "$method" "$url" \
      -H "Authorization: Bearer ${CF_API_TOKEN}"
  )"

  if [[ "$(echo "$response" | jq -r '.success // false')" != "true" ]]; then
    echo "Cloudflare API error for ${method} ${path}" >&2
    echo "$response" | jq -r '.errors // .'
    return 1
  fi

  printf '%s' "$response"
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --check-health)
      CHECK_HEALTH=1
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
require_env CF_ZONE_ID

suffix=".${STATION_SUBDOMAIN_ROOT}.${BASE_DOMAIN}"
page=1
total_pages=1
all='[]'

while [[ "$page" -le "$total_pages" ]]; do
  resp="$(cf_api "GET" "/zones/${CF_ZONE_ID}/dns_records?type=CNAME&per_page=100&page=${page}")"
  filtered="$(
    echo "$resp" | jq --arg suffix "$suffix" '
      .result
      | map(select(.name | endswith($suffix)))
    '
  )"
  all="$(jq -s '.[0] + .[1]' <(echo "$all") <(echo "$filtered"))"
  total_pages="$(echo "$resp" | jq -r '.result_info.total_pages // 1')"
  page=$((page + 1))
done

count="$(echo "$all" | jq 'length')"
echo "Found ${count} station DNS records under *${suffix}"
echo ""

if [[ "$count" == "0" ]]; then
  exit 0
fi

while IFS=$'\t' read -r fqdn target proxied modified; do
  if [[ "$CHECK_HEALTH" -eq 1 ]]; then
    code="$(curl -sS -o /dev/null -w '%{http_code}' --max-time 6 "https://${fqdn}/health" || true)"
    if [[ -z "$code" || "$code" == "000" ]]; then
      code="unreachable"
    fi
    echo "${fqdn} -> ${target} | ${proxied} | health=${code} | updated=${modified}"
  else
    echo "${fqdn} -> ${target} | ${proxied} | updated=${modified}"
  fi
done < <(
  echo "$all" | jq -r '
    sort_by(.name)
    | .[]
    | [
        .name,
        .content,
        (if .proxied then "proxied" else "dns-only" end),
        (.modified_on // .created_on // "-")
      ]
    | @tsv
  '
)
