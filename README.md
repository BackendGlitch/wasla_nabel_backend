# Station Backend - Transportation Management System

A robust, high-performance backend system built with Go and Gin for managing transportation station operations.

## Architecture

This project uses a microservices architecture with the following services:

- **Auth Service** (Port 8001) - Staff authentication and session management
- **Queue Service** (Port 8002) - Vehicle queue management and real-time updates
- **Booking Service** (Port 8003) - Booking creation and management
- **WebSocket Hub** (Port 8004) - Real-time communication hub
- **Printer Service** (Port 8005) - Printing tickets and managing print queue
- **Statistics Service** (Port 8006) - Staff/station income statistics and reporting
- **Public Service** (Port 8007) - Public/local-node endpoints for station info, route availability, and online booking holds

## Technology Stack

- **Backend**: Go 1.21+ with Gin framework
- **Database**: PostgreSQL with pgx driver
- **Cache**: Redis for session management and real-time events
- **Real-time**: WebSockets with Gorilla WebSocket
- **Authentication**: JWT tokens
- **Deployment**: Docker & Docker Compose

## Features

- **High Performance**: 15,000-30,000 requests/second
- **Real-time Updates**: Sub-millisecond WebSocket broadcasts
- **Queue Management**: Drag-and-drop vehicle reordering
- **Booking System**: Atomic seat reservation with race condition protection
- **Authentication**: JWT-based staff authentication
- **Microservices**: Scalable, independent services

## Quick Start

### Prerequisites

- Go 1.21 or higher
- PostgreSQL 15+
- Redis 7+
- Docker & Docker Compose (optional)

### Local Development

1. **Clone and setup**:
   ```bash
   git clone <repository-url>
   cd station-backend
   make deps
   ```

2. **Start database services**:
   ```bash
   make docker-up
   ```

3. **Run auth service**:
   ```bash
   make run-auth
   ```

4. **Test the service**:
   ```bash
   curl -X POST http://localhost:8001/api/v1/auth/login \
     -H "Content-Type: application/json" \
     -d '{"cin": "12345678"}'
   ```

### Docker Development

1. **Start all services**:
   ```bash
   docker-compose up --build
   ```

2. **Access services**:
   - Auth Service: http://localhost:8001
   - Queue Service: http://localhost:8002
   - Booking Service: http://localhost:8003
   - WebSocket Hub: ws://localhost:8004
   - Printer Service: http://localhost:8005
   - Statistics Service: http://localhost:8006
   - Public Service: http://localhost:8007

### Apply Database Migration

Run this command to apply the public booking hold migration:

```bash
psql "postgresql://ivan:Lost2409@localhost:5432/main-ste?sslmode=disable" -f migrations/008_local_node_booking_hold.sql
```

## API Endpoints

### Auth Service (Port 8001)

- `POST /api/v1/auth/login` - Staff login with CIN
- `POST /api/v1/auth/refresh` - Refresh JWT token
- `POST /api/v1/auth/logout` - Staff logout
- `GET /health` - Health check

### Queue Service (Port 8002)

- `GET /api/v1/queue/:stationId` - Get vehicle queue
- `POST /api/v1/queue/:stationId/vehicle` - Add vehicle to queue
- `PUT /api/v1/queue/:stationId/reorder` - Reorder vehicles
- `PUT /api/v1/queue/:stationId/vehicle/:vehicleId/position` - Update vehicle position
- `DELETE /api/v1/queue/:stationId/vehicle/:vehicleId` - Remove vehicle
- `GET /ws/queue/:stationId` - WebSocket connection for real-time updates

### Booking Service (Port 8003)

- `POST /api/v1/booking/:stationId` - Create booking
- `GET /api/v1/booking/:stationId/:bookingId` - Get booking details
- `PUT /api/v1/booking/:stationId/:bookingId/cancel` - Cancel booking
- `POST /api/v1/booking/:stationId/:bookingId/verify` - Verify booking

### Public Service (Port 8007)

- `POST /info` - Initialize local station metadata
- `GET /info` - Read local station metadata and runtime uptime
- `GET /routes` - Route availability from current queue
- `GET /routes/:id` - Vehicle availability for a destination
- `POST /bookings` - Create booking hold (idempotent)
- `GET /bookings/:id` - Get booking state
- `POST /bookings/:id/confirm` - Confirm hold after payment
- `POST /bookings/:id/cancel` - Cancel hold and release seats
- `POST /api/v1/auth/login` - Login through public-service (proxied to auth-service)
- `GET /api/v1/public-statistics/overview/day?date=YYYY-MM-DD` - Full day overview (actual + staff + stations), auth required
- `GET /api/v1/public-statistics/overview/month?year=YYYY&month=MM` - Full month overview (actual + staff + stations), auth required

`POST /info` can only initialize once. If station config already exists, it returns `200` with `already_configured: true` and a message saying reconfiguration is not allowed.

For `/info` responses, `public_ip` is resolved in this order:
1. Tailscale IPv4 from a local `tailscale*` interface (if present)
2. Public IPv4 from external IP providers
3. Request client IP as final fallback

## Real-time Features

The system provides real-time updates through WebSockets:

