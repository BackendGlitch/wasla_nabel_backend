package statistics

import (
	"context"
	"log"
	"time"

	"station-backend/pkg/events"
	"station-backend/internal/websocket"
)

// RealTimeStatsHub manages real-time statistics broadcasting
type RealTimeStatsHub struct {
	wsHub   *websocket.Hub
	service *Service
}

// StatsUpdate represents a real-time statistics update
type StatsUpdate struct {
	Type      string      `json:"type"`
	StaffID   string      `json:"staffId,omitempty"`
	StationID string      `json:"stationId,omitempty"`
	Data      interface{} `json:"data"`
	Timestamp time.Time   `json:"timestamp"`
}

// StaffIncomeUpdate represents a staff income update
type StaffIncomeUpdate struct {
	StaffID                 string    `json:"staffId"`
	StaffName               string    `json:"staffName"`
	Date                    time.Time `json:"date"`
	SeatBookings            int       `json:"seatBookings"`
	SeatIncome              float64   `json:"seatIncome"`
	DayPassSales            int       `json:"dayPassSales"`
	DayPassIncome           float64   `json:"dayPassIncome"`
	TotalIncome             float64   `json:"totalIncome"`
	TotalTransactions       int       `json:"totalTransactions"`
	AverageIncomePerSeat    float64   `json:"averageIncomePerSeat"`
	AverageIncomePerDayPass float64   `json:"averageIncomePerDayPass"`
}

// StationIncomeUpdate represents a station income update
type StationIncomeUpdate struct {
	StationID               string    `json:"stationId"`
	StationName             string    `json:"stationName"`
	Date                    time.Time `json:"date"`
	TotalSeatBookings       int       `json:"totalSeatBookings"`
	TotalSeatIncome         float64   `json:"totalSeatIncome"`
	TotalDayPassSales       int       `json:"totalDayPassSales"`
	TotalDayPassIncome      float64   `json:"totalDayPassIncome"`
	TotalIncome             float64   `json:"totalIncome"`
	TotalTransactions       int       `json:"totalTransactions"`
	ActiveStaffCount        int       `json:"activeStaffCount"`
	AverageIncomePerStaff   float64   `json:"averageIncomePerStaff"`
	AverageIncomePerSeat    float64   `json:"averageIncomePerSeat"`
	AverageIncomePerDayPass float64   `json:"averageIncomePerDayPass"`
}

// TransactionUpdate represents a new transaction
type TransactionUpdate struct {
	TransactionID   string    `json:"transactionId"`
	StaffID         string    `json:"staffId"`
	StaffName       string    `json:"staffName"`
	TransactionType string    `json:"transactionType"`
	Amount          float64   `json:"amount"`
	Quantity        int       `json:"quantity"`
	StationID       string    `json:"stationId"`
	StationName     string    `json:"stationName"`
	CreatedAt       time.Time `json:"createdAt"`
}

// NewRealTimeStatsHub creates a new real-time statistics hub
func NewRealTimeStatsHub(service *Service) *RealTimeStatsHub {
	return &RealTimeStatsHub{
		service: service,
	}
}

// SetWebSocketHub sets the websocket hub for broadcasting
func (h *RealTimeStatsHub) SetWebSocketHub(wsHub *websocket.Hub) {
	h.wsHub = wsHub
}

// Run starts the real-time statistics hub
func (h *RealTimeStatsHub) Run() {
	ticker := time.NewTicker(30 * time.Second) // Send periodic updates every 30 seconds
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			h.BroadcastPeriodicUpdates()
		}
	}
}

// BroadcastTransactionUpdate broadcasts a new transaction update
func (h *RealTimeStatsHub) BroadcastTransactionUpdate(update TransactionUpdate) {
	if h.wsHub == nil {
		return
	}

	statsUpdate := StatsUpdate{
		Type:      "transaction_update",
		StaffID:   update.StaffID,
		StationID: update.StationID,
		Data:      update,
		Timestamp: time.Now(),
	}

	// Broadcast to all stations using the existing websocket hub
	h.wsHub.BroadcastToStation("*", events.StatisticsUpdate, statsUpdate)
}

