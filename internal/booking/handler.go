package booking

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	"station-backend/pkg/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

func (h *Handler) Create(c *gin.Context) {
	var req CreateBookingByDestinationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	// Use current logged staff from context
	if staffID, ok := c.Get("staff_id"); ok {
		if sid, ok2 := staffID.(string); ok2 && sid != "" {
			req.StaffID = sid
		}
	}
	b, err := h.service.CreateBookingByDestination(context.Background(), req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to create booking", err)
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Booking created", b)
}

func (h *Handler) CreateByQueueEntry(c *gin.Context) {
	var req CreateBookingByQueueEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	// Use current logged staff from context
	if staffID, ok := c.Get("staff_id"); ok {
		if sid, ok2 := staffID.(string); ok2 && sid != "" {
			req.StaffID = sid
		}
	}
	response, err := h.service.CreateBookingByQueueEntry(context.Background(), req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to create bookings", err)
		return
	}

	utils.SuccessResponse(c, http.StatusCreated, "Bookings created", response)
}

func (h *Handler) CancelOneByQueueEntry(c *gin.Context) {
	var req CancelOneByQueueEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	if staffID, ok := c.Get("staff_id"); ok {
		if sid, ok2 := staffID.(string); ok2 && sid != "" {
			req.StaffID = sid
		}
	}
	b, err := h.service.CancelOneBookingByQueueEntry(context.Background(), req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to cancel booking", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Booking cancelled", b)
}

// CancelLastMine cancels the logged-in operator's most recent active queued (non-ghost) booking.
func (h *Handler) CancelLastMine(c *gin.Context) {
	sid, ok := c.Get("staff_id")
	if !ok {
		utils.UnauthorizedResponse(c, "staff context missing")
		return
	}
	staffID, ok2 := sid.(string)
	if !ok2 || staffID == "" {
		utils.UnauthorizedResponse(c, "invalid staff context")
		return
	}
	b, err := h.service.CancelLastBookingForStaff(context.Background(), staffID)
	if err != nil {
		if errors.Is(err, ErrCancelLastNoBooking) ||
			errors.Is(err, ErrCancelLastQueueMissing) ||
			errors.Is(err, ErrCancelLastExitWindowExpired) ||
			errors.Is(err, ErrCancelLastIncompleteAudit) {
			utils.BadRequestResponse(c, err.Error())
			return
		}
		utils.InternalServerErrorResponse(c, "Failed to cancel last booking", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Last booking cancelled", b)
}

func (h *Handler) Cancel(c *gin.Context) {
	id := c.Param("id")
	var body struct {
		StaffID string  `json:"staffId" binding:"required"`
		Reason  *string `json:"reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	b, err := h.service.CancelBooking(context.Background(), id, body.StaffID, body.Reason)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to cancel booking", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Booking cancelled", b)
}

func (h *Handler) ListTrips(c *gin.Context) {
	// optional: limit query param
	// simple: use default in service/repo
	trips, err := h.service.ListTrips(c.Request.Context(), 0)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list trips", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Trips fetched", trips)
}

func (h *Handler) ListTodayTrips(c *gin.Context) {
	search := c.Query("search")
	limit := 0
	if s := c.Query("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil {
			limit = n
		}
	}
	trips, err := h.service.ListTodayTrips(c.Request.Context(), search, limit)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list today's trips", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Today's trips fetched", trips)
}

func (h *Handler) GetTodayTripsCount(c *gin.Context) {
	// Get destination ID from query parameter
	destinationID := c.Query("destination_id")
	var destID *string
	if destinationID != "" {
		destID = &destinationID
	}

	count, err := h.service.GetTodayTripsCount(c.Request.Context(), destID)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get today's trips count", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Today's trips count fetched", map[string]int{"count": count})
}

func (h *Handler) GetTodayBookedTicketsByDestination(c *gin.Context) {
	destinationID := c.Query("destination_id")
	var destID *string
	if destinationID != "" {
		destID = &destinationID
	}

	items, err := h.service.GetTodayBookedTicketsByDestination(c.Request.Context(), destID)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get today's booked tickets by destination", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Today's booked tickets by destination fetched", items)
}

func (h *Handler) CreateGhostBooking(c *gin.Context) {
	var req CreateGhostBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	// Use current logged staff from context
	if staffID, ok := c.Get("staff_id"); ok {
		if sid, ok2 := staffID.(string); ok2 && sid != "" {
			req.StaffID = sid
		}
	}
	bookings, err := h.service.CreateGhostBooking(context.Background(), req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to create ghost booking", err)
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Ghost bookings created", map[string]interface{}{
		"bookings": bookings,
	})
}

func (h *Handler) GetGhostBookingCount(c *gin.Context) {
	destinationID := c.Query("destination_id")
	count, err := h.service.GetGhostBookingCount(context.Background(), destinationID)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get ghost booking count", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Ghost booking count fetched", map[string]int{"count": count})
}

func (h *Handler) GetTodayTripsCountByLicensePlate(c *gin.Context) {
	licensePlate := c.Query("license_plate")
	if licensePlate == "" {
		utils.BadRequestResponse(c, "license_plate parameter is required")
		return
	}
	count, err := h.service.GetTodayTripsCountByLicensePlate(context.Background(), licensePlate)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get trips count by license plate", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Trips count by license plate fetched", map[string]int{"count": count})
}
