package queue

import (
	"context"
	"fmt"
	"net/http"

	"station-backend/pkg/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

func NewHandler(service *Service) *Handler { return &Handler{service: service} }

// ===== Routes =====

func (h *Handler) ListRoutes(c *gin.Context) {
	list, err := h.service.ListRoutes(context.Background())
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list routes", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Routes fetched", list)
}

func (h *Handler) CreateRoute(c *gin.Context) {
	var req CreateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	res, err := h.service.CreateRoute(context.Background(), req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to create route", err)
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Route created", res)
}

func (h *Handler) UpdateRoute(c *gin.Context) {
	id := c.Param("id")
	var req UpdateRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	res, err := h.service.UpdateRoute(context.Background(), id, req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to update route", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Route updated", res)
}

func (h *Handler) DeleteRoute(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteRoute(context.Background(), id); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to delete route", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Route deleted", nil)
}

// ===== Vehicles =====

func (h *Handler) ListVehicles(c *gin.Context) {
	searchQuery := c.Query("search")
	list, err := h.service.ListVehicles(context.Background(), searchQuery)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list vehicles", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Vehicles fetched", list)
}

// SearchVehicles provides enhanced search functionality
func (h *Handler) SearchVehicles(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		utils.BadRequestResponse(c, "Search query parameter 'q' is required")
		return
	}

	list, err := h.service.ListVehicles(context.Background(), query)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to search vehicles", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Vehicles found", list)
}

func (h *Handler) CreateVehicle(c *gin.Context) {
	var req CreateVehicleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	res, err := h.service.CreateVehicle(context.Background(), req)
	if err != nil {
		if err == ErrInvalidLicensePlate {
			utils.BadRequestResponse(c, err.Error())
			return
		}
		utils.InternalServerErrorResponse(c, "Failed to create vehicle", err)
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Vehicle created", res)
}

func (h *Handler) UpdateVehicle(c *gin.Context) {
	id := c.Param("id")
	var req UpdateVehicleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	res, err := h.service.UpdateVehicle(context.Background(), id, req)
	if err != nil {
		if err == ErrInvalidLicensePlate {
			utils.BadRequestResponse(c, err.Error())
			return
		}
		utils.InternalServerErrorResponse(c, "Failed to update vehicle", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Vehicle updated", res)
}

func (h *Handler) DeleteVehicle(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteVehicle(context.Background(), id); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to delete vehicle", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Vehicle deleted", nil)
}

// ===== Authorized routes =====

func (h *Handler) ListAuthorizedRoutes(c *gin.Context) {
	vehicleID := c.Param("id")
	list, err := h.service.ListAuthorizedRoutes(context.Background(), vehicleID)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list authorized routes", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Authorized routes fetched", list)
}

func (h *Handler) AddAuthorizedRoute(c *gin.Context) {
	vehicleID := c.Param("id")
	var req AddAuthorizedRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	res, err := h.service.AddAuthorizedRoute(context.Background(), vehicleID, req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to add authorized route", err)
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Authorized route added", res)
}

func (h *Handler) UpdateAuthorizedRoute(c *gin.Context) {
	authID := c.Param("authId")
	var req UpdateAuthorizedRouteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	res, err := h.service.UpdateAuthorizedRoute(context.Background(), authID, req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to update authorized route", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Authorized route updated", res)
}

func (h *Handler) DeleteAuthorizedRoute(c *gin.Context) {
	authID := c.Param("authId")
	if err := h.service.DeleteAuthorizedRoute(context.Background(), authID); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to delete authorized route", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Authorized route deleted", nil)
}

// ===== Queue entries =====

func (h *Handler) ListQueue(c *gin.Context) {
	destinationID := c.Param("destinationId")
	// Optional subRoute filter
	var sub *string
	if sr := c.Query("subRoute"); sr != "" {
		sub = &sr
	}
	list, err := h.service.ListQueue(context.Background(), destinationID, sub)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list queue", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Queue fetched", list)
}

// ListQueueSummaries returns aggregated per-destination stats
func (h *Handler) ListQueueSummaries(c *gin.Context) {
	// Get station parameter from query string
	station := c.Query("station")

	list, err := h.service.ListQueueSummaries(context.Background(), station)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list queue summaries", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Queue summaries fetched", list)
}

func (h *Handler) ListAllDestinations(c *gin.Context) {
	list, err := h.service.ListAllDestinations(context.Background())
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list destinations", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Destinations fetched", list)
}

