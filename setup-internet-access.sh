#!/bin/bash

# Quick Internet Access Setup for Station Backend
# This script provides multiple options for external access

echo "=== STATION BACKEND INTERNET ACCESS SETUP ==="
echo ""

echo "Your server details:"
echo "• Public IP: 154.111.251.198"
echo "• Local IP: 192.168.192.100"
echo "• Services running on ports: 8001-8007"
echo ""

echo "Choose your preferred method:"
echo ""
echo "1. QUICK TEST with ngrok (temporary URLs):"
echo "   ngrok http 8001 --log=stdout"
echo "   ngrok http 8002 --log=stdout"
echo "   ngrok http 8003 --log=stdout"
echo ""
echo "2. PERMANENT with Cloudflare Tunnel:"
echo "   ./start-cloudflare-tunnel.sh"
echo ""
echo "3. ROUTER PORT FORWARDING:"
echo "   Configure your router to forward:"
echo "   8001-8007 -> 192.168.192.100:8001-8007"
echo ""

echo "=== QUICK NGROK TEST ==="
echo "Starting ngrok for Auth Service (port 8001)..."
echo "Press Ctrl+C to stop"
echo ""

# Start ngrok for the auth service as an example
ngrok http 8001 --log=stdout
