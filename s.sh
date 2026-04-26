#!/bin/bash

echo "=== Killing processes on ports 8001-8007 ==="

for PORT in $(seq 8001 8007); do
    # Get the PID listening on this port
    PID=$(sudo ss -lptn "sport = :$PORT" | grep -oP 'pid=\K[0-9]+')
    
    if [ -n "$PID" ]; then
        PROCESS=$(sudo ss -lptn "sport = :$PORT" | grep -oP '"[^"]+"' | head -1)
        echo "Port $PORT → PID $PID ($PROCESS) → killing..."
        sudo kill -9 $PID
    else
        echo "Port $PORT → no process found, skipping."
    fi
done

echo ""
echo "=== Verifying (waiting 1s)... ==="
sleep 1

ALL_CLEAR=true
for PORT in $(seq 8001 8007); do
    PID=$(sudo ss -lptn "sport = :$PORT" | grep -oP 'pid=\K[0-9]+')
    if [ -n "$PID" ]; then
        echo "❌ Port $PORT still has PID $PID running!"
        ALL_CLEAR=false
    else
        echo "✅ Port $PORT is free."
    fi
done

echo ""
if $ALL_CLEAR; then
    echo "✅ All ports 8001-8007 are clear!"
else
    echo "❌ Some ports are still in use. You may need to re-run the script."
fi
