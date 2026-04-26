#!/bin/bash

# Test script for Station Backend Auth Service

echo "🚀 Testing Station Backend Auth Service"
echo "========================================"

# Check if auth service is running
echo "📡 Checking if auth service is running..."
if curl -s http://localhost:8001/health > /dev/null; then
    echo "✅ Auth service is running"
else
    echo "❌ Auth service is not running. Please start it first:"
    echo "   cd /home/ivan/prod/station-backend"
    echo "   export PATH=\$PATH:/home/ivan/go/bin"
    echo "   ./bin/auth-service"
    exit 1
fi

# Test health endpoint
echo ""
echo "🏥 Testing health endpoint..."
response=$(curl -s http://localhost:8001/health)
echo "Response: $response"

# Test login endpoint (this will fail without proper database setup)
echo ""
echo "🔐 Testing login endpoint..."
login_response=$(curl -s -X POST http://localhost:8001/api/v1/auth/login \
  -H "Content-Type: application/json" \
  -d '{"cin": "12345678"}')

echo "Login response: $login_response"

echo ""
echo "✨ Test completed!"
echo ""
echo "📝 Next steps:"
echo "   1. Set up PostgreSQL database with staff table"
echo "   2. Set up Redis for session management"
echo "   3. Test with real CIN values"
echo "   4. Implement queue and booking services"
