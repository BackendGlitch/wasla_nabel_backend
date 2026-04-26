#!/bin/bash

# Test script for TM-T20X printer integration
# This script tests the printer service endpoints

echo "🖨️ Testing TM-T20X Printer Integration"
echo "======================================"

# Configuration
PRINTER_SERVICE_URL="http://localhost:8005"
PRINTER_ID="printer1"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

# Function to test endpoint
test_endpoint() {
    local method=$1
    local endpoint=$2
    local data=$3
    local description=$4
    
    echo -n "Testing $description... "
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "%{http_code}" -o /tmp/response.json "$PRINTER_SERVICE_URL$endpoint")
    elif [ "$method" = "POST" ]; then
        response=$(curl -s -w "%{http_code}" -o /tmp/response.json -X POST -H "Content-Type: application/json" -d "$data" "$PRINTER_SERVICE_URL$endpoint")
    elif [ "$method" = "PUT" ]; then
        response=$(curl -s -w "%{http_code}" -o /tmp/response.json -X PUT -H "Content-Type: application/json" -d "$data" "$PRINTER_SERVICE_URL$endpoint")
    fi
    
    http_code="${response: -3}"
    
    if [ "$http_code" = "200" ] || [ "$http_code" = "201" ]; then
        echo -e "${GREEN}✓ Success (HTTP $http_code)${NC}"
        if [ -f /tmp/response.json ]; then
            echo "Response: $(cat /tmp/response.json)"
        fi
    else
        echo -e "${RED}✗ Failed (HTTP $http_code)${NC}"
        if [ -f /tmp/response.json ]; then
            echo "Error: $(cat /tmp/response.json)"
        fi
    fi
    echo ""
}

# Check if printer service is running
echo "Checking if printer service is running..."
if curl -s "$PRINTER_SERVICE_URL/health" > /dev/null; then
    echo -e "${GREEN}✓ Printer service is running${NC}"
else
    echo -e "${RED}✗ Printer service is not running. Please start it first.${NC}"
    echo "Run: cd station-backend && make run-printer"
    exit 1
fi
echo ""

# Test 1: Get printer configuration
test_endpoint "GET" "/api/printer/config/$PRINTER_ID" "" "Get printer configuration"

# Test 2: Update printer configuration
printer_config='{
    "id": "printer1",
    "name": "TM-T20X Thermal Printer",
    "ip": "192.168.192.11",
    "port": 9100,
    "width": 48,
    "timeout": 10000,
    "model": "TM-T20X",
    "enabled": true,
    "isDefault": true
}'
test_endpoint "PUT" "/api/printer/config/$PRINTER_ID" "$printer_config" "Update printer configuration"

# Test 3: Test printer connection
test_endpoint "POST" "/api/printer/test/$PRINTER_ID" "" "Test printer connection"

# Test 4: Get print queue status
test_endpoint "GET" "/api/printer/queue/status" "" "Get print queue status"

# Test 5: Print a test booking ticket
test_ticket='{
    "licensePlate": "TEST-123",
    "destinationName": "Test Destination",
    "seatNumber": 1,
    "verificationCode": "TEST-CODE-123",
    "totalAmount": 15.50,
    "createdBy": "Test User",
    "createdAt": "'$(date -u +"%Y-%m-%dT%H:%M:%SZ")'",
    "stationName": "Test Station",
    "routeName": "Test Route"
}'
test_endpoint "POST" "/api/printer/$PRINTER_ID/print/booking" "$test_ticket" "Print test booking ticket"

# Test 6: Print a test entry ticket
test_endpoint "POST" "/api/printer/$PRINTER_ID/print/entry" "$test_ticket" "Print test entry ticket"

# Test 7: Print a test talon
test_endpoint "POST" "/api/printer/$PRINTER_ID/print/talon" "$test_ticket" "Print test talon"

# Test 8: Add a print job to queue
print_job='{
    "jobType": "booking_ticket",
    "content": "Test print job content",
    "staffName": "Test Staff",
    "priority": 100
}'
test_endpoint "POST" "/api/printer/queue/add" "$print_job" "Add print job to queue"

# Test 9: Get print queue
test_endpoint "GET" "/api/printer/queue" "" "Get print queue"

echo "======================================"
echo "🎉 Printer integration tests completed!"
echo ""
echo "Next steps:"
echo "1. Make sure your TM-T20X printer is connected to 192.168.192.11:9100"
echo "2. Test the printer configuration in the frontend app"
echo "3. Try booking a ticket to see automatic printing"
echo ""
echo "Frontend app: http://localhost:5173"
echo "Printer service: http://localhost:8005"