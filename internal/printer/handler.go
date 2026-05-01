package printer

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"station-backend/internal/pricing"
	"station-backend/internal/queue"
	"station-backend/pkg/middleware"

	"github.com/gin-gonic/gin"
)

// mapTicketType resolves the URL path segment used by /render/:type to the
// internal PrintJobType. Keep this list in sync with the existing /print/*
// endpoints so the two flows always cover the same set of tickets.
func mapTicketType(t string) (PrintJobType, bool) {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "booking":
		return PrintJobTypeBookingTicket, true
	case "entry":
		return PrintJobTypeEntryTicket, true
	case "exit":
		return PrintJobTypeExitTicket, true
	case "daypass":
		return PrintJobTypeDayPassTicket, true
	case "exitpass":
		return PrintJobTypeExitPassTicket, true
	case "talon":
		return PrintJobTypeTalon, true
	default:
		return "", false
	}
}

// Handler handles HTTP requests for printer operations
type Handler struct {
	service      *Service
	queueService *queue.Service
}

// NewHandler creates a new printer handler
func NewHandler(service *Service, queueService *queue.Service) *Handler {
	return &Handler{
		service:      service,
		queueService: queueService,
	}
}

// formatPrinterError provides user-friendly error messages for printer issues
func formatPrinterError(err error) string {
	errorMsg := err.Error()
	if strings.Contains(errorMsg, "dial tcp") || strings.Contains(errorMsg, "connection refused") {
		return "Printer not connected or unreachable. Please check printer power and network connection."
	} else if strings.Contains(errorMsg, "timeout") {
		return "Printer connection timeout. Please check network connectivity."
	}
	return errorMsg
}

// GetPrinterConfig godoc
// @Summary Get printer configuration
// @Description Get the configuration for a specific printer
// @Tags printer
// @Accept json
// @Produce json
// @Param id path string true "Printer ID"
// @Success 200 {object} PrinterConfig
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/config/{id} [get]
func (h *Handler) GetPrinterConfig(c *gin.Context) {
	printerID := c.Param("id")
	if printerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "printer ID is required"})
		return
	}

	config, err := h.service.GetPrinterConfig(printerID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, config)
}

// UpdatePrinterConfig godoc
// @Summary Update printer configuration
// @Description Update the configuration for a specific printer
// @Tags printer
// @Accept json
// @Produce json
// @Param id path string true "Printer ID"
// @Param config body PrinterConfig true "Printer configuration"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/config/{id} [put]
func (h *Handler) UpdatePrinterConfig(c *gin.Context) {
	printerID := c.Param("id")
	if printerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "printer ID is required"})
		return
	}

	var config PrinterConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	config.ID = printerID

	err := h.service.UpdatePrinterConfig(&config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "printer configuration updated successfully"})
}

// TestPrinterConnection godoc
// @Summary Test printer connection
// @Description Test the connection to a specific printer
// @Tags printer
// @Accept json
// @Produce json
// @Param id path string true "Printer ID"
// @Success 200 {object} PrinterStatus
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/test/{id} [post]
func (h *Handler) TestPrinterConnection(c *gin.Context) {
	printerID := c.Param("id")
	if printerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "printer ID is required"})
		return
	}

	err := h.service.TestPrinterConnection(printerID)
	status := &PrinterStatus{
		Connected: err == nil,
		Error:     "",
	}

	if err != nil {
		status.Error = err.Error()
		c.JSON(http.StatusOK, status)
		return
	}

	c.JSON(http.StatusOK, status)
}

// GetPrintQueue godoc
// @Summary Get print queue
// @Description Get the current print queue
// @Tags printer
// @Accept json
// @Produce json
// @Success 200 {array} QueuedPrintJob
// @Failure 500 {object} map[string]string
// @Router /printer/queue [get]
func (h *Handler) GetPrintQueue(c *gin.Context) {
	queue, err := h.service.GetPrintQueue()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, queue)
}

