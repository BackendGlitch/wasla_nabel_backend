package main

import (
	"context"
	"log"
	"os"

	"strconv"
	"time"

	"station-backend/internal/database"
	"station-backend/internal/publicapi"
	"station-backend/pkg/middleware"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func durationSecondsFromEnv(key string, fallback int) time.Duration {
	raw := os.Getenv(key)
	if raw == "" {
		return time.Duration(fallback) * time.Second
	}
	v, err := strconv.Atoi(raw)
	if err != nil || v <= 0 {
		return time.Duration(fallback) * time.Second
	}
	return time.Duration(v) * time.Second
}

func main() {
	if err := godotenv.Load("configs/environment.env", ".env"); err != nil {
		log.Printf("Warning: Could not load .env file: %v", err)
	}

	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	holdTTL := durationSecondsFromEnv("PUBLIC_BOOKING_HOLD_TTL_SECONDS", 600)
	expireInterval := durationSecondsFromEnv("PUBLIC_BOOKING_EXPIRE_INTERVAL_SECONDS", 30)
	startedAt := time.Now()

	repo := publicapi.NewRepository(db.Pool)
	service := publicapi.NewService(repo, holdTTL, expireInterval, startedAt)
	service.SetDomainManager(publicapi.NewCloudflareDomainManagerFromEnv())
	h := publicapi.NewHandler(service)
	authProxy, err := publicapi.NewAuthProxyFromEnv()
	if err != nil {
		log.Printf("Warning: auth proxy disabled: %v", err)
	} else {
		h.SetAuthProxy(authProxy)
	}
	statsProxy, err := publicapi.NewStatisticsProxyFromEnv()
	if err != nil {
		log.Printf("Warning: statistics proxy disabled: %v", err)
	} else {
		h.SetStatisticsProxy(statsProxy)
	}
	queueProxy, err := publicapi.NewQueueProxyFromEnv()
	if err != nil {
		log.Printf("Warning: queue proxy disabled: %v", err)
	} else {
		h.SetQueueProxy(queueProxy)
	}

	workerCtx, cancelWorker := context.WithCancel(context.Background())
	defer cancelWorker()
	service.StartExpiryWorker(workerCtx)
	service.EnsureDomainReady(workerCtx)

	r := gin.Default()
	if os.Getenv("ENVIRONMENT") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"service": "public-service",
		})
	})

	r.POST("/info", h.InitializeInfo)
	r.GET("/info", h.GetInfo)
	r.GET("/routes", h.ListRoutes)
	r.GET("/routes/:id", h.GetRoute)
	r.POST("/bookings", h.CreateBooking)
	r.GET("/bookings/:id", h.GetBooking)
	r.POST("/bookings/:id/confirm", h.ConfirmBooking)
	r.POST("/bookings/:id/cancel", h.CancelBooking)
	r.POST("/api/v1/auth/login", h.ProxyAuthLogin)

	stats := r.Group("/api/v1/statistics", middleware.AuthRequired())
	{
		stats.Any("", h.ProxyStatistics)
		stats.Any("/*path", h.ProxyStatistics)
	}

	publicStats := r.Group("/api/v1/public-statistics", middleware.AuthRequired())
	{
		publicStats.GET("/overview/day", h.ProxyStatisticsOverviewDay)
		publicStats.GET("/overview/month", h.ProxyStatisticsOverviewMonth)
		publicStats.GET("/ws", h.ProxyStatisticsWebSocket)
	}

	publicStaff := r.Group("/api/v1/public-staff", middleware.AuthRequired())
	{
		publicStaff.GET("", h.ProxyStaffList)
		publicStaff.GET("/:id", h.ProxyStaffGet)
		publicStaff.POST("", h.ProxyStaffCreate)
		publicStaff.PUT("/:id", h.ProxyStaffUpdate)
		publicStaff.DELETE("/:id", h.ProxyStaffDelete)
	}

	publicVehicles := r.Group("/api/v1/public-vehicles", middleware.AuthRequired())
	{
		publicVehicles.GET("", h.ProxyVehiclesList)
		publicVehicles.GET("/search", h.ProxyVehiclesSearch)
		publicVehicles.POST("", h.ProxyVehiclesCreate)
		publicVehicles.PUT("/:id", h.ProxyVehiclesUpdate)
		publicVehicles.DELETE("/:id", h.ProxyVehiclesDelete)
		publicVehicles.GET("/:id/authorized-routes", h.ProxyVehicleAuthorizedRoutesList)
		publicVehicles.POST("/:id/authorized-routes", h.ProxyVehicleAuthorizedRoutesAdd)
		publicVehicles.PUT("/:id/authorized-routes/:authId", h.ProxyVehicleAuthorizedRoutesUpdate)
		publicVehicles.DELETE("/:id/authorized-routes/:authId", h.ProxyVehicleAuthorizedRoutesDelete)
	}

	publicRoutes := r.Group("/api/v1/public-routes", middleware.AuthRequired())
	{
		publicRoutes.GET("", h.ProxyRoutesList)
	}

	port := os.Getenv("PUBLIC_SERVICE_PORT")
	if port == "" {
		port = "8007"
	}

	log.Printf("Public service starting on port %s (holdTTL=%s, expireInterval=%s)", port, holdTTL, expireInterval)
	if err := r.Run(":" + port); err != nil {
		log.Fatal("server error:", err)
	}
}
