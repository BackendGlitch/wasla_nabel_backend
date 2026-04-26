package websocket

import (
	"net/http"

	"station-backend/pkg/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	hub *Hub
}

func NewHandler(hub *Hub) *Handler {
	return &Handler{hub: hub}
}

// WebSocketHandler handles WebSocket connections
func (h *Handler) WebSocketHandler(c *gin.Context) {
	stationID := c.Param("stationId")
	if stationID == "" {
		utils.BadRequestResponse(c, "Station ID is required")
		return
	}

	// Get staff ID from JWT token (set by AuthRequired middleware)
	staffID, exists := c.Get("staff_id")
	if !exists {
		utils.UnauthorizedResponse(c, "Staff ID not found in context")
		return
	}

	// Upgrade HTTP connection to WebSocket
	ServeWS(h.hub, c.Writer, c.Request, stationID, staffID.(string))
}

// BroadcastMessage handles manual message broadcasting (for testing)
func (h *Handler) BroadcastMessage(c *gin.Context) {
	var req struct {
		StationID string      `json:"stationId" binding:"required"`
		Type      string      `json:"type" binding:"required"`
		Data      interface{} `json:"data" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request format")
		return
	}

	// Broadcast message to station
	h.hub.BroadcastToStation(req.StationID, req.Type, req.Data)

	utils.SuccessResponse(c, http.StatusOK, "Message broadcasted successfully", nil)
}

// GetConnectionStats returns WebSocket connection statistics
func (h *Handler) GetConnectionStats(c *gin.Context) {
	stationID := c.Query("stationId")

	if stationID != "" {
		stats := map[string]interface{}{
			"totalClients":   h.hub.GetConnectedClients(),
			"stationClients": h.hub.GetStationClients(stationID),
			"stationId":      stationID,
		}
		utils.SuccessResponse(c, http.StatusOK, "Connection stats retrieved", stats)
	} else {
		// Return detailed stats for all clients
		stats := h.hub.GetClientStats()
		utils.SuccessResponse(c, http.StatusOK, "Detailed connection stats retrieved", stats)
	}
}

// TestConnection tests WebSocket connection (for development)
func (h *Handler) TestConnection(c *gin.Context) {
	stationID := c.Param("stationId")
	if stationID == "" {
		utils.BadRequestResponse(c, "Station ID is required")
		return
	}

	// Send a test message to all clients in the station
	h.hub.BroadcastToStation(stationID, "test_message", map[string]interface{}{
		"message":   "Hello from WebSocket Hub!",
		"timestamp": utils.GetCurrentTimestamp(),
	})

	utils.SuccessResponse(c, http.StatusOK, "Test message sent", gin.H{
		"stationId": stationID,
		"clients":   h.hub.GetStationClients(stationID),
	})
}
