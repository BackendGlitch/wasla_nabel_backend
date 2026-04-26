# Wasla Backend API Documentation

This documentation is generated from the current backend source code in `cmd/*/main.go` and `internal/*/handler.go`.

Authoritative route matrix:
- `docs/API_ROUTES.generated.md` (generated directly from code)

Verification commands:
- `npm run routes:sync`
- `npm run docs:check`
- `npm run routes:verify`
- `npm run hooks:install` (install local pre-commit drift checks)

## Base URLs (local)

- Auth Service: `http://localhost:8001`
- Queue Service: `http://localhost:8002`
- Booking Service: `http://localhost:8003`
- WebSocket Hub: `http://localhost:8004`
- Printer Service: `http://localhost:8005` (falls back to `8084` only if `PRINTER_SERVICE_PORT` is unset)
- Statistics Service: `http://localhost:8006`
- Public Service: `http://localhost:8007`

Postman collection host switch:
- Variable: `server_ip` (default `localhost`)
- Base URL variables use it: `http://{{server_ip}}:8001..8006`
- For remote server testing, set `server_ip` to your server IP once, all requests will follow.

## Authentication

- Protected endpoints require `Authorization: Bearer <JWT>`.
- JWT is issued by `POST /api/v1/auth/login`.
- Session must also exist in Redis; token-only is not enough.
- For WebSocket/browser use-cases, auth middleware also accepts `?token=<JWT>` query parameter.

## Common Response Shape (most services)

Used by auth, queue, booking, statistics, websocket admin endpoints:

```json
{
  "success": true,
  "message": "...",
  "data": {}
}
```

Error shape:

```json
{
  "success": false,
  "message": "...",
  "error": "Bad Request | Unauthorized | Not Found | <internal error>"
}
```

Note: Printer service and Public service endpoints return direct JSON payloads (not wrapped in `success/message/data`).

## Auth Service (`http://localhost:8001`)

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/health` | No | - | - |
| POST | `/api/v1/auth/login` | No | - | `{ cin }` |
| POST | `/api/v1/auth/refresh` | Yes | - | - |
| POST | `/api/v1/auth/logout` | Yes | - | - |
| GET | `/api/v1/staff/` | Yes | - | - |
| GET | `/api/v1/staff/:id` | Yes | - | - |
| POST | `/api/v1/staff/` | Yes | - | `{ cin, phoneNumber, firstName, lastName, role }` |
| PUT | `/api/v1/staff/:id` | Yes | - | `{ phoneNumber, firstName, lastName, role, isActive? }` |
| DELETE | `/api/v1/staff/:id` | Yes | - | - |

## Queue Service (`http://localhost:8002`)

### System & Docs

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/health` | No | - | - |
| GET | `/openapi-queue.json` | No | - | - |
| GET | `/docs-queue` | No | - | - |
| GET | `/openapi-all.json` | No | - | - |
| GET | `/docs-all` | No | - | - |

### Routes

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/api/v1/routes` | Yes | - | - |
| POST | `/api/v1/routes` | Yes | - | `{ stationId, stationName, basePrice, governorate?, governorateAr?, delegation?, delegationAr? }` |
| PUT | `/api/v1/routes/:id` | Yes | - | `{ stationName?, basePrice?, governorate?, governorateAr?, delegation?, delegationAr?, isActive? }` |
| DELETE | `/api/v1/routes/:id` | Yes | - | - |

### Vehicles

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/api/v1/vehicles` | Yes | `search` (optional) | - |
| GET | `/api/v1/vehicles/search` | Yes | `q` (required) | - |
| POST | `/api/v1/vehicles` | Yes | - | `{ licensePlate, capacity, phoneNumber? }` |
| PUT | `/api/v1/vehicles/:id` | Yes | - | `{ capacity?, phoneNumber?, isActive?, isAvailable?, isBanned?, defaultDestinationId?, defaultDestinationName? }` |
| DELETE | `/api/v1/vehicles/:id` | Yes | - | - |

### Vehicle Authorized Routes

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/api/v1/vehicles/:id/authorized-routes` | Yes | - | - |
| POST | `/api/v1/vehicles/:id/authorized-routes` | Yes | - | `{ stationId, stationName?, priority, isDefault }` |
| PUT | `/api/v1/vehicles/:id/authorized-routes/:authId` | Yes | - | `{ priority?, isDefault? }` |
| DELETE | `/api/v1/vehicles/:id/authorized-routes/:authId` | Yes | - | - |