// ListRouteSummaries returns all active routes with aggregated seats from the queue
func (h *Handler) ListRouteSummaries(c *gin.Context) {
	list, err := h.service.ListRouteSummaries(context.Background())
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list route summaries", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Route summaries fetched", list)
}

func (h *Handler) AddQueueEntry(c *gin.Context) {
	var req AddQueueEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	// ensure path dest matches body
	dest := c.Param("destinationId")
	req.DestinationID = dest
	// inject current staff as creator if available
	if staffID, ok := c.Get("staff_id"); ok {
		if sid, ok2 := staffID.(string); ok2 && sid != "" {
			req.CreatedBy = sid
		}
	}
	res, err := h.service.AddQueueEntry(context.Background(), req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to add queue entry", err)
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Queue entry added", res)
}

// GetVehicleDayPass returns the current day pass for a specific vehicle
func (h *Handler) GetVehicleDayPass(c *gin.Context) {
	vehicleID := c.Param("vehicleId")
	if vehicleID == "" {
		utils.BadRequestResponse(c, "Vehicle ID is required")
		return
	}

	dayPass, err := h.service.GetVehicleDayPass(context.Background(), vehicleID)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get vehicle day pass", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Vehicle day pass retrieved", dayPass)
}

func (h *Handler) UpdateQueueEntry(c *gin.Context) {
	id := c.Param("id")
	var req UpdateQueueEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	res, err := h.service.UpdateQueueEntry(context.Background(), id, req)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to update queue entry", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Queue entry updated", res)
}

func (h *Handler) DeleteQueueEntry(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteQueueEntry(context.Background(), id); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to delete queue entry", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Queue entry deleted", nil)
}

func (h *Handler) ReorderQueue(c *gin.Context) {
	destinationID := c.Param("destinationId")
	var req ReorderQueueRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	if err := h.service.ReorderQueue(context.Background(), destinationID, req.EntryIDs); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to reorder queue", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Queue reordered", gin.H{"entryIds": req.EntryIDs})
}

func (h *Handler) MoveEntry(c *gin.Context) {
	destinationID := c.Param("destinationId")
	entryID := c.Param("id")
	var req MoveQueueEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	if err := h.service.MoveEntry(context.Background(), destinationID, entryID, req.NewPosition); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to move entry", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Entry moved", gin.H{"id": entryID, "newPosition": req.NewPosition})
}

func (h *Handler) TransferSeats(c *gin.Context) {
	destinationID := c.Param("destinationId")
	var req TransferSeatsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}

	// Log the request for debugging
	fmt.Printf("TransferSeats request: destinationID=%s, fromEntryID=%s, toEntryID=%s, seats=%d\n",
		destinationID, req.FromEntryID, req.ToEntryID, req.Seats)

	if err := h.service.TransferSeats(context.Background(), destinationID, req.FromEntryID, req.ToEntryID, req.Seats); err != nil {
		fmt.Printf("TransferSeats error: %v\n", err)
		utils.InternalServerErrorResponse(c, "Failed to transfer seats", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Seats transferred", gin.H{"from": req.FromEntryID, "to": req.ToEntryID, "seats": req.Seats})
}

func (h *Handler) ChangeDestination(c *gin.Context) {
	oldDest := c.Param("destinationId")
	entryID := c.Param("id")
	var req ChangeDestinationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	if err := h.service.ChangeDestination(context.Background(), entryID, oldDest, req.NewDestinationID, req.NewDestinationName); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to change destination", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Destination changed", gin.H{"id": entryID, "destinationId": req.NewDestinationID})
}

func (h *Handler) ListDayPasses(c *gin.Context) {
	passes, err := h.service.ListDayPasses(c.Request.Context(), 0)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list day passes", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Day passes fetched", passes)
}

func (h *Handler) ClearQueue(c *gin.Context) {
	destinationID := c.Param("destinationId")
	if destinationID == "" {
		utils.BadRequestResponse(c, "Destination ID is required")
		return
	}
	if err := h.service.ClearQueue(context.Background(), destinationID); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to clear queue", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Queue cleared", gin.H{"destinationId": destinationID})
}

func (h *Handler) ClearAllQueues(c *gin.Context) {
	if err := h.service.ClearAllQueues(context.Background()); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to clear all queues", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "All queues cleared", nil)
}

func (h *Handler) TriggerMidnightMaintenance(c *gin.Context) {
	expiredPasses, clearedQueueEntries, err := h.service.TriggerMidnightMaintenance(c.Request.Context())
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to run midnight maintenance", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Midnight maintenance completed", gin.H{
		"expiredDayPasses":     expiredPasses,
		"clearedRegularQueues": clearedQueueEntries,
	})
}
