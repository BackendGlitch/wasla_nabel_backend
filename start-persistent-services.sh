#!/bin/bash

echo "🚀 WASLA BACKEND - STARTING PERSISTENT SERVICES"
echo "=============================================="
echo ""

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Function to stop user-level systemd services that may auto-restart binaries
stop_systemd_user_services() {
    if ! command -v systemctl >/dev/null 2>&1; then
        return
    fi

    local active_units
    active_units=$(systemctl --user list-units 'wasla-*.service' --type=service --state=active --no-legend --no-pager 2>/dev/null | awk '{print $1}' | grep -v '^wasla-backend\.service$' || true)

    if [ -z "$active_units" ]; then
        return
    fi

    echo -e "${BLUE}🔄 Stopping active user systemd services...${NC}"
    while IFS= read -r unit; do
        [ -n "$unit" ] || continue
        echo -e "${BLUE}   • Stopping $unit${NC}"
        systemctl --user stop "$unit" >/dev/null 2>&1 || true
    done <<< "$active_units"
}

# Function to force-kill any remaining listeners on service ports
free_service_ports() {
    local ports=(8001 8002 8003 8004 8005 8006 8007)
    local port

    for port in "${ports[@]}"; do
        if command -v fuser >/dev/null 2>&1; then
            if fuser -n tcp "$port" >/dev/null 2>&1; then
                echo -e "${YELLOW}⚠️  Port $port is busy, terminating listener...${NC}"
                fuser -k -n tcp "$port" >/dev/null 2>&1 || true
            fi
        elif command -v lsof >/dev/null 2>&1; then
            local pids
            pids=$(lsof -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null || true)
            if [ -n "$pids" ]; then
                echo -e "${YELLOW}⚠️  Port $port is busy, terminating listener...${NC}"
                kill $pids >/dev/null 2>&1 || true
                sleep 1
                kill -9 $pids >/dev/null 2>&1 || true
            fi
        fi
    done
}

# Function to get listener PID(s) for a TCP port
get_port_pids() {
    local port=$1

    if command -v fuser >/dev/null 2>&1; then
        fuser -n tcp "$port" 2>/dev/null | tr -s ' ' | sed 's/^ //'
        return
    fi

    if command -v lsof >/dev/null 2>&1; then
        lsof -t -iTCP:"$port" -sTCP:LISTEN 2>/dev/null | tr '\n' ' ' | sed 's/ $//'
        return
    fi
}

# Function to check whether a TCP port is in use
port_in_use() {
    local port=$1
    local pids
    pids=$(get_port_pids "$port")
    [ -n "$pids" ]
}

# Function to check if a service is running
check_service() {
    local service_name=$1
    local port=$2
    local pid_file=$3
    local allow_port_only=${4:-1}
    
    if [ -f "$pid_file" ]; then
        local pid=$(cat "$pid_file")
        if ps -p $pid > /dev/null 2>&1; then
            echo -e "${GREEN}✅ $service_name is running (PID: $pid)${NC}"
            return 0
        else
            echo -e "${RED}❌ $service_name PID file exists but process not running${NC}"
            rm -f "$pid_file"
            return 1
        fi
    elif [ "$allow_port_only" = "1" ] && port_in_use "$port"; then
        local pids
        pids=$(get_port_pids "$port")
        echo -e "${GREEN}✅ $service_name is running on port $port (PID: $pids)${NC}"
        return 0
    else
        echo -e "${YELLOW}⚠️  $service_name is not running${NC}"
        return 1
    fi
}

