# API Routes (Generated)

This file is auto-generated from route declarations in Go source code.
Do not edit manually. Run `node scripts/sync-api-route-matrix.js`.

Total routes: **109**

## Auth Service

Base URL: `http://localhost:8001`

| Method | Path | Auth | Source |
|---|---|---|---|
| POST | `/api/v1/auth/login` | No | `cmd/auth-service/main.go:66` |
| POST | `/api/v1/auth/logout` | Yes | `cmd/auth-service/main.go:68` |
| POST | `/api/v1/auth/refresh` | Yes | `cmd/auth-service/main.go:67` |
| GET | `/api/v1/staff` | Yes | `cmd/auth-service/main.go:73` |
| POST | `/api/v1/staff` | Yes | `cmd/auth-service/main.go:75` |
| DELETE | `/api/v1/staff/:id` | Yes | `cmd/auth-service/main.go:77` |
| GET | `/api/v1/staff/:id` | Yes | `cmd/auth-service/main.go:74` |
| PUT | `/api/v1/staff/:id` | Yes | `cmd/auth-service/main.go:76` |
| GET | `/health` | No | `cmd/auth-service/main.go:56` |

## Queue Service

Base URL: `http://localhost:8002`

| Method | Path | Auth | Source |
|---|---|---|---|
| GET | `/api/v1/day-pass/vehicle/:vehicleId` | Yes | `cmd/queue-service/main.go:96` |
| GET | `/api/v1/day-passes` | Yes | `cmd/queue-service/main.go:95` |
| GET | `/api/v1/destinations` | Yes | `cmd/queue-service/main.go:91` |
| GET | `/api/v1/queue-summaries` | Yes | `cmd/queue-service/main.go:90` |
| GET | `/api/v1/queue/:destinationId` | Yes | `cmd/queue-service/main.go:78` |
| POST | `/api/v1/queue/:destinationId` | Yes | `cmd/queue-service/main.go:79` |
| DELETE | `/api/v1/queue/:destinationId/clear` | Yes | `cmd/queue-service/main.go:86` |
| DELETE | `/api/v1/queue/:destinationId/entry/:id` | Yes | `cmd/queue-service/main.go:82` |
| PUT | `/api/v1/queue/:destinationId/entry/:id` | Yes | `cmd/queue-service/main.go:81` |
| PUT | `/api/v1/queue/:destinationId/entry/:id/change-destination` | Yes | `cmd/queue-service/main.go:85` |
| PUT | `/api/v1/queue/:destinationId/entry/:id/move` | Yes | `cmd/queue-service/main.go:83` |
| PUT | `/api/v1/queue/:destinationId/reorder` | Yes | `cmd/queue-service/main.go:80` |
| POST | `/api/v1/queue/:destinationId/transfer-seats` | Yes | `cmd/queue-service/main.go:84` |
| DELETE | `/api/v1/queue/clear-all` | Yes | `cmd/queue-service/main.go:87` |
| GET | `/api/v1/route-summaries` | Yes | `cmd/queue-service/main.go:92` |
| GET | `/api/v1/routes` | Yes | `cmd/queue-service/main.go:59` |
| POST | `/api/v1/routes` | Yes | `cmd/queue-service/main.go:60` |
| DELETE | `/api/v1/routes/:id` | Yes | `cmd/queue-service/main.go:62` |
| PUT | `/api/v1/routes/:id` | Yes | `cmd/queue-service/main.go:61` |
| GET | `/api/v1/vehicles` | Yes | `cmd/queue-service/main.go:65` |
| POST | `/api/v1/vehicles` | Yes | `cmd/queue-service/main.go:67` |
| DELETE | `/api/v1/vehicles/:id` | Yes | `cmd/queue-service/main.go:69` |
| PUT | `/api/v1/vehicles/:id` | Yes | `cmd/queue-service/main.go:68` |
| GET | `/api/v1/vehicles/:id/authorized-routes` | Yes | `cmd/queue-service/main.go:72` |
| POST | `/api/v1/vehicles/:id/authorized-routes` | Yes | `cmd/queue-service/main.go:73` |
| DELETE | `/api/v1/vehicles/:id/authorized-routes/:authId` | Yes | `cmd/queue-service/main.go:75` |
| PUT | `/api/v1/vehicles/:id/authorized-routes/:authId` | Yes | `cmd/queue-service/main.go:74` |
| GET | `/api/v1/vehicles/search` | Yes | `cmd/queue-service/main.go:66` |
| GET | `/docs-all` | No | `internal/queue/docs.go:19` |
| GET | `/docs-queue` | No | `internal/queue/docs.go:13` |
| GET | `/health` | No | `cmd/queue-service/main.go:54` |
| GET | `/openapi-all.json` | No | `internal/queue/docs.go:16` |
| GET | `/openapi-queue.json` | No | `internal/queue/docs.go:10` |

