package main

import (
	"log"
	"os"

	"station-backend/internal/auth"
	"station-backend/internal/database"
	"station-backend/pkg/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	// Load environment variables
	if err := godotenv.Load("configs/environment.env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	// Initialize database
	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Initialize Redis
	redis, err := database.NewRedis()
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer redis.Close()

	// Initialize repository
	authRepo := auth.NewRepository(db.Pool)

	// Initialize service
	authService := auth.NewService(authRepo, redis.Client)

	// Initialize handler
	authHandler := auth.NewHandler(authService)

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
			"service": "auth-service",
		})
	})

	// Routes
	api := r.Group("/api/v1")
	{
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/refresh", middleware.AuthRequired(), authHandler.RefreshToken)
		api.POST("/auth/logout", middleware.AuthRequired(), authHandler.Logout)

		// Staff CRUD (protected)
		staff := api.Group("/staff", middleware.AuthRequired())
		{
			staff.GET("/", authHandler.ListStaff)
			staff.GET(":id", authHandler.GetStaff)
			staff.POST("/", authHandler.CreateStaff)
			staff.PUT(":id", authHandler.UpdateStaff)
			staff.DELETE(":id", authHandler.DeleteStaff)
		}
	}

	// Get port from environment or use default
	port := os.Getenv("AUTH_SERVICE_PORT")
	if port == "" {
		port = "8001"
	}

	log.Printf("Auth service starting on port %s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