// GetPrintQueueStatus godoc
// @Summary Get print queue status
// @Description Get the current status of the print queue
// @Tags printer
// @Accept json
// @Produce json
// @Success 200 {object} PrintQueueStatus
// @Failure 500 {object} map[string]string
// @Router /printer/queue/status [get]
func (h *Handler) GetPrintQueueStatus(c *gin.Context) {
	status, err := h.service.GetPrintQueueStatus()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, status)
}

// ListPrintJobs returns recent durable print jobs from Postgres
func (h *Handler) ListPrintJobs(c *gin.Context) {
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if h.service == nil || h.service.jobsRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "print jobs disabled"})
		return
	}
	list, err := h.service.jobsRepo.ListRecent(c.Request.Context(), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"jobs": list})
}

// GetPrintJob returns a single durable print job
func (h *Handler) GetPrintJob(c *gin.Context) {
	id := c.Param("id")
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}
	if h.service == nil || h.service.jobsRepo == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "print jobs disabled"})
		return
	}
	job, err := h.service.jobsRepo.GetJob(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, job)
}

// PrintBookingTicket godoc
// @Summary Print booking ticket
// @Description Print a booking ticket using printer config from request
// @Tags printer
// @Accept json
// @Produce json
// @Param ticket body TicketData true "Ticket data with printer config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/print/booking [post]
func (h *Handler) PrintBookingTicket(c *gin.Context) {
	var ticketData TicketData
	if err := c.ShouldBindJSON(&ticketData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobID, err := h.service.EnqueueTicket(c.Request.Context(), &ticketData, PrintJobTypeBookingTicket)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "booking ticket enqueued", "jobId": jobID})
}

// PrintEntryTicket godoc
// @Summary Print entry ticket
// @Description Print an entry ticket using printer config from request
// @Tags printer
// @Accept json
// @Produce json
// @Param ticket body TicketData true "Ticket data with printer config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/print/entry [post]
func (h *Handler) PrintEntryTicket(c *gin.Context) {
	var ticketData TicketData
	if err := c.ShouldBindJSON(&ticketData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobID, err := h.service.EnqueueTicket(c.Request.Context(), &ticketData, PrintJobTypeEntryTicket)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "entry ticket enqueued", "jobId": jobID})
}

// PrintExitTicket godoc
// @Summary Print exit ticket
// @Description Print an exit ticket using printer config from request
// @Tags printer
// @Accept json
// @Produce json
// @Param ticket body TicketData true "Ticket data with printer config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/print/exit [post]
func (h *Handler) PrintExitTicket(c *gin.Context) {
	var ticketData TicketData
	if err := c.ShouldBindJSON(&ticketData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobID, err := h.service.EnqueueTicket(c.Request.Context(), &ticketData, PrintJobTypeExitTicket)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "exit ticket enqueued", "jobId": jobID})
}

// PrintDayPassTicket godoc
// @Summary Print day pass ticket
// @Description Print a day pass ticket using printer config from request
// @Tags printer
// @Accept json
// @Produce json
// @Param ticket body TicketData true "Ticket data with printer config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/print/daypass [post]
func (h *Handler) PrintDayPassTicket(c *gin.Context) {
	var ticketData TicketData
	if err := c.ShouldBindJSON(&ticketData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobID, err := h.service.EnqueueTicket(c.Request.Context(), &ticketData, PrintJobTypeDayPassTicket)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "day pass ticket enqueued", "jobId": jobID})
}

// PrintExitPassTicket godoc
// @Summary Print exit pass ticket
// @Description Print an exit pass ticket using printer config from request
// @Tags printer
// @Accept json
// @Produce json
// @Param ticket body TicketData true "Ticket data with printer config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/print/exitpass [post]
func (h *Handler) PrintExitPassTicket(c *gin.Context) {
	var ticketData TicketData
	if err := c.ShouldBindJSON(&ticketData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobID, err := h.service.EnqueueTicket(c.Request.Context(), &ticketData, PrintJobTypeExitPassTicket)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "exit pass ticket enqueued", "jobId": jobID})
}

