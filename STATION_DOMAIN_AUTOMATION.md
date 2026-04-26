# Station Domain Automation (Cloudflare)

This automation creates a unique station URL like:

- `monastir.station.backendglitch.com`
- or `monastir.backendglitch.com` when subdomain root is empty

It also sets up a Cloudflare Tunnel to `public-service` on `http://localhost:8007`,
so no router port-forwarding is required.

## Prerequisites

- `curl`
- `jq`
- `cloudflared` (required if you want the tunnel process started automatically)
- Cloudflare API token with:
  - Zone DNS Edit permission on `backendglitch.com`
  - Account Cloudflare Tunnel Edit permission

## Required environment variables

Use `configs/cloudflare-station.env.example` as template:

- `CF_API_TOKEN`
- `CF_ACCOUNT_ID`
- `CF_ZONE_ID`

Optional:

- `BASE_DOMAIN` (default: `backendglitch.com`)
- `STATION_SUBDOMAIN_ROOT` (default: `station`, set empty for direct `slug.backendglitch.com`)
- `PUBLIC_SERVICE_URL` (default: `http://localhost:8007`)
- `INGRESS_ORIGIN_URL` (default: `http://localhost:8007`)

## Register station + create unique domain + tunnel

```bash
cd wasla_backend
set -a
source configs/cloudflare-station.env
set +a

./scripts/register-station-cloudflare.sh --name "Monastir" --location "Monastir"
```

Or via manager:

```bash
./manage-services.sh register-station --name "Monastir" --location "Monastir"
```

## Discover stations (from Cloudflare DNS)

```bash
./scripts/list-stations-cloudflare.sh
./scripts/list-stations-cloudflare.sh --check-health
```

Or:

```bash
./manage-services.sh list-stations --check-health
```

## Notes

- State is saved in `configs/station-cloudflare-state.json` (contains tunnel token, protected with `chmod 600`).
- Running registration again reuses saved state unless `--force-new-tunnel` is passed.