# Function to start a service
start_service() {
    local service_name=$1
    local binary_path=$2
    local port=$3
    local log_file=$4
    local pid_file=$5
    
    echo -e "${BLUE}🔄 Starting $service_name...${NC}"
    
    # Check if already running
    if check_service "$service_name" "$port" "$pid_file" "0"; then
        echo -e "${YELLOW}⚠️  $service_name is already running, skipping...${NC}"
        return 0
    fi

    # Fail fast if another process is already bound to this service port
    if port_in_use "$port"; then
        local pids
        pids=$(get_port_pids "$port")
        echo -e "${RED}❌ Cannot start $service_name: port $port is already in use (PID: $pids)${NC}"
        return 1
    fi
    
    # Start the service with nohup, ensuring we're in the correct directory
    local script_dir="$(cd "$(dirname "$0")" && pwd)"
    cd "$script_dir" || exit 1
    nohup "$binary_path" > "$log_file" 2>&1 &
    local pid=$!
    
    # Save PID to file
    echo $pid > "$pid_file"
    
    # Wait longer and check if it started successfully by checking if the port is listening
    sleep 3
    
    # Check if process is still running
    if ! ps -p $pid > /dev/null 2>&1; then
        echo -e "${RED}❌ Failed to start $service_name${NC}"
        rm -f "$pid_file"
        return 1
    fi
    
    # Check if port is listening (up to 5 more seconds)
    local timeout=5
    while [ $timeout -gt 0 ]; do
        if netstat -tuln 2>/dev/null | grep -q ":$port"; then
            echo -e "${GREEN}✅ $service_name started successfully (PID: $pid, Port: $port)${NC}"
            return 0
        fi
        sleep 1
        timeout=$((timeout - 1))
    done
    
    # If port is not listening but process is running, still consider it a success
    if ps -p $pid > /dev/null 2>&1; then
        echo -e "${GREEN}✅ $service_name started successfully (PID: $pid, Port: $port)${NC}"
        return 0
    else
        echo -e "${RED}❌ Failed to start $service_name${NC}"
        rm -f "$pid_file"
        return 1
    fi
}

# Function to stop all services
stop_all_services() {
    echo -e "${YELLOW}🛑 Stopping all services...${NC}"

    # Stop user-level systemd services that may restart binaries automatically
    stop_systemd_user_services
    
    # Kill all services by PID files
    for pid_file in *.pid; do
        if [ -f "$pid_file" ]; then
            local pid=$(cat "$pid_file")
            local service_name=$(basename "$pid_file" .pid)
            if ps -p $pid > /dev/null 2>&1; then
                echo -e "${BLUE}🔄 Stopping $service_name (PID: $pid)...${NC}"
                kill $pid
                sleep 1
                if ps -p $pid > /dev/null 2>&1; then
                    echo -e "${YELLOW}⚠️  Force killing $service_name...${NC}"
                    kill -9 $pid
                fi
            fi
            rm -f "$pid_file"
        fi
    done
    
    # Also kill any remaining processes by name
    pkill -f "auth-service" 2>/dev/null
    pkill -f "queue-service" 2>/dev/null
    pkill -f "booking-service" 2>/dev/null
    pkill -f "websocket-hub" 2>/dev/null
    pkill -f "printer-service" 2>/dev/null
    pkill -f "statistics-service" 2>/dev/null
    pkill -f "public-service" 2>/dev/null

    # Ensure service ports are free before attempting restart
    free_service_ports
    
    echo -e "${GREEN}✅ All services stopped${NC}"
}

# Function to show status
show_status() {
    echo -e "${BLUE}📊 SERVICE STATUS:${NC}"
    echo ""
    
    check_service "Auth Service" "8001" "auth-service.pid"
    check_service "Queue Service" "8002" "queue-service.pid"
    check_service "Booking Service" "8003" "booking-service.pid"
    check_service "WebSocket Hub" "8004" "websocket-hub.pid"
    check_service "Printer Service" "8005" "printer-service.pid"
    check_service "Statistics Service" "8006" "statistics-service.pid"
    check_service "Public Service" "8007" "public-service.pid"
    
    echo ""
    echo -e "${BLUE}📋 SERVICE PORTS:${NC}"
    echo "• Auth Service:      http://localhost:8001"
    echo "• Queue Service:     http://localhost:8002"
    echo "• Booking Service:   http://localhost:8003"
    echo "• WebSocket Hub:     ws://localhost:8004"
    echo "• Printer Service:   http://localhost:8005"
    echo "• Statistics Service: http://localhost:8006"
    echo "• Public Service:    http://localhost:8007"
}

