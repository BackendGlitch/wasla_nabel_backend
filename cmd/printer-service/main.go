package main

import (
	"context"
	"log"
	"os"

	"station-backend/internal/database"
	"station-backend/internal/printer"
	"station-backend/internal/queue"
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

	// Initialize Redis for printer service
	redis, err := database.NewRedis()
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer redis.Close()

	// Initialize PostgreSQL for queue service
	db, err := database.NewPostgres()
	if err != nil {
		log.Fatal("DB error:", err)
	}
	defer db.Close()

	// Initialize repository
	printerRepo := printer.NewRepository(redis.Client)
	auditRepo := printer.NewAuditRepository(db.Pool)
	jobsRepo := printer.NewPrintJobsRepository(db.Pool)

	// Initialize queue repository and service
	queueRepo := queue.NewRepository(db.Pool)
	wsHub := websocket.NewHub()
	statsLogger := statistics.NewStatisticsLogger(db.Pool)
	queueService := queue.NewService(queueRepo, wsHub, statsLogger)

	// Initialize service
	printerService := printer.NewService(printerRepo, auditRepo, jobsRepo)
	printerService.StartPrintWorker(context.Background())

	// Initialize handler
	printerHandler := printer.NewHandler(printerService, queueService)

	// Setup Gin router
	// Set Gin mode based on environment
	if os.Getenv("ENVIRONMENT") == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.Default()

	// Middleware
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())

	// Printer routes
	printerGroup := r.Group("/api/printer")
	{
		// Configuration routes
		printerGroup.GET("/config/:id", printerHandler.GetPrinterConfig)
		printerGroup.PUT("/config/:id", printerHandler.UpdatePrinterConfig)
		printerGroup.POST("/test/:id", printerHandler.TestPrinterConnection)

		// Queue routes
		printerGroup.GET("/queue", printerHandler.GetPrintQueue)
		printerGroup.GET("/queue/status", printerHandler.GetPrintQueueStatus)
		printerGroup.POST("/queue/add", printerHandler.AddPrintJob)

		// Durable print jobs (Postgres)
		printerGroup.GET("/jobs", printerHandler.ListPrintJobs)
		printerGroup.GET("/jobs/:id", printerHandler.GetPrintJob)

		// Print routes (backend_tcp delivery — used by management app, unchanged).
		printerGroup.POST("/print/booking", printerHandler.PrintBookingTicket)
		printerGroup.POST("/print/entry", printerHandler.PrintEntryTicket)
		printerGroup.POST("/print/exit", printerHandler.PrintExitTicket)
		printerGroup.POST("/print/daypass", printerHandler.PrintDayPassTicket)
		printerGroup.POST("/print/exitpass", printerHandler.PrintExitPassTicket)
		printerGroup.POST("/print/exitpass-and-remove", printerHandler.PrintExitPassAndRemoveFromQueue)
		printerGroup.POST("/print/talon", printerHandler.PrintTalon)

		// Render routes (client_local delivery — used by POS staff machines).
		// Backend renders ESC/POS bytes; the POS Electron app writes them to USB
		// and then POSTs /jobs/:id/ack with ok=true|false. Strictly additive:
		// the existing /print/* flow is untouched.
		printerGroup.POST("/render/:type", printerHandler.RenderTicket)
		printerGroup.POST("/jobs/:id/ack", printerHandler.AckPrintJob)
	}

	// Health check
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok", "service": "printer-service"})
	})

	// Get port from environment or use default
	port := os.Getenv("PRINTER_SERVICE_PORT")
	if port == "" {
		port = "8084"
	}

	log.Printf("Printer service starting on port %s", port)
	log.Fatal(r.Run(":" + port))
}
