package models

import (
	"time"
)

// Staff represents a staff member
type Staff struct {
	ID        string    `json:"id"`
	CIN       string    `json:"cin"`
	PhoneNumber string  `json:"phoneNumber"`
	FirstName string    `json:"firstName"`
	LastName  string    `json:"lastName"`
	Role      string    `json:"role"` // 'WORKER' or 'SUPERVISOR'
	IsActive  bool      `json:"isActive"`
	LastLogin *time.Time `json:"lastLogin"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// Vehicle represents a vehicle in the queue
type Vehicle struct {
	ID              string    `json:"id"`
	LicensePlate    string    `json:"licensePlate"`
	Capacity        int       `json:"capacity"`
	PhoneNumber     *string   `json:"phoneNumber"`
	IsActive        bool      `json:"isActive"`
	IsAvailable     bool      `json:"isAvailable"`
	IsBanned        bool      `json:"isBanned"`
	CurrentStationID *string  `json:"currentStationId"`
	QueuePosition   *int      `json:"queuePosition"`
	Status          string    `json:"status"` // 'WAITING', 'LOADING', 'READY', 'DEPARTED'
	AvailableSeats  int       `json:"availableSeats"`
	TotalSeats      int       `json:"totalSeats"`
	BasePrice       float64   `json:"basePrice"`
	DestinationID   string    `json:"destinationId"`
	DestinationName string    `json:"destinationName"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

// Booking represents a booking made by a customer
type Booking struct {
	ID                string     `json:"id"`
	VehicleID         string     `json:"vehicleId"`
	SeatsBooked       int        `json:"seatsBooked"`
	TotalAmount       float64    `json:"totalAmount"`
	BookingSource     string     `json:"bookingSource"`
	BookingType       string     `json:"bookingType"`
	BookingStatus     string     `json:"bookingStatus"` // 'ACTIVE', 'CANCELLED', 'COMPLETED', 'REFUNDED'
	PaymentStatus     string     `json:"paymentStatus"`
	PaymentMethod     string     `json:"paymentMethod"`
	VerificationCode  string     `json:"verificationCode"`
	IsVerified        bool       `json:"isVerified"`
	VerifiedAt        *time.Time `json:"verifiedAt"`
	VerifiedByID      *string    `json:"verifiedById"`
	CreatedBy         string     `json:"createdBy"`
	CancelledAt       *time.Time `json:"cancelledAt"`
	CancelledBy       *string    `json:"cancelledBy"`
	CancellationReason *string   `json:"cancellationReason"`
	RefundAmount      *float64   `json:"refundAmount"`
	CreatedAt         time.Time  `json:"createdAt"`
	UpdatedAt         time.Time  `json:"updatedAt"`
}

// Station represents a station
type Station struct {
	ID            string    `json:"id"`
	StationID     string    `json:"stationId"`
	StationName   string    `json:"stationName"`
	Governorate   string    `json:"governorate"`
	Delegation    string    `json:"delegation"`
	Address       *string   `json:"address"`
	OpeningTime   string    `json:"openingTime"`
	ClosingTime   string    `json:"closingTime"`
	IsOperational bool      `json:"isOperational"`
	ServiceFee    float64   `json:"serviceFee"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// WebSocketMessage represents a message sent through WebSocket
type WebSocketMessage struct {
	Type      string      `json:"type"`
	StationID string      `json:"stationId"`
	Data      interface{} `json:"data"`
	Timestamp int64       `json:"timestamp"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	CIN string `json:"cin" binding:"required"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string `json:"token"`
	Staff Staff  `json:"staff"`
}

// ReorderRequest represents a vehicle reorder request
type ReorderRequest struct {
	VehicleIDs []string `json:"vehicleIds" binding:"required"`
}

// AddVehicleRequest represents an add vehicle request
type AddVehicleRequest struct {
	LicensePlate string `json:"licensePlate" binding:"required"`
	Capacity     int    `json:"capacity" binding:"required"`
	Destination  string `json:"destination" binding:"required"`
}

// CreateBookingRequest represents a create booking request
type CreateBookingRequest struct {
	VehicleID string `json:"vehicleId" binding:"required"`
	Seats     int    `json:"seats" binding:"required"`
	StaffID   string `json:"staffId" binding:"required"`
}
