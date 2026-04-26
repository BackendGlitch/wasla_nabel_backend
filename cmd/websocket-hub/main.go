package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"strings"

	"station-backend/internal/websocket"
	"station-backend/pkg/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
	"github.com/redis/go-redis/v9"
)

func main() {
	// Load environment variables
	if err := godotenv.Load("configs/environment.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize WebSocket hub
	hub := websocket.NewHub()
	go hub.Run()

	// Redis PubSub subscriber: fan out events published by other services.
	redisURL := strings.TrimSpace(os.Getenv("REDIS_URL"))
	if redisURL != "" {
		if opt, err := redis.ParseURL(redisURL); err == nil {
			rdb := redis.NewClient(opt)
			channel := strings.TrimSpace(os.Getenv("WS_REDIS_CHANNEL"))
			if channel == "" {
				channel = "wasla:ws:events"
			}

			go func() {
				sub := rdb.Subscribe(context.Background(), channel)
				ch := sub.Channel()
				log.Printf("WebSocket Hub subscribed to Redis channel %s", channel)
				for m := range ch {
					var pm struct {
						Type      string          `json:"type"`
						StationID string          `json:"stationId"`
						Data      json.RawMessage `json:"data"`
						Timestamp int64           `json:"timestamp"`
					}
					if err := json.Unmarshal([]byte(m.Payload), &pm); err != nil {
						continue
					}
					// Pass Data as json.RawMessage so it stays exactly as published.
					hub.BroadcastToStation(pm.StationID, pm.Type, pm.Data)
				}
			}()
		} else {
			log.Printf("WebSocket Hub: invalid REDIS_URL: %v", err)
		}
	}

	// Initialize handler
	wsHandler := websocket.NewHandler(hub)

	// Setup Gin router
	// Set Gin mode based on environment
	if os.Getenv("ENVIRONMENT") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// Middleware
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	// Health check endpoint
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "websocket-hub",
			"clients": hub.GetConnectedClients(),
		})
	})

	// WebSocket endpoint (requires authentication)
	r.GET("/ws/queue/:stationId", middleware.AuthRequired(), wsHandler.WebSocketHandler)

	// Admin endpoints (for testing and management)
	admin := r.Group("/admin")
	admin.Use(middleware.AuthRequired())
	{
		admin.POST("/broadcast", wsHandler.BroadcastMessage)
		admin.GET("/stats", wsHandler.GetConnectionStats)
		admin.POST("/test/:stationId", wsHandler.TestConnection)
	}

	// Get port from environment or use default
	port := os.Getenv("WEBSOCKET_HUB_PORT")
	if port == "" {
		port = "8004"
	}

	log.Printf("WebSocket Hub starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
