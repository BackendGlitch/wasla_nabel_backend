## Wasla station backend service map

Ports are configured in `configs/environment.env`.

### Auth service (`:8001`)
- **Health**: `GET /health`
- **Login**: `POST /api/v1/auth/login` (proxied via public-service too)

### Queue service (`:8002`)
- **Routes**: `GET/POST/PUT/DELETE /api/v1/routes`
- **Destinations**: `GET /api/v1/destinations`
- **Queue**: `GET /api/v1/queue/:destinationId` and mutations (`add`, `reorder`, `update`, `delete`, `transfer-seats`, `change-destination`, `clear`)
- **Summaries**: `GET /api/v1/queue-summaries`

### Booking service (`:8003`)
- **Bookings**: `POST /api/v1/bookings`, `POST /api/v1/bookings/by-queue-entry`, cancel endpoints
- **Ghost bookings**: `POST /api/v1/bookings/ghost`, `GET /api/v1/bookings/ghost/count`
- **Trips**: `GET /api/v1/trips`, `GET /api/v1/trips/today`, `GET /api/v1/trips/today/count`

### WebSocket hub (`:8004`)
- **Queue WS**: `GET /ws/queue/:stationId?token=...`
- **Admin**: `/admin/broadcast`, `/admin/stats`, `/admin/test/:stationId`

### Printer service (`:8005`)
- **Printer config**: `GET/PUT /api/printer/config/:id`
- **Printer test**: `POST /api/printer/test/:id`
- **Print queue**: `GET /api/printer/queue`, `GET /api/printer/queue/status`, `POST /api/printer/queue/add`
- **Print**: `POST /api/printer/print/*` (booking/entry/exit/daypass/exitpass/talon)

### Statistics service (`:8006`)
- **Stats HTTP**: `GET /api/v1/statistics/*`
- **Stats WS**: `GET /api/v1/statistics/ws?token=...`
- **Broadcast trigger**: `POST /api/v1/statistics/broadcast`

### Public service (`:8007`)
Public entrypoint for external clients (station info + routes + public booking flow), plus proxy endpoints to auth/statistics/queue/management.\n+
