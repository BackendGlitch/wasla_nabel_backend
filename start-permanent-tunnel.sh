#!/bin/bash

# Start Permanent Cloudflare Tunnel for Wasla Backend
# This creates permanent URLs that don't change

echo "=== STARTING PERMANENT CLOUDFLARE TUNNEL ==="
echo "Tunnel: wasla-tunnel"
echo ""

echo "Your permanent URLs will be:"
echo "• Auth Service: https://auth.wasla-backend.tunnel.cloudflare.com"
echo "• Queue Service: https://queue.wasla-backend.tunnel.cloudflare.com"
echo "• Booking Service: https://booking.wasla-backend.tunnel.cloudflare.com"
echo "• WebSocket Hub: wss://ws.wasla-backend.tunnel.cloudflare.com"
echo "• Printer Service: https://printer.wasla-backend.tunnel.cloudflare.com"
echo "• Statistics Service: https://stats.wasla-backend.tunnel.cloudflare.com"
echo ""

echo "Starting tunnel... (Press Ctrl+C to stop)"
echo ""

# Start the tunnel
cloudflared tunnel --config /home/ste/wasla_backend/wasla-tunnel.yml run wasla-tunnel