package main

import (
	"log"
	"os"

	"station-backend/internal/database"
	"station-backend/internal/statistics"
	"station-backend/internal/websocket"
	"station-backend/pkg/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load("configs/environment.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Local websocket hub for statistics websocket endpoint
	wsHub := websocket.NewHub()
	go wsHub.Run()

	repo := statistics.NewRepository(db.Pool)
	service := statistics.NewService(repo)

	// Initialize real-time statistics hub
	realtimeHub := statistics.NewRealTimeStatsHub(service)
	realtimeHub.SetWebSocketHub(wsHub)
	go realtimeHub.Run()

	h := statistics.NewHandler(service, realtimeHub)

	r := gin.Default()
	// Set Gin mode based on environment
	if os.Getenv("ENVIRONMENT") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())
	r.GET("/health", func(c *gin.Context) { c.String(200, "ok") })

	api := r.Group("/api/v1")
	{
		// Staff income endpoints
		api.GET("/statistics/staff/:staffId/daily", middleware.AuthRequired(), h.GetStaffDailyIncome)
		api.GET("/statistics/staff/:staffId/today", middleware.AuthRequired(), h.GetTodayStaffIncome)
		api.GET("/statistics/staff/:staffId/range", middleware.AuthRequired(), h.GetStaffIncomeRange)
		api.GET("/statistics/staff/all", middleware.AuthRequired(), h.GetAllStaffIncomeForDate)
		api.GET("/statistics/staff/all-month", middleware.AuthRequired(), h.GetAllStaffIncomeForMonth)
		api.GET("/statistics/staff/:staffId/transactions", middleware.AuthRequired(), h.GetStaffTransactionLog)

		// Station income endpoints
		api.GET("/statistics/station/:stationId/daily", middleware.AuthRequired(), h.GetStationDailyIncome)
		api.GET("/statistics/station/:stationId/today", middleware.AuthRequired(), h.GetTodayStationIncome)
		api.GET("/statistics/station/:stationId/range", middleware.AuthRequired(), h.GetStationIncomeRange)
		api.GET("/statistics/station/all", middleware.AuthRequired(), h.GetAllStationIncomeForDate)
		api.GET("/statistics/station/all-month", middleware.AuthRequired(), h.GetAllStationIncomeForMonth)
		api.GET("/statistics/station/:stationId/transactions", middleware.AuthRequired(), h.GetStationTransactionLog)

		// Real-time WebSocket endpoint
		api.GET("/statistics/ws", h.WebSocketStats)

		// Trigger real-time broadcast (for cross-service communication)
		api.POST("/statistics/broadcast", h.TriggerBroadcast)

		// Actual income calculation endpoints
		api.GET("/statistics/income/actual", middleware.AuthRequired(), h.GetActualIncome)
		api.GET("/statistics/income/period", middleware.AuthRequired(), h.GetActualIncomeForPeriod)
		api.GET("/statistics/income/day", middleware.AuthRequired(), h.GetActualIncomeForDay)
		api.GET("/statistics/income/month", middleware.AuthRequired(), h.GetActualIncomeForMonth)
		api.GET("/statistics/overview/day", middleware.AuthRequired(), h.GetOverviewForDay)
		api.GET("/statistics/overview/month", middleware.AuthRequired(), h.GetOverviewForMonth)

		// Per-staff per-destination breakdown
		api.GET("/statistics/staff/by-destination", middleware.AuthRequired(), h.GetStaffDestinationBreakdown)
		api.GET("/statistics/staff/by-destination/month", middleware.AuthRequired(), h.GetStaffDestinationBreakdownMonth)
	}

	port := os.Getenv("STATISTICS_SERVICE_PORT")
	if port == "" {
		port = "8006"
	}
	log.Printf("Statistics service starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("server error:", err)
	}
}