### Queue Management

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/api/v1/queue/:destinationId` | Yes | `subRoute` (optional) | - |
| POST | `/api/v1/queue/:destinationId` | Yes | - | `{ vehicleId, destinationName, subRoute?, subRouteName?, queueType?, createdBy? }` (`destinationId` forced from path) |
| PUT | `/api/v1/queue/:destinationId/reorder` | Yes | - | `{ entryIds: string[] }` |
| PUT | `/api/v1/queue/:destinationId/entry/:id` | Yes | - | `{ status?, availableSeats?, estimatedDeparture?, subRoute?, subRouteName? }` |
| DELETE | `/api/v1/queue/:destinationId/entry/:id` | Yes | - | - |
| PUT | `/api/v1/queue/:destinationId/entry/:id/move` | Yes | - | `{ newPosition }` |
| POST | `/api/v1/queue/:destinationId/transfer-seats` | Yes | - | `{ fromEntryId, toEntryId, seats }` |
| PUT | `/api/v1/queue/:destinationId/entry/:id/change-destination` | Yes | - | `{ newDestinationId, newDestinationName }` |
| DELETE | `/api/v1/queue/:destinationId/clear` | Yes | - | - |
| DELETE | `/api/v1/queue/clear-all` | Yes | - | - |

### Aggregates & Day Passes

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/api/v1/queue-summaries` | Yes | `station` (optional) | - |
| GET | `/api/v1/destinations` | Yes | - | - |
| GET | `/api/v1/route-summaries` | Yes | - | - |
| GET | `/api/v1/day-passes` | Yes | - | - |
| GET | `/api/v1/day-pass/vehicle/:vehicleId` | Yes | - | - |

## Booking Service (`http://localhost:8003`)

