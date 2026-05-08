package booking

import "time"

type CreateBookingByDestinationRequest struct {
	DestinationID  string  `json:"destinationId" binding:"required"`
	Seats          int     `json:"seats" binding:"required"`
	StaffID        string  `json:"staffId"`
	SubRoute       *string `json:"subRoute"`
	PreferExactFit bool    `json:"preferExactFit"`
	IdempotencyKey string  `json:"idempotencyKey"`
}

type CreateBookingByQueueEntryRequest struct {
	QueueEntryID string `json:"queueEntryId" binding:"required"`
	Seats        int    `json:"seats" binding:"required"`
	StaffID      string `json:"staffId"`
	IdempotencyKey string `json:"idempotencyKey"`
}

type CancelOneByQueueEntryRequest struct {
	QueueEntryID string `json:"queueEntryId" binding:"required"`
	StaffID      string `json:"staffId"`
}

type CreateGhostBookingRequest struct {
	DestinationID string `json:"destinationId" binding:"required"`
	Seats         int    `json:"seats" binding:"required"`
	StaffID       string `json:"staffId"`
	// IdempotencyKey ensures retries/double-clicks don't create multiple ghost bookings.
	// Client should generate a UUID per user action and reuse it for retries.
	IdempotencyKey string `json:"idempotencyKey"`
}

type GhostBooking struct {
	ID               string    `json:"id"`
	DestinationID    string    `json:"destinationId"`
	DestinationName  string    `json:"destinationName"`
	SeatsBooked      int       `json:"seatsBooked"`
	SeatNumber       int       `json:"seatNumber"` // Daily per-destination ghost booking index (0-based, resets at midnight)
	TotalAmount      float64   `json:"totalAmount"`
	BookingStatus    string    `json:"bookingStatus"`
	PaymentStatus    string    `json:"paymentStatus"`
	VerificationCode string    `json:"verificationCode"`
	CreatedBy        string    `json:"createdBy"`
	CreatedByName    string    `json:"createdByName"`
	CreatedAt        time.Time `json:"createdAt"`
	// Ghost-specific fields
	IsGhostBooking bool    `json:"isGhostBooking"`
	BasePrice      float64 `json:"basePrice"`
}

type Booking struct {
	ID               string    `json:"id"`
	QueueID          string    `json:"queueId"`
	VehicleID        string    `json:"vehicleId"`
	LicensePlate     string    `json:"licensePlate"`
	SeatsBooked      int       `json:"seatsBooked"`
	SeatNumber       int       `json:"seatNumber"` // Individual seat number for this booking
	TotalAmount      float64   `json:"totalAmount"`
	BookingStatus    string    `json:"bookingStatus"`
	PaymentStatus    string    `json:"paymentStatus"`
	VerificationCode string    `json:"verificationCode"`
	CreatedBy        string    `json:"createdBy"`
	CreatedByName    string    `json:"createdByName"` // Staff name instead of just ID
	CreatedAt        time.Time `json:"createdAt"`
	// FirstTripOfDay is true for every talon until this license plate has its first trips row today (any destination/queue); counting uses trips.license_plate so it still works after the queue row is deleted and the vehicle re-queues elsewhere.
	FirstTripOfDay bool `json:"firstTripOfDay,omitempty"`
}

type CreateBookingByQueueEntryResponse struct {
	Bookings    []Booking `json:"bookings"`
	ExitPass    *ExitPass `json:"exitPass,omitempty"`
	HasExitPass bool      `json:"hasExitPass"`
}

type PreviousVehicle struct {
	LicensePlate string    `json:"licensePlate"`
	ExitTime     time.Time `json:"exitTime"`
}

type ExitPass struct {
	ID              string    `json:"id"`
	QueueID         string    `json:"queueId"`
	VehicleID       string    `json:"vehicleId"`
	LicensePlate    string    `json:"licensePlate"`
	DestinationID   string    `json:"destinationId"`
	DestinationName string    `json:"destinationName"`
	CurrentExitTime time.Time `json:"currentExitTime"` // Current vehicle exit time
	TotalPrice      float64   `json:"totalPrice"`      // Base price × vehicle capacity
	CreatedBy       string    `json:"createdBy"`
	CreatedByName   string    `json:"createdByName"` // Staff name
	CreatedAt       time.Time `json:"createdAt"`
	// Vehicle and pricing information for ticket generation
	VehicleCapacity int     `json:"vehicleCapacity"` // Vehicle capacity
	BasePrice       float64 `json:"basePrice"`       // Base price per seat from route
}

type ExitPassCreatedEvent struct {
	ExitPassID       string    `json:"exitPassId"`
	QueueID          string    `json:"queueId"`
	VehicleID        string    `json:"vehicleId"`
	LicensePlate     string    `json:"licensePlate"`
	DestinationID    string    `json:"destinationId"`
	DestinationName  string    `json:"destinationName"`
	PreviousVehicles []string  `json:"previousVehicles"`
	TotalPrice       float64   `json:"totalPrice"`
	CreatedBy        string    `json:"createdBy"`
	CreatedByName    string    `json:"createdByName"`
	CreatedAt        time.Time `json:"createdAt"`
}

type Trip struct {
	ID              string    `json:"id"`
	VehicleID       string    `json:"vehicleId"`
	LicensePlate    string    `json:"licensePlate"`
	DestinationID   string    `json:"destinationId"`
	DestinationName string    `json:"destinationName"`
	QueueID         *string   `json:"queueId"` // Made nullable to handle SET NULL constraint
	SeatsBooked     int       `json:"seatsBooked"`
	StartTime       time.Time `json:"startTime"`
	CreatedAt       time.Time `json:"createdAt"`
	// Vehicle and pricing information
	VehicleCapacity *int     `json:"vehicleCapacity"` // Vehicle capacity (nullable)
	BasePrice       *float64 `json:"basePrice"`       // Base price per seat from route (nullable)
	// True when this trip row is chronologically first for its plate today (exit pass tariff uses first-trip deduction).
	FirstTripOfDay bool `json:"firstTripOfDay,omitempty"`
}

type QueueEntry struct {
	ID                 string     `json:"id"`
	VehicleID          string     `json:"vehicleId"`
	LicensePlate       string     `json:"licensePlate"`
	DestinationID      string     `json:"destinationId"`
	DestinationName    string     `json:"destinationName"`
	SubRoute           *string    `json:"subRoute,omitempty"`
	SubRouteName       *string    `json:"subRouteName,omitempty"`
	QueueType          string     `json:"queueType"`
	QueuePosition      int        `json:"queuePosition"`
	Status             string     `json:"status"`
	EnteredAt          time.Time  `json:"enteredAt"`
	AvailableSeats     int        `json:"availableSeats"`
	TotalSeats         int        `json:"totalSeats"`
	BasePrice          float64    `json:"basePrice"`
	EstimatedDeparture *time.Time `json:"estimatedDeparture,omitempty"`
	ActualDeparture    *time.Time `json:"actualDeparture,omitempty"`
	IsGarageBlocked    bool       `json:"isGarageBlocked,omitempty"`
}

// TodayBookedTicketsByDestination returns the number of tickets booked today,
// grouped by destination.
// - "ghostCountToday" sums ghost bookings (is_ghost_booking=true)
// - "regularCountToday" sums regular bookings (is_ghost_booking=false)
// For regular bookings, "seats_booked" is the number of tickets.
type TodayBookedTicketsByDestination struct {
	DestinationID     string `json:"destinationId"`
	RegularCountToday int    `json:"regularCountToday"`
	GhostCountToday   int    `json:"ghostCountToday"`
	TotalToday        int    `json:"totalToday"`
}