// PrintExitPassAndRemoveFromQueue godoc
// @Summary Print exit pass ticket and remove vehicle from queue
// @Description Print an exit pass ticket with booked seats calculation and remove the vehicle from queue
// @Tags printer
// @Accept json
// @Produce json
// @Param request body ExitPassAndRemoveRequest true "Exit pass and remove request with printer config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/print/exitpass-and-remove [post]
func (h *Handler) PrintExitPassAndRemoveFromQueue(c *gin.Context) {
	var request ExitPassAndRemoveRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var totalAmount float64
	var seatNumber int

	// Exit pass: tariff only (booked seats × destination base price). First trip today: − entry day fee.
	grossTariff := 0.0
	if request.BookedSeats > 0 {
		grossTariff = float64(request.BookedSeats) * request.BasePrice
		seatNumber = request.BookedSeats
	}
	totalAmount = grossTariff

	firstTrip := false
	entryDayDeduction := 0.0
	if request.BookedSeats > 0 && grossTariff > 0 {
		nTripsToday, cerr := h.queueService.CountTripsTodayForLicensePlate(c.Request.Context(), request.LicensePlate)
		if cerr != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to resolve first trip today: " + cerr.Error()})
			return
		}
		firstTrip = nTripsToday == 0
		if firstTrip {
			entryDayDeduction = pricing.EntryDayPassFeeTND
			totalAmount = grossTariff - entryDayDeduction
			if totalAmount < 0 {
				totalAmount = 0
			}
		}
	}

	// Create ticket data for printing
	ticketData := &TicketData{
		LicensePlate:    request.LicensePlate,
		DestinationName: request.DestinationName,
		SeatNumber:      seatNumber,
		TotalAmount:     totalAmount,
		CreatedBy:       request.CreatedBy,
		CreatedAt:       time.Now(),
		StationName:     request.StationName,
		RouteName:       request.RouteName,
		VehicleCapacity: request.TotalSeats,
		BasePrice:       request.BasePrice,
		ServiceFee:      0,
		ExitPassCount:   request.ExitPassCount,
		FirstTripOfDay:  firstTrip,
		CompanyName:     request.CompanyName,
		CompanyLogo:     request.CompanyLogo,
		StaffFirstName:  request.StaffFirstName,
		StaffLastName:   request.StaffLastName,
		PrinterConfig:   request.PrinterConfig,
	}

	// Print the exit pass ticket
	err := h.service.PrintExitPassTicket(ticketData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to print exit pass ticket: " + err.Error()})
		return
	}

	// Create a trip record before removing from queue
	if request.QueueEntryID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "queueEntryId is required"})
		return
	}

	// Create trip record with appropriate seat count
	tripSeatsBooked := request.BookedSeats
	if request.BookedSeats == 0 {
		// For empty vehicles, record the vehicle capacity as "seats booked" for trip tracking
		tripSeatsBooked = request.TotalSeats
	}

	if _, tripErr := h.queueService.CreateTripFromExit(context.Background(), request.QueueEntryID, request.LicensePlate, request.DestinationName, tripSeatsBooked, request.TotalSeats, request.BasePrice); tripErr != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create trip record: " + tripErr.Error()})
		return
	}

	// Remove vehicle from queue
	err = h.queueService.DeleteQueueEntry(context.Background(), request.QueueEntryID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to remove vehicle from queue: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":           "exit pass ticket printed and vehicle removed from queue successfully",
		"totalAmount":       totalAmount,
		"grossTariffAmount": grossTariff,
		"entryDayDeduction": entryDayDeduction,
		"firstTripOfDay":    firstTrip,
		"bookedSeats":       request.BookedSeats,
		"basePrice":         request.BasePrice,
		"isEmpty":           request.BookedSeats == 0,
	})
}

// PrintTalon godoc
// @Summary Print talon
// @Description Print a talon using printer config from request
// @Tags printer
// @Accept json
// @Produce json
// @Param ticket body TicketData true "Ticket data with printer config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/print/talon [post]
func (h *Handler) PrintTalon(c *gin.Context) {
	var ticketData TicketData
	if err := c.ShouldBindJSON(&ticketData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobID, err := h.service.EnqueueTicket(c.Request.Context(), &ticketData, PrintJobTypeTalon)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"message": "talon enqueued", "jobId": jobID})
}