### System & Docs

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/health` | No | - | - |
| GET | `/openapi.json` | No | - | - |
| GET | `/docs` | No | - | - |

### Booking & Trips

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| POST | `/api/v1/bookings` | Yes | - | `{ destinationId, seats, staffId?, subRoute?, preferExactFit? }` |
| POST | `/api/v1/bookings/by-queue-entry` | Yes | - | `{ queueEntryId, seats, staffId? }` |
| POST | `/api/v1/bookings/cancel-one-by-queue-entry` | Yes | - | `{ queueEntryId, staffId? }` |
| PUT | `/api/v1/bookings/:id/cancel` | Yes | - | `{ staffId, reason? }` |
| POST | `/api/v1/bookings/ghost` | Yes | - | `{ destinationId, seats, staffId?, idempotencyKey? }` |
| GET | `/api/v1/bookings/ghost/count` | Yes | `destination_id` (required) | - |
| GET | `/api/v1/trips` | Yes | - | - |
| GET | `/api/v1/trips/today` | Yes | `search` (optional) | - |
| GET | `/api/v1/trips/today/count` | Yes | `destination_id` (optional) | - |
| GET | `/api/v1/trips/count-by-license` | Yes | `license_plate` (required) | - |

## Public Service (`http://localhost:8007`)

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/health` | No | - | - |
| POST | `/info` | No | - | `{ name, location }` |
| GET | `/info` | No | - | - |
| GET | `/routes` | No | - | - |
| GET | `/routes/:id` | No | - | - |
| POST | `/bookings` | No | - | `{ destination_id, seats_booked, booking_source?, booking_type?, user_ref?, idempotency_key }` |
| GET | `/bookings/:id` | No | - | - |
| POST | `/bookings/:id/confirm` | No | - | `{ payment_status: "PAID", payment_method, payment_processed_at? }` |
| POST | `/bookings/:id/cancel` | No | - | `{ cancelled_by?, cancellation_reason? }` |
| POST | `/api/v1/auth/login` | No | - | `{ cin }` |
| ANY | `/api/v1/statistics` | Yes | - | - |
| ANY | `/api/v1/statistics/*path` | Yes | - | - |
| GET | `/api/v1/public-statistics/overview/day` | Yes | `date` (optional, `YYYY-MM-DD`) | - |
| GET | `/api/v1/public-statistics/overview/month` | Yes | `year`, `month` (optional) | - |
| GET | `/api/v1/public-statistics/ws` | Yes | `token` (optional fallback if no `Authorization` header) | - |

`GET /api/v1/public-statistics/ws` notes:
- Connect with `wss://<station-domain>/api/v1/public-statistics/ws?token=<JWT>` (or send `Authorization: Bearer <JWT>`).
- Current stream messages are pushed as JSON objects like:

```json
{
  "type": "statistics_update",
  "stationId": "*",
  "data": {
    "type": "station_income_update",
    "stationId": "st_...",
    "timestamp": "2026-02-21T12:00:00Z"
  },
  "timestamp": 1771675200
}
```

`POST /info` and `GET /info` IP behavior:
- `public_ip` returns the server Tailscale IPv4 when a `tailscale*` interface is available.
- If no Tailscale IP is found, it falls back to the server public IPv4 from external IP providers.

`POST /info` init rule:
- Can only be called once per station node.
- If station config already exists, it returns `200` with existing station info and:

```json
{
  "already_configured": true,
  "message": "Station is already configured and cannot be configured again"
}
```

`POST /bookings/:id/confirm` response shape:

```json
{
  "booking_id": "bkg_...",
  "booking_status": "ACTIVE",
  "payment_status": "PAID",
  "payment_method": "CLICK",
  "payment_processed_at": "2026-02-16T12:04:30Z",
  "vehicle_license_plate": "123 TUN 4567"
}
```

Note: `queue_id` is intentionally not returned by the confirm endpoint.

## Printer Service (`http://localhost:8005`)

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/health` | No | - | - |
| GET | `/api/printer/config/:id` | No | - | - |
| PUT | `/api/printer/config/:id` | No | - | `PrinterConfig` |
| POST | `/api/printer/test/:id` | No | - | - |
| GET | `/api/printer/queue` | No | - | - |
| GET | `/api/printer/queue/status` | No | - | - |
| POST | `/api/printer/queue/add` | No | - | `{ jobType, content, staffName?, priority? }` |
| POST | `/api/printer/print/booking` | No | - | `TicketData` |
| POST | `/api/printer/print/entry` | No | - | `TicketData` |
| POST | `/api/printer/print/exit` | No | - | `TicketData` |
| POST | `/api/printer/print/daypass` | No | - | `TicketData` |
| POST | `/api/printer/print/exitpass` | No | - | `TicketData` |
| POST | `/api/printer/print/exitpass-and-remove` | No | - | `ExitPassAndRemoveRequest` |
| POST | `/api/printer/print/talon` | No | - | `TicketData` |

### `TicketData` JSON fields

```json
{
  "licensePlate": "123 TUN 4567",
  "destinationName": "Tunis",
  "seatNumber": 1,
  "totalAmount": 5.15,
  "createdBy": "staff_123",
  "createdAt": "2026-02-15T10:30:00Z",
  "stationName": "Main Station",
  "routeName": "Tunis Route",
  "vehicleCapacity": 8,
  "basePrice": 5.0,
  "exitPassCount": 2,
  "companyName": "STE DHRAIFF SERVICES TRANSPORT",
  "companyLogo": "",
  "staffFirstName": "Ali",
  "staffLastName": "Ben Salah",
  "printerConfig": {
    "ip": "192.168.1.50",
    "port": 9100
  }
}
```

## Statistics Service (`http://localhost:8006`)

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/health` | No | - | - |
| GET | `/api/v1/statistics/staff/:staffId/daily` | Yes | `date` (optional, `YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/staff/:staffId/today` | Yes | - | - |
| GET | `/api/v1/statistics/staff/:staffId/range` | Yes | `startDate` + `endDate` (`YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/staff/all` | Yes | `date` (optional, `YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/staff/all-month` | Yes | `year`, `month` | - |
| GET | `/api/v1/statistics/staff/:staffId/transactions` | Yes | `limit` (optional) | - |
| GET | `/api/v1/statistics/station/:stationId/daily` | Yes | `date` (optional, `YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/station/:stationId/today` | Yes | - | - |
| GET | `/api/v1/statistics/station/:stationId/range` | Yes | `startDate` + `endDate` (`YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/station/all` | Yes | `date` (optional, `YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/station/all-month` | Yes | `year`, `month` | - |
| GET | `/api/v1/statistics/station/:stationId/transactions` | Yes | `limit` (optional) | - |
| GET | `/api/v1/statistics/ws` | No | - | - |
| POST | `/api/v1/statistics/broadcast` | No | - | - |
| GET | `/api/v1/statistics/income/actual` | Yes | `date` (optional, `YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/income/period` | Yes | `start`, `end` (RFC3339) | - |
| GET | `/api/v1/statistics/income/day` | Yes | `date` (`YYYY-MM-DD`, required) | - |
| GET | `/api/v1/statistics/income/month` | Yes | `year`, `month` | - |
| GET | `/api/v1/statistics/overview/day` | Yes | `date` (optional, `YYYY-MM-DD`) | - |
| GET | `/api/v1/statistics/overview/month` | Yes | `year`, `month` (optional) | - |

## WebSocket Hub Service (`http://localhost:8004`)

| Method | Path | Auth | Query Params | Body |
|---|---|---|---|---|
| GET | `/health` | No | - | - |
| GET | `/ws/queue/:stationId` | Yes | `token` optional fallback | - |
| POST | `/admin/broadcast` | Yes | - | `{ stationId, type, data }` |
| GET | `/admin/stats` | Yes | `stationId` (optional) | - |
| POST | `/admin/test/:stationId` | Yes | - | - |

## Login Flow (recommended)

1. Call `POST /api/v1/auth/login` with CIN.
2. Save `data.token` from response.
3. Use `Authorization: Bearer <token>` for protected endpoints.
4. If you call logout, token becomes invalid because Redis session is deleted.

## Notes

- Queue/Booking services include lightweight Swagger pages (`/docs*`) but they are not complete for every endpoint; this document and the Postman collection cover the full route set from code.
- `websocket-hub` reads `WEBSOCKET_HUB_PORT`; your env file currently defines `WEBSOCKET_SERVICE_PORT`, so hub will use default `8004` unless you add `WEBSOCKET_HUB_PORT`.