- **Vehicle Movement**: Instant updates when vehicles are reordered
- **Booking Creation**: Real-time seat availability updates
- **Queue Changes**: Live queue position updates
- **System Events**: Staff activity and system notifications

## Performance Characteristics

- **Concurrent Connections**: 10,000+ WebSocket connections per server
- **Request Latency**: < 5ms for API requests
- **WebSocket Latency**: < 1ms for real-time updates
- **Database Performance**: Optimized queries with connection pooling
- **Memory Usage**: ~2KB per WebSocket connection

## Development Commands

```bash
# Build all services
make build

# Run specific service
make run-auth

# Run all services
make run-all

# Run tests
make test

# Regenerate + validate API docs/postman route coverage
npm run docs:sync

# Push docs/Wasla_Backend.postman_collection.json to Postman API
npm run postman:push

# Format code
make fmt

# Clean build artifacts
make clean

# Start Docker services
make docker-up

# Stop Docker services
make docker-down
```

## Configuration

Configuration is managed through environment variables:

- `DATABASE_URL` - PostgreSQL connection string
- `REDIS_URL` - Redis connection string
- `JWT_SECRET_KEY` - JWT signing key
- `AUTH_SERVICE_PORT` - Auth service port (default: 8001)
- `QUEUE_SERVICE_PORT` - Queue service port (default: 8002)
- `BOOKING_SERVICE_PORT` - Booking service port (default: 8003)
- `WEBSOCKET_HUB_PORT` - WebSocket hub port (default: 8004)
- `PRINTER_SERVICE_PORT` - Printer service port (default: 8005)
- `STATISTICS_SERVICE_PORT` - Statistics service port (default: 8006)
- `PUBLIC_SERVICE_PORT` - Public service port (default: 8007)
- `PUBLIC_AUTH_PROXY_URL` - Optional auth upstream URL for public-service login proxy (default: `http://localhost:<AUTH_SERVICE_PORT>`)
- `PUBLIC_AUTH_PROXY_TIMEOUT_SECONDS` - Auth upstream response-header timeout for proxy (default: 15)
- `PUBLIC_STATS_PROXY_URL` - Optional stats upstream URL for public-service proxy (default: `http://localhost:<STATISTICS_SERVICE_PORT>`)
- `PUBLIC_STATS_PROXY_TIMEOUT_SECONDS` - Stats upstream response-header timeout for proxy (default: 15)
- `PUBLIC_BOOKING_HOLD_TTL_SECONDS` - Booking hold TTL in seconds (default: 600)
- `PUBLIC_BOOKING_EXPIRE_INTERVAL_SECONDS` - Expiry worker interval in seconds (default: 30)
- `PUBLIC_DOMAIN_AUTOMATION_ENABLED` - Enable automatic Cloudflare domain+tunnel on `/info` init
- `CF_API_TOKEN` - Cloudflare API token (DNS + Tunnel permissions)
- `CF_ACCOUNT_ID` - Cloudflare account id
- `CF_ZONE_ID` - Cloudflare zone id for `backendglitch.com`
- `PUBLIC_DOMAIN_BASE` - Domain root (default: `backendglitch.com`)
- `PUBLIC_DOMAIN_SUBDOMAIN_ROOT` - Middle label; set to `station` for `slug.station.backendglitch.com`, or empty for `slug.backendglitch.com`
- `PUBLIC_DOMAIN_TUNNEL_ORIGIN_URL` - Local service origin for tunnel ingress (default: `http://localhost:8007`)
- `PUBLIC_DOMAIN_START_TUNNEL` - Start/keep `cloudflared` process from API runtime (default: true)
- `PUBLIC_DOMAIN_STATE_FILE` - Local state/token file path
- `PUBLIC_DOMAIN_PID_FILE` - PID file path for managed `cloudflared`
- `PUBLIC_DOMAIN_LOG_FILE` - Log file path for managed `cloudflared`
- `PUBLIC_DOMAIN_CLOUDFLARED_BIN` - `cloudflared` binary name/path
- `POSTMAN_API_KEY` - Postman API key (required for `npm run postman:push` or CI sync workflow)
- `POSTMAN_COLLECTION_UID` - Target Postman collection UID (required for `npm run postman:push` or CI sync workflow)

## Postman Sync Automation

- Workflow: `.github/workflows/postman-sync.yml`
- Trigger: every push to `main` when the collection/script changes, or manual `workflow_dispatch`
- Required GitHub repository secrets:
  - `POSTMAN_API_KEY`
  - `POSTMAN_COLLECTION_UID`

## Database Schema

The system uses PostgreSQL with the following main tables:

- `staff` - Staff members and authentication
- `vehicles` - Vehicle information and queue positions
- `bookings` - Booking records and transactions
- `stations` - Station configuration
- `vehicle_queue` - Queue management with real-time triggers

## Security

- JWT-based authentication
- CORS protection
- Input validation
- SQL injection protection with parameterized queries
- Session management with Redis

## Monitoring

- Health check endpoints for all services
- Structured logging
- Performance metrics
- Error tracking

## Contributing

1. Fork the repository
2. Create a feature branch
3. Make your changes
4. Add tests
5. Submit a pull request

## License

MIT License - see LICENSE file for details