## Booking Service

Base URL: `http://localhost:8003`

| Method | Path | Auth | Source |
|---|---|---|---|
| POST | `/api/v1/bookings` | Yes | `cmd/booking-service/main.go:52` |
| PUT | `/api/v1/bookings/:id/cancel` | Yes | `cmd/booking-service/main.go:55` |
| POST | `/api/v1/bookings/by-queue-entry` | Yes | `cmd/booking-service/main.go:53` |
| POST | `/api/v1/bookings/cancel-one-by-queue-entry` | Yes | `cmd/booking-service/main.go:54` |
| POST | `/api/v1/bookings/ghost` | Yes | `cmd/booking-service/main.go:60` |
| GET | `/api/v1/bookings/ghost/count` | Yes | `cmd/booking-service/main.go:61` |
| GET | `/api/v1/trips` | Yes | `cmd/booking-service/main.go:56` |
| GET | `/api/v1/trips/count-by-license` | Yes | `cmd/booking-service/main.go:63` |
| GET | `/api/v1/trips/today` | Yes | `cmd/booking-service/main.go:57` |
| GET | `/api/v1/trips/today/count` | Yes | `cmd/booking-service/main.go:58` |
| GET | `/docs` | No | `internal/booking/docs.go:14` |
| GET | `/health` | No | `cmd/booking-service/main.go:45` |
| GET | `/openapi.json` | No | `internal/booking/docs.go:11` |

## WebSocket Hub Service

Base URL: `http://localhost:8004`

| Method | Path | Auth | Source |
|---|---|---|---|
| POST | `/admin/broadcast` | Yes | `cmd/websocket-hub/main.go:54` |
| GET | `/admin/stats` | Yes | `cmd/websocket-hub/main.go:55` |
| POST | `/admin/test/:stationId` | Yes | `cmd/websocket-hub/main.go:56` |
| GET | `/health` | No | `cmd/websocket-hub/main.go:39` |
| GET | `/ws/queue/:stationId` | Yes | `cmd/websocket-hub/main.go:48` |

## Printer Service

Base URL: `http://localhost:8005`

| Method | Path | Auth | Source |
|---|---|---|---|
| GET | `/api/printer/config/:id` | No | `cmd/printer-service/main.go:68` |
| PUT | `/api/printer/config/:id` | No | `cmd/printer-service/main.go:69` |
| POST | `/api/printer/print/booking` | No | `cmd/printer-service/main.go:78` |
| POST | `/api/printer/print/daypass` | No | `cmd/printer-service/main.go:81` |
| POST | `/api/printer/print/entry` | No | `cmd/printer-service/main.go:79` |
| POST | `/api/printer/print/exit` | No | `cmd/printer-service/main.go:80` |
| POST | `/api/printer/print/exitpass` | No | `cmd/printer-service/main.go:82` |
| POST | `/api/printer/print/exitpass-and-remove` | No | `cmd/printer-service/main.go:83` |
| POST | `/api/printer/print/talon` | No | `cmd/printer-service/main.go:84` |
| GET | `/api/printer/queue` | No | `cmd/printer-service/main.go:73` |
| POST | `/api/printer/queue/add` | No | `cmd/printer-service/main.go:75` |
| GET | `/api/printer/queue/status` | No | `cmd/printer-service/main.go:74` |
| POST | `/api/printer/test/:id` | No | `cmd/printer-service/main.go:70` |
| GET | `/health` | No | `cmd/printer-service/main.go:88` |

