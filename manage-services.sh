#!/bin/bash

# Quick management script for Wasla Backend services
# This provides simple commands to manage the persistent services

case "${1:-help}" in
    "register-station")
        shift
        ./scripts/register-station-cloudflare.sh "$@"
        ;;

    "list-stations")
        shift
        ./scripts/list-stations-cloudflare.sh "$@"
        ;;

    "start"|"up")
        echo " Starting all services..."
        ./start-persistent-services.sh start
        ;;
        
    "stop"|"down")
        echo "Stopping all services..."
        ./start-persistent-services.sh stop
        ;;
        
    "restart")
        echo "Restarting all services..."
        ./start-persistent-services.sh restart
        ;;
        
    "status"|"ps")
        echo " Checking service status..."
        ./start-persistent-services.sh status
        ;;
        
    "test"|"health")
        echo " Testing all services..."
        ./start-persistent-services.sh test
        ;;
        
    "logs")
        echo " Available log files:"
        ./start-persistent-services.sh logs
        echo ""
        echo " To view live logs:"
        echo "   tail -f auth-service.log"
        echo "   tail -f queue-service.log"
        echo "   tail -f booking-service.log"
        echo "   tail -f websocket-hub.log"
        echo "   tail -f printer-service.log"
        echo "   tail -f statistics-service.log"
        echo "   tail -f public-service.log"
        ;;
        
    "build")
        echo "Building all services..."
        make build
        ;;
        
    "clean")
        echo " Cleaning up..."
        ./start-persistent-services.sh stop
        make clean
        ;;
        
    "help"|*)
        echo "WASLA BACKEND SERVICE MANAGER"
        echo "================================"
        echo ""
        echo "Usage: $0 [command]"
        echo ""
        echo "Commands:"
        echo "  start, up     - Start all services persistently"
        echo "  stop, down    - Stop all services"
        echo "  restart      - Restart all services"
        echo "  status, ps    - Show service status"
        echo "  test, health  - Test all services"
        echo "  logs          - Show available log files"
        echo "  build         - Build all services"
        echo "  clean         - Stop services and clean build artifacts"
        echo "  register-station - Init/register station + Cloudflare domain+tunnel"
        echo "  list-stations - List discovered station subdomains from Cloudflare DNS"
        echo "  help          - Show this help"
        echo ""
        echo "Examples:"
        echo "  $0 start      # Start all services"
        echo "  $0 status     # Check if services are running"
        echo "  $0 test       # Test all services"
        echo "  $0 stop       # Stop all services"
        echo "  $0 register-station --name Monastir --location Monastir"
        echo "  $0 list-stations --check-health"
        echo ""
        echo " Services will stay running even if you close the terminal!"
        echo " Use 'tail -f *.log' to monitor service logs"
        ;;
esac
