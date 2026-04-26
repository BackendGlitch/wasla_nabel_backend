package statistics

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"station-backend/internal/pricing"
)

// StatisticsLogger handles logging of transactions for statistics
type StatisticsLogger struct {
	db          *pgxpool.Pool
	realtimeHub *RealTimeStatsHub
}

// NewStatisticsLogger creates a new statistics logger
func NewStatisticsLogger(db *pgxpool.Pool) *StatisticsLogger {
	return &StatisticsLogger{db: db}
}

// NewStatisticsLoggerWithRealtime creates a new statistics logger with real-time broadcasting
func NewStatisticsLoggerWithRealtime(db *pgxpool.Pool, realtimeHub *RealTimeStatsHub) *StatisticsLogger {
	return &StatisticsLogger{db: db, realtimeHub: realtimeHub}
}

// LogSeatBookingTransaction logs a seat booking transaction
func (sl *StatisticsLogger) LogSeatBookingTransaction(ctx context.Context, staffID, bookingID, stationID string, seats int) error {
	seatIncome := float64(seats) * pricing.ServiceFeePerSeatTND

	_, err := sl.db.Exec(ctx, `
		SELECT log_staff_transaction($1, $2, $3, $4, $5, $6)
	`, staffID, "SEAT_BOOKING", bookingID, seatIncome, seats, stationID)

	if err != nil {
		log.Printf("Failed to log seat booking transaction: %v", err)
		return fmt.Errorf("failed to log seat booking transaction: %w", err)
	}

	// Broadcast real-time update if hub is available
	if sl.realtimeHub != nil {
		// Get staff and station names for the update
		var staffName, stationName string
		sl.db.QueryRow(ctx, `SELECT CONCAT(first_name, ' ', last_name) FROM staff WHERE id = $1`, staffID).Scan(&staffName)
		sl.db.QueryRow(ctx, `SELECT station_name FROM stations WHERE id = $1`, stationID).Scan(&stationName)

		update := TransactionUpdate{
			TransactionID:   bookingID,
			StaffID:         staffID,
			StaffName:       staffName,
			TransactionType: "SEAT_BOOKING",
			Amount:          seatIncome,
			Quantity:        seats,
			StationID:       stationID,
			StationName:     stationName,
			CreatedAt:       time.Now(),
		}

		sl.realtimeHub.BroadcastTransactionUpdate(update)
	}

	return nil
}

// LogDayPassTransaction logs a day pass transaction
func (sl *StatisticsLogger) LogDayPassTransaction(ctx context.Context, staffID, dayPassID, stationID string) error {
	// Day pass income: 2 TND per day pass
	_, err := sl.db.Exec(ctx, `
		SELECT log_staff_transaction($1, $2, $3, $4, $5, $6)
	`, staffID, "DAY_PASS_SALE", dayPassID, 2.0, 1, stationID)

	if err != nil {
		log.Printf("Failed to log day pass transaction: %v", err)
		return fmt.Errorf("failed to log day pass transaction: %w", err)
	}

	// Broadcast real-time update if hub is available
	if sl.realtimeHub != nil {
		// Get staff and station names for the update
		var staffName, stationName string
		sl.db.QueryRow(ctx, `SELECT CONCAT(first_name, ' ', last_name) FROM staff WHERE id = $1`, staffID).Scan(&staffName)
		sl.db.QueryRow(ctx, `SELECT station_name FROM stations WHERE id = $1`, stationID).Scan(&stationName)

		update := TransactionUpdate{
			TransactionID:   dayPassID,
			StaffID:         staffID,
			StaffName:       staffName,
			TransactionType: "DAY_PASS_SALE",
			Amount:          2.0,
			Quantity:        1,
			StationID:       stationID,
			StationName:     stationName,
			CreatedAt:       time.Now(),
		}

		sl.realtimeHub.BroadcastTransactionUpdate(update)
	}

	return nil
}

// LogSeatBookingTransactionAsync logs a seat booking transaction asynchronously
func (sl *StatisticsLogger) LogSeatBookingTransactionAsync(staffID, bookingID, stationID string, seats int) {
	go func() {
		ctx := context.Background()
		if err := sl.LogSeatBookingTransaction(ctx, staffID, bookingID, stationID, seats); err != nil {
			log.Printf("Async seat booking transaction logging failed: %v", err)
		}

		// Trigger statistics service to broadcast update
		// This will notify all connected WebSocket clients
		go func() {
			time.Sleep(100 * time.Millisecond) // Small delay to ensure DB write is complete
			sl.triggerStatisticsBroadcast()
		}()
	}()
}

// LogDayPassTransactionAsync logs a day pass transaction asynchronously
func (sl *StatisticsLogger) LogDayPassTransactionAsync(staffID, dayPassID, stationID string) {
	go func() {
		ctx := context.Background()
		if err := sl.LogDayPassTransaction(ctx, staffID, dayPassID, stationID); err != nil {
			log.Printf("Async day pass transaction logging failed: %v", err)
		}

		// Trigger statistics service to broadcast update
		go func() {
			time.Sleep(100 * time.Millisecond)
			sl.triggerStatisticsBroadcast()
		}()
	}()
}

// triggerStatisticsBroadcast triggers the statistics service to broadcast updates
func (sl *StatisticsLogger) triggerStatisticsBroadcast() {
	// Call the statistics service endpoint to trigger a broadcast
	statisticsURL := "http://127.0.0.1:8006/api/v1/statistics/broadcast"
	resp, err := http.Post(statisticsURL, "application/json", nil)
	if err != nil {
		log.Printf("Failed to trigger statistics broadcast: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Statistics broadcast returned status: %d", resp.StatusCode)
	}
}
