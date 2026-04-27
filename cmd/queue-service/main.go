package main

import (
	"context"
	"log"
	"os"

	"station-backend/internal/database"
	"station-backend/internal/queue"
	"station-backend/internal/statistics"
	"station-backend/internal/websocket"
	"station-backend/pkg/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load("configs/environment.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// DB
	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal("DB error:", err)
	}
	defer db.Close()

	// Realtime broadcaster (Redis PubSub -> websocket-hub fanout)
	wsHub := websocket.NewRedisBroadcasterFromEnv()

	// Initialize statistics logger
	statsLogger := statistics.NewStatisticsLogger(db.Pool)

	// Repo / Service / Handler
	repo := queue.NewRepository(db.Pool)
	service := queue.NewService(repo, wsHub, statsLogger)
	h := queue.NewHandler(service)
	go service.RunMidnightMaintenance(context.Background())

	r := gin.Default()
	// Set Gin mode based on environment
	if os.Getenv("ENVIRONMENT") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	// Minimal OpenAPI + Swagger UI
	queue.RegisterDocsRoutes(r)

	// Health
	r.GET("/health", func(c *gin.Context) { c.JSON(200, gin.H{"status": "healthy", "service": "queue-service"}) })

	// Bootstrap endpoint (no auth required - called before login)
	initHandler := queue.NewInitHandler(db.Pool)
	r.GET("/api/v1/init", initHandler.GetInit)

	// Serve static assets (company logo, etc.) from repository assets dir.
	// NOTE: historically this pointed to ./static/assets, which can become stale
	// when only ./assets/company-logo.png is updated.
	r.Static("/assets", "./assets")

	api := r.Group("/api/v1", middleware.AuthRequired())
	{
		// Routes
		api.GET("/routes", h.ListRoutes)
		api.POST("/routes", h.CreateRoute)
		api.PUT("/routes/:id", h.UpdateRoute)
		api.DELETE("/routes/:id", h.DeleteRoute)

		// Vehicles
		api.GET("/vehicles", h.ListVehicles)
		api.GET("/vehicles/search", h.SearchVehicles)
		api.POST("/vehicles", h.CreateVehicle)
		api.PUT("/vehicles/:id", h.UpdateVehicle)
		api.DELETE("/vehicles/:id", h.DeleteVehicle)

		// Authorized routes per vehicle
		api.GET("/vehicles/:id/authorized-routes", h.ListAuthorizedRoutes)
		api.POST("/vehicles/:id/authorized-routes", h.AddAuthorizedRoute)
		api.PUT("/vehicles/:id/authorized-routes/:authId", h.UpdateAuthorizedRoute)
		api.DELETE("/vehicles/:id/authorized-routes/:authId", h.DeleteAuthorizedRoute)

		// Queue entries per destination
		api.GET("/queue/:destinationId", h.ListQueue)
		api.POST("/queue/:destinationId", h.AddQueueEntry)
		api.PUT("/queue/:destinationId/reorder", h.ReorderQueue)
		api.PUT("/queue/:destinationId/entry/:id", h.UpdateQueueEntry)
		api.DELETE("/queue/:destinationId/entry/:id", h.DeleteQueueEntry)
		api.PUT("/queue/:destinationId/entry/:id/move", h.MoveEntry)
		api.POST("/queue/:destinationId/transfer-seats", h.TransferSeats)
		api.PUT("/queue/:destinationId/entry/:id/change-destination", h.ChangeDestination)
		api.DELETE("/queue/:destinationId/clear", h.ClearQueue)
		api.DELETE("/queue/clear-all", h.ClearAllQueues)
		api.POST("/maintenance/midnight", h.TriggerMidnightMaintenance)

		// Aggregates
		api.GET("/queue-summaries", h.ListQueueSummaries)
		api.GET("/destinations", h.ListAllDestinations)
		api.GET("/route-summaries", h.ListRouteSummaries)

		// Day Passes (read-only)
		api.GET("/day-passes", h.ListDayPasses)
		api.GET("/day-pass/vehicle/:vehicleId", h.GetVehicleDayPass)
	}

	port := os.Getenv("QUEUE_SERVICE_PORT")
	if port == "" {
		port = "8002"
	}
	log.Printf("Queue service starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("server error:", err)
	}
}