// AddPrintJob godoc
// @Summary Add print job to queue
// @Description Add a print job to the print queue
// @Tags printer
// @Accept json
// @Produce json
// @Param job body map[string]interface{} true "Print job data"
// @Success 200 {object} QueuedPrintJob
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/queue/add [post]
func (h *Handler) AddPrintJob(c *gin.Context) {
	var request struct {
		JobType   string `json:"jobType" binding:"required"`
		Content   string `json:"content" binding:"required"`
		StaffName string `json:"staffName"`
		Priority  int    `json:"priority"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	jobType := PrintJobType(request.JobType)
	priority := request.Priority
	if priority == 0 {
		priority = 100 // Default priority
	}

	job, err := h.service.AddPrintJob(jobType, request.Content, request.StaffName, priority)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, job)
}

// RenderTicket godoc
// @Summary Render a ticket for client-local (POS USB) delivery
// @Description Generate ESC/POS bytes for a ticket and persist a print_jobs row
// @Description in 'rendered' status (delivery_mode='client_local'). The caller
// @Description (POS Electron app) writes the bytes to its USB printer and then
// @Description POSTs /api/printer/jobs/{id}/ack with ok=true|false.
// @Tags printer
// @Accept json
// @Produce json
// @Param type path string true "Ticket type: booking|entry|exit|daypass|exitpass|talon"
// @Param ticket body TicketData true "Ticket data (printerConfig is ignored in this flow)"
// @Success 200 {object} RenderResult
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /printer/render/{type} [post]
func (h *Handler) RenderTicket(c *gin.Context) {
	jobType, ok := mapTicketType(c.Param("type"))
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported ticket type: " + c.Param("type")})
		return
	}

	var ticketData TicketData
	if err := c.ShouldBindJSON(&ticketData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if ticketData.CreatedAt.IsZero() {
		ticketData.CreatedAt = time.Now()
	}

	res, err := h.service.RenderTicket(
		c.Request.Context(),
		&ticketData,
		jobType,
		middleware.MachineID(c),
	)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"jobId":          res.JobID,
		"status":         string(res.Status),
		"contentBase64":  res.ContentBase64,
		"alreadyPrinted": res.AlreadyPrinted,
		"deliveryMode":   res.DeliveryMode,
	})
}

// AckPrintJob godoc
// @Summary Acknowledge a client_local print job
// @Description POS device reports back whether the local USB write succeeded.
// @Description Refuses to ack jobs that are not client_local.
// @Tags printer
// @Accept json
// @Produce json
// @Param id path string true "Print job id"
// @Param body body printerAckBody true "Ack payload"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Router /printer/jobs/{id}/ack [post]
func (h *Handler) AckPrintJob(c *gin.Context) {
	id := c.Param("id")
	if strings.TrimSpace(id) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "id is required"})
		return
	}

	var body printerAckBody
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	printedAt := time.Now()
	if body.PrintedAt != nil && !body.PrintedAt.IsZero() {
		printedAt = *body.PrintedAt
	}

	if err := h.service.AckPrintJob(c.Request.Context(), id, body.Ok, body.Error, printedAt); err != nil {
		msg := err.Error()
		if strings.Contains(msg, "no rows") {
			c.JSON(http.StatusNotFound, gin.H{"error": "job not found"})
			return
		}
		if strings.Contains(msg, "not a client_local job") {
			c.JSON(http.StatusConflict, gin.H{"error": msg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "ack recorded", "id": id, "ok": body.Ok})
}

// printerAckBody is the body of POST /api/printer/jobs/:id/ack.
type printerAckBody struct {
	Ok        bool       `json:"ok"`
	Error     string     `json:"error,omitempty"`
	PrintedAt *time.Time `json:"printedAt,omitempty"`
}
