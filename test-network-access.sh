#!/bin/bash

# Network Access Test Script for Station Backend Services
# This script tests if all services are accessible over the network

echo "=== Station Backend Network Access Test ==="
echo "Testing services on $(hostname -I | awk '{print $1}')"
echo ""

# Function to test a service
test_service() {
    local service_name=$1
    local port=$2
    local endpoint=$3
    
    echo -n "Testing $service_name on port $port... "
    
    if curl -s --connect-timeout 5 "http://0.0.0.0:$port$endpoint" > /dev/null 2>&1; then
        echo "✓ OK"
        return 0
    else
        echo "✗ FAILED"
        return 1
    fi
}

# Test all services
echo "Testing service health endpoints:"
echo "--------------------------------"

test_service "Auth Service" "8001" "/health"
test_service "Queue Service" "8002" "/health"
test_service "Booking Service" "8003" "/health"
test_service "WebSocket Hub" "8004" "/health"
test_service "Printer Service" "8005" "/health"
test_service "Statistics Service" "8006" "/health"
test_service "Public Service" "8007" "/health"

echo ""
echo "Testing database services:"
echo "-------------------------"

# Test PostgreSQL
echo -n "Testing PostgreSQL on port 5432... "
if nc -z 0.0.0.0 5432 2>/dev/null; then
    echo "✓ OK"
else
    echo "✗ FAILED"
fi

# Test Redis
echo -n "Testing Redis on port 6379... "
if nc -z 0.0.0.0 6379 2>/dev/null; then
    echo "✓ OK"
else
    echo "✗ FAILED"
fi

echo ""
echo "=== Network Access Instructions ==="
echo "To access services from external machines:"
echo ""
echo "Replace 'localhost' with your server's IP address:"
echo "  Auth Service:      http://YOUR_SERVER_IP:8001"
echo "  Queue Service:     http://YOUR_SERVER_IP:8002"
echo "  Booking Service:   http://YOUR_SERVER_IP:8003"
echo "  WebSocket Hub:     ws://YOUR_SERVER_IP:8004"
echo "  Printer Service:   http://YOUR_SERVER_IP:8005"
echo "  Statistics Service: http://YOUR_SERVER_IP:8006"
echo "  Public Service:    http://YOUR_SERVER_IP:8007"
echo ""
echo "Database connections:"
echo "  PostgreSQL:        YOUR_SERVER_IP:5432"
echo "  Redis:             YOUR_SERVER_IP:6379"
echo ""
echo "Make sure your firewall allows these ports!"