// BroadcastStaffIncomeUpdate broadcasts a staff income update
func (h *RealTimeStatsHub) BroadcastStaffIncomeUpdate(staffID string, update StaffIncomeUpdate) {
	if h.wsHub == nil {
		return
	}

	statsUpdate := StatsUpdate{
		Type:      "staff_income_update",
		StaffID:   staffID,
		Data:      update,
		Timestamp: time.Now(),
	}

	h.wsHub.BroadcastToStation("*", events.StatisticsUpdate, statsUpdate)
}

// BroadcastStationIncomeUpdate broadcasts a station income update
func (h *RealTimeStatsHub) BroadcastStationIncomeUpdate(stationID string, update StationIncomeUpdate) {
	if h.wsHub == nil {
		return
	}

	statsUpdate := StatsUpdate{
		Type:      "station_income_update",
		StationID: stationID,
		Data:      update,
		Timestamp: time.Now(),
	}

	h.wsHub.BroadcastToStation("*", events.StatisticsUpdate, statsUpdate)
}

// BroadcastPeriodicUpdates sends periodic updates to all clients
func (h *RealTimeStatsHub) BroadcastPeriodicUpdates() {
	if h.wsHub == nil || h.service == nil {
		return
	}

	ctx := context.Background()
	now := time.Now()

	// Get today's statistics for all staff
	staffStats, err := h.service.GetAllStaffIncomeForDate(ctx, now)
	if err != nil {
		log.Printf("Error getting staff stats for periodic update: %v", err)
		return
	}

	// Get today's statistics for all stations
	stationStats, err := h.service.GetAllStationIncomeForDate(ctx, now)
	if err != nil {
		log.Printf("Error getting station stats for periodic update: %v", err)
		return
	}

	// Broadcast staff updates
	for _, staffStat := range staffStats {
		update := StaffIncomeUpdate{
			StaffID:                 staffStat.StaffID,
			StaffName:               staffStat.StaffName,
			Date:                    staffStat.Date,
			SeatBookings:            staffStat.SeatBookings,
			SeatIncome:              staffStat.SeatIncome,
			DayPassSales:            staffStat.DayPassSales,
			DayPassIncome:           staffStat.DayPassIncome,
			TotalIncome:             staffStat.TotalIncome,
			TotalTransactions:       staffStat.TotalTransactions,
			AverageIncomePerSeat:    staffStat.AverageIncomePerSeat,
			AverageIncomePerDayPass: staffStat.AverageIncomePerDayPass,
		}
		h.BroadcastStaffIncomeUpdate(staffStat.StaffID, update)
	}

	// Broadcast station updates
	for _, stationStat := range stationStats {
		update := StationIncomeUpdate{
			StationID:               stationStat.StationID,
			StationName:             stationStat.StationName,
			Date:                    stationStat.Date,
			TotalSeatBookings:       stationStat.TotalSeatBookings,
			TotalSeatIncome:         stationStat.TotalSeatIncome,
			TotalDayPassSales:       stationStat.TotalDayPassSales,
			TotalDayPassIncome:      stationStat.TotalDayPassIncome,
			TotalIncome:             stationStat.TotalIncome,
			TotalTransactions:       stationStat.TotalTransactions,
			ActiveStaffCount:        stationStat.ActiveStaffCount,
			AverageIncomePerStaff:   stationStat.AverageIncomePerStaff,
			AverageIncomePerSeat:    stationStat.AverageIncomePerSeat,
			AverageIncomePerDayPass: stationStat.AverageIncomePerDayPass,
		}
		h.BroadcastStationIncomeUpdate(stationStat.StationID, update)
	}
}

// GetConnectedClientsCount returns the number of connected clients
func (h *RealTimeStatsHub) GetConnectedClientsCount() int {
	if h.wsHub == nil {
		return 0
	}
	return h.wsHub.GetConnectedClients()
}