# Function to test services
test_services() {
    echo -e "${BLUE}🧪 TESTING SERVICES:${NC}"
    echo ""
    
    local services=(
        "Auth Service:8001:/health"
        "Queue Service:8002:/health"
        "Booking Service:8003:/health"
        "WebSocket Hub:8004:/health"
        "Printer Service:8005:/health"
        "Statistics Service:8006:/health"
        "Public Service:8007:/health"
    )
    
    for service_info in "${services[@]}"; do
        IFS=':' read -r name port endpoint <<< "$service_info"
        echo -n "Testing $name (port $port)... "
        
        if curl -s "http://localhost:$port$endpoint" > /dev/null 2>&1; then
            echo -e "${GREEN}✅ OK${NC}"
        else
            echo -e "${RED}❌ FAILED${NC}"
        fi
    done
}

# Main execution
case "${1:-start}" in
    "start")
        echo -e "${BLUE}🚀 Starting all services persistently...${NC}"
        echo ""
        
        # Stop any existing services first
        stop_all_services
        sleep 2
        
        # Ensure we're in the correct directory
        cd "$(dirname "$0")" || exit 1

        # Apply DB migrations before bringing services up
        if [ -x "./scripts/apply-migrations.sh" ]; then
            echo -e "${BLUE}🗄️  Applying database migrations...${NC}"
            ./scripts/apply-migrations.sh || {
                echo -e "${RED}❌ Migration apply failed; refusing to start services.${NC}"
                exit 1
            }
            echo ""
        fi
        
        # Start all services (binaries are in bin/ directory)
        start_service "Auth Service" "./bin/auth-service" "8001" "auth-service.log" "auth-service.pid"
        start_service "Queue Service" "./bin/queue-service" "8002" "queue-service.log" "queue-service.pid"
        start_service "Booking Service" "./bin/booking-service" "8003" "booking-service.log" "booking-service.pid"
        start_service "WebSocket Hub" "./bin/websocket-hub" "8004" "websocket-hub.log" "websocket-hub.pid"
        start_service "Printer Service" "./bin/printer-service" "8005" "printer-service.log" "printer-service.pid"
        start_service "Statistics Service" "./bin/statistics-service" "8006" "statistics-service.log" "statistics-service.pid"
        start_service "Public Service" "./bin/public-service" "8007" "public-service.log" "public-service.pid"
        
        echo ""
        echo -e "${GREEN}🎉 ALL SERVICES STARTED PERSISTENTLY!${NC}"
        echo ""
        echo -e "${BLUE}💡 Services will continue running even if you close the terminal${NC}"
        echo -e "${BLUE}💡 Use './start-persistent-services.sh status' to check status${NC}"
        echo -e "${BLUE}💡 Use './start-persistent-services.sh stop' to stop all services${NC}"
        echo -e "${BLUE}💡 Use './start-persistent-services.sh test' to test services${NC}"
        echo ""
        show_status
        ;;
        
    "stop")
        stop_all_services
        ;;
        
    "restart")
        echo -e "${BLUE}🔄 Restarting all services...${NC}"
        stop_all_services
        sleep 3
        $0 start
        ;;
        
    "status")
        show_status
        ;;
        
    "test")
        test_services
        ;;
        
    "logs")
        echo -e "${BLUE}📋 SERVICE LOGS:${NC}"
        echo ""
        echo "Available log files:"
        ls -la *.log 2>/dev/null || echo "No log files found"
        echo ""
        echo "To view logs:"
        echo "• tail -f auth-service.log"
        echo "• tail -f queue-service.log"
        echo "• tail -f booking-service.log"
        echo "• tail -f websocket-hub.log"
        echo "• tail -f printer-service.log"
        echo "• tail -f statistics-service.log"
        echo "• tail -f public-service.log"
        ;;
        
    *)
        echo -e "${BLUE}📖 USAGE:${NC}"
        echo ""
        echo "$0 [command]"
        echo ""
        echo "Commands:"
        echo "  start    - Start all services persistently (default)"
        echo "  stop     - Stop all services"
        echo "  restart  - Restart all services"
        echo "  status   - Show service status"
        echo "  test     - Test all services"
        echo "  logs     - Show available log files"
        echo ""
        echo "Examples:"
        echo "  $0              # Start all services"
        echo "  $0 start        # Start all services"
        echo "  $0 status       # Check status"
        echo "  $0 test         # Test services"
        echo "  $0 stop         # Stop all services"
        ;;
esac
