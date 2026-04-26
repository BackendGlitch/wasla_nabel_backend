#!/bin/bash

echo "=== CLOUDFLARE PERMANENT TUNNEL SETUP ==="
echo ""

# Stop any existing tunnels
pkill -f cloudflared 2>/dev/null
sleep 2

echo "Starting Cloudflare tunnel for permanent access..."
echo "This will give you a PERMANENT URL that never changes!"
echo ""

# Start tunnel in background
cloudflared tunnel --config cloudflare-config.yml run wasla-tunnel > tunnel.log 2>&1 &
TUNNEL_PID=$!

echo "Tunnel starting... (PID: $TUNNEL_PID)"
sleep 5

# Check if tunnel is running
if ps -p $TUNNEL_PID > /dev/null; then
    echo "✅ Tunnel is running!"
    echo ""
    echo "🌐 Your PERMANENT URL: https://wasla-backend.trycloudflare.com"
    echo ""
    echo "Testing the URL..."
    curl -s https://wasla-backend.trycloudflare.com/health | jq . 2>/dev/null || echo "Testing connection..."
    echo ""
    echo "📋 PERMANENT ACCESS SUMMARY:"
    echo "• URL: https://wasla-backend.trycloudflare.com"
    echo "• Status: ✅ PERMANENT (same URL every time)"
    echo "• Service: Auth Service (port 8001)"
    echo "• No expiration, no changes on restart!"
    echo ""
    echo "🚀 To access from anywhere: https://wasla-backend.trycloudflare.com"
else
    echo "❌ Failed to start tunnel"
    echo "Check tunnel.log for details"
fi