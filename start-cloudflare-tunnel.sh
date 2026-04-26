# Cloudflare Tunnel Configuration for Station Backend
# This creates secure tunnels to your services without router configuration

# Create tunnel (run this once to authenticate)
# cloudflared tunnel login
# cloudflared tunnel create station-backend

# Start tunnel for all services
cloudflared tunnel --config /home/ste/wasla_backend/cloudflare-tunnel.yml run station-backend