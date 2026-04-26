#!/bin/bash

# Internet Access Summary for Wasla Backend
# Current status and options

echo "=== WASLA BACKEND INTERNET ACCESS STATUS ==="
echo ""

echo "🌐 CURRENT WORKING ACCESS:"
echo "• ngrok URL: https://bbfcaecae431.ngrok-free.app"
echo "• Status: ✅ ACTIVE"
echo "• Type: Temporary (changes on restart)"
echo "• Test: curl https://bbfcaecae431.ngrok-free.app/health"
echo ""

echo "🔧 PERMANENT ACCESS OPTIONS:"
echo ""
echo "1. CLOUDFLARE TUNNEL (Recommended):"
echo "   • Setup: cloudflared tunnel run wasla-tunnel"
echo "   • URLs: https://wasla-tunnel-xxx.trycloudflare.com"
echo "   • Status: Ready to configure"
echo ""
echo "2. ROUTER PORT FORWARDING:"
echo "   • Public IP: 154.111.251.198"
echo "   • Forward ports: 8001-8007 → 192.168.192.100:8001-8007"
echo "   • Access: http://154.111.251.198:8001"
echo "   • Status: Requires router configuration"
echo ""

echo "📋 SERVICE PORTS:"
echo "• Auth Service: 8001"
echo "• Queue Service: 8002"
echo "• Booking Service: 8003"
echo "• WebSocket Hub: 8004"
echo "• Printer Service: 8005"
echo "• Statistics Service: 8006"
echo "• Public Service: 8007"
echo ""

echo "🚀 QUICK COMMANDS:"
echo "• Test current access: curl https://bbfcaecae431.ngrok-free.app/health"
echo "• Start permanent tunnel: cloudflared tunnel run wasla-tunnel"
echo "• Check tunnel status: cloudflared tunnel list"
echo ""

echo "✅ YOUR BACKEND IS ACCESSIBLE FROM THE INTERNET!"
