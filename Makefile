.PHONY: build run test clean docker-up docker-down

# Build all services
build:
	@echo "Building all services..."
	go build -o bin/auth-service ./cmd/auth-service
	go build -o bin/queue-service ./cmd/queue-service
	go build -o bin/booking-service ./cmd/booking-service
	go build -o bin/public-service ./cmd/public-service
	go build -o bin/websocket-hub ./cmd/websocket-hub
	go build -o bin/printer-service ./cmd/printer-service
	go build -o bin/statistics-service ./cmd/statistics-service
	@echo "Build complete!"

# Run auth service locally
run-auth:
	@echo "Starting auth service..."
	go run ./cmd/auth-service

# Run queue service locally
run-queue:
	@echo "Starting queue service..."
	go run ./cmd/queue-service

# Run booking service locally
run-booking:
	@echo "Starting booking service..."
	go run ./cmd/booking-service

# Run public service locally
run-public:
	@echo "Starting public service..."
	go run ./cmd/public-service

# Run websocket hub locally
run-websocket:
	@echo "Starting websocket hub..."
	go run ./cmd/websocket-hub

# Run printer service locally
run-printer:
	@echo "Starting printer service..."
	go run ./cmd/printer-service

# Run statistics service locally
run-statistics:
	@echo "Starting statistics service..."
	go run ./cmd/statistics-service

# Run all services
run-all:
	@echo "Starting all services..."
	go run ./cmd/auth-service &
	go run ./cmd/queue-service &
	go run ./cmd/booking-service &
	go run ./cmd/public-service &
	go run ./cmd/websocket-hub &
	go run ./cmd/printer-service &
	go run ./cmd/statistics-service &
	wait

# Test all packages
test:
	@echo "Running tests..."
	go test ./...

# Clean build artifacts
clean:
	@echo "Cleaning build artifacts..."
	rm -rf bin/
	go clean

# Start Docker services
docker-up:
	@echo "Starting Docker services..."
	docker-compose up -d postgres redis

# Stop Docker services
docker-down:
	@echo "Stopping Docker services..."
	docker-compose down

# Build and run auth service with Docker
docker-auth:
	@echo "Building and running auth service with Docker..."
	docker-compose up --build auth-service

# Run database migrations
migrate:
	@echo "Running database migrations..."
	@echo "Applying initial schema..."
	psql -d main-ste -f migrations/001_initial_schema.sql
	@echo "Applying statistics schema..."
	psql -d main-ste -f migrations/002_statistics_schema.sql
	@echo "Applying trips and exit passes schema..."
	psql -d main-ste -f migrations/003_trips_and_exit_passes.sql
	@echo "Applying ghost bookings schema..."
	psql -d main-ste -f migrations/004_ghost_bookings.sql
	@echo "Applying ghost booking FK fix..."
	psql -d main-ste -f migrations/005_fix_ghost_booking_fk.sql
	@echo "Applying staff deletion guard..."
	psql -d main-ste -f migrations/006_prevent_staff_deletion.sql
	@echo "Applying vehicle soft delete..."
	psql -d main-ste -f migrations/007_vehicle_soft_delete.sql
	@echo "Applying public booking hold schema..."
	psql -d main-ste -f migrations/008_local_node_booking_hold.sql
	@echo "Migrations complete!"

# Install dependencies
deps:
	@echo "Installing dependencies..."
	go mod download
	go mod tidy

# Format code
fmt:
	@echo "Formatting code..."
	go fmt ./...

# Lint code
lint:
	@echo "Linting code..."
	golangci-lint run

# Help
help:
	@echo "Available commands:"
	@echo "  build         - Build all services"
	@echo "  run-auth      - Run auth service locally"
	@echo "  run-queue     - Run queue service locally"
	@echo "  run-booking   - Run booking service locally"
	@echo "  run-public    - Run public service locally"
	@echo "  run-websocket - Run websocket hub locally"
	@echo "  run-printer   - Run printer service locally"
	@echo "  run-statistics - Run statistics service locally"
	@echo "  run-all       - Run all services locally"
	@echo "  test          - Run tests"
	@echo "  clean         - Clean build artifacts"
	@echo "  docker-up     - Start Docker services (postgres, redis)"
	@echo "  docker-down   - Stop Docker services"
	@echo "  docker-auth   - Build and run auth service with Docker"
	@echo "  migrate       - Run database migrations"
	@echo "  deps          - Install dependencies"
	@echo "  fmt           - Format code"
	@echo "  lint          - Lint code"
	@echo "  help          - Show this help"