## Statistics Service

Base URL: `http://localhost:8006`

| Method | Path | Auth | Source |
|---|---|---|---|
| POST | `/api/v1/statistics/broadcast` | No | `cmd/statistics-service/main.go:73` |
| GET | `/api/v1/statistics/income/actual` | Yes | `cmd/statistics-service/main.go:76` |
| GET | `/api/v1/statistics/income/day` | Yes | `cmd/statistics-service/main.go:78` |
| GET | `/api/v1/statistics/income/month` | Yes | `cmd/statistics-service/main.go:79` |
| GET | `/api/v1/statistics/income/period` | Yes | `cmd/statistics-service/main.go:77` |
| GET | `/api/v1/statistics/overview/day` | Yes | `cmd/statistics-service/main.go:80` |
| GET | `/api/v1/statistics/overview/month` | Yes | `cmd/statistics-service/main.go:81` |
| GET | `/api/v1/statistics/staff/:staffId/daily` | Yes | `cmd/statistics-service/main.go:54` |
| GET | `/api/v1/statistics/staff/:staffId/range` | Yes | `cmd/statistics-service/main.go:56` |
| GET | `/api/v1/statistics/staff/:staffId/today` | Yes | `cmd/statistics-service/main.go:55` |
| GET | `/api/v1/statistics/staff/:staffId/transactions` | Yes | `cmd/statistics-service/main.go:59` |
| GET | `/api/v1/statistics/staff/all` | Yes | `cmd/statistics-service/main.go:57` |
| GET | `/api/v1/statistics/staff/all-month` | Yes | `cmd/statistics-service/main.go:58` |
| GET | `/api/v1/statistics/station/:stationId/daily` | Yes | `cmd/statistics-service/main.go:62` |
| GET | `/api/v1/statistics/station/:stationId/range` | Yes | `cmd/statistics-service/main.go:64` |
| GET | `/api/v1/statistics/station/:stationId/today` | Yes | `cmd/statistics-service/main.go:63` |
| GET | `/api/v1/statistics/station/:stationId/transactions` | Yes | `cmd/statistics-service/main.go:67` |
| GET | `/api/v1/statistics/station/all` | Yes | `cmd/statistics-service/main.go:65` |
| GET | `/api/v1/statistics/station/all-month` | Yes | `cmd/statistics-service/main.go:66` |
| GET | `/api/v1/statistics/ws` | No | `cmd/statistics-service/main.go:70` |
| GET | `/health` | No | `cmd/statistics-service/main.go:49` |

## Public Service

Base URL: `http://localhost:8007`

| Method | Path | Auth | Source |
|---|---|---|---|
| POST | `/api/v1/auth/login` | No | `cmd/public-service/main.go:88` |
| GET | `/api/v1/public-statistics/overview/day` | Yes | `cmd/public-service/main.go:98` |
| GET | `/api/v1/public-statistics/overview/month` | Yes | `cmd/public-service/main.go:99` |
| GET | `/api/v1/public-statistics/ws` | Yes | `cmd/public-service/main.go:100` |
| ANY | `/api/v1/statistics` | Yes | `cmd/public-service/main.go:92` |
| ANY | `/api/v1/statistics/*path` | Yes | `cmd/public-service/main.go:93` |
| POST | `/bookings` | No | `cmd/public-service/main.go:84` |
| GET | `/bookings/:id` | No | `cmd/public-service/main.go:85` |
| POST | `/bookings/:id/cancel` | No | `cmd/public-service/main.go:87` |
| POST | `/bookings/:id/confirm` | No | `cmd/public-service/main.go:86` |
| GET | `/health` | No | `cmd/public-service/main.go:73` |
| GET | `/info` | No | `cmd/public-service/main.go:81` |
| POST | `/info` | No | `cmd/public-service/main.go:80` |
| GET | `/routes` | No | `cmd/public-service/main.go:82` |
| GET | `/routes/:id` | No | `cmd/public-service/main.go:83` |
