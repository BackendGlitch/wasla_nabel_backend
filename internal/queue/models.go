package queue

import "time"

type Route struct {
	ID            string    `json:"id"`
	StationID     string    `json:"stationId"`
	StationName   string    `json:"stationName"`
	BasePrice     float64   `json:"basePrice"`
	ServiceFee    float64   `json:"serviceFee"`
	Governorate   *string   `json:"governorate,omitempty"`
	GovernorateAr *string   `json:"governorateAr,omitempty"`
	Delegation    *string   `json:"delegation,omitempty"`
	DelegationAr  *string   `json:"delegationAr,omitempty"`
	IsActive      bool      `json:"isActive"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

type Destination struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	BasePrice float64 `json:"basePrice"`
	ServiceFee float64 `json:"serviceFee"`
	IsActive  bool    `json:"isActive"`
}

type Vehicle struct {
	ID                 string                     `json:"id"`
	LicensePlate       string                     `json:"licensePlate"`
	Capacity           int                        `json:"capacity"`
	PhoneNumber        *string                    `json:"phoneNumber,omitempty"`
	IsActive           bool                       `json:"isActive"`
	IsAvailable        bool                       `json:"isAvailable"`
	IsBanned           bool                       `json:"isBanned"`
	DefaultDestID      *string                    `json:"defaultDestinationId,omitempty"`
	DefaultDestName    *string                    `json:"defaultDestinationName,omitempty"`
	AvailableSeats     int                        `json:"availableSeats"`
	TotalSeats         int                        `json:"totalSeats"`
	BasePrice          float64                    `json:"basePrice"`
	DestinationID      *string                    `json:"destinationId,omitempty"`
	DestinationName    *string                    `json:"destinationName,omitempty"`
	CreatedAt          time.Time                  `json:"createdAt"`
	UpdatedAt          time.Time                  `json:"updatedAt"`
	AuthorizedStations []VehicleAuthorizedStation `json:"authorizedStations,omitempty"`
}

type VehicleAuthorizedStation struct {
	ID          string    `json:"id"`
	VehicleID   string    `json:"vehicleId"`
	StationID   string    `json:"stationId"`
	StationName *string   `json:"stationName,omitempty"`
	Priority    int       `json:"priority"`
	IsDefault   bool      `json:"isDefault"`
	CreatedAt   time.Time `json:"createdAt"`
}

type DayPass struct {
	ID           string    `json:"id"`
	VehicleID    string    `json:"vehicleId"`
	LicensePlate string    `json:"licensePlate"`
	Price        float64   `json:"price"`
	PurchaseDate time.Time `json:"purchaseDate"`
	ValidFrom    time.Time `json:"validFrom"`
	ValidUntil   time.Time `json:"validUntil"`
	IsActive     bool      `json:"isActive"`
	IsExpired    bool      `json:"isExpired"`
	CreatedBy    string    `json:"createdBy"`
}

// DayPassCreatedEvent represents a day pass creation event for WebSocket broadcasting
type DayPassCreatedEvent struct {
	DayPassID       string    `json:"dayPassId"`
	VehicleID       string    `json:"vehicleId"`
	LicensePlate    string    `json:"licensePlate"`
	DestinationID   string    `json:"destinationId"`
	DestinationName string    `json:"destinationName"`
	Price           float64   `json:"price"`
	PurchaseDate    time.Time `json:"purchaseDate"`
	ValidFrom       time.Time `json:"validFrom"`
	ValidUntil      time.Time `json:"validUntil"`
	CreatedBy       string    `json:"createdBy"`
}

// Aggregated view of a destination queue
type QueueSummary struct {
	DestinationID   string  `json:"destinationId"`
	DestinationName string  `json:"destinationName"`
	TotalVehicles   int     `json:"totalVehicles"`
	TotalSeats      int     `json:"totalSeats"`
	AvailableSeats  int     `json:"availableSeats"`
	BasePrice       float64 `json:"basePrice"`
}

// RouteSummary aggregates seats per route (route == destination)
type RouteSummary struct {
	RouteID        string `json:"routeId"`
	RouteName      string `json:"routeName"`
	TotalVehicles  int    `json:"totalVehicles"`
	TotalSeats     int    `json:"totalSeats"`
	AvailableSeats int    `json:"availableSeats"`
}

type CreateRouteRequest struct {
	StationID     string  `json:"stationId" binding:"required"`
	StationName   string  `json:"stationName" binding:"required"`
	BasePrice     float64 `json:"basePrice" binding:"required"`
	ServiceFee    *float64 `json:"serviceFee,omitempty"`
	Governorate   *string `json:"governorate"`
	GovernorateAr *string `json:"governorateAr"`
	Delegation    *string `json:"delegation"`
	DelegationAr  *string `json:"delegationAr"`
}

type UpdateRouteRequest struct {
	StationName   *string  `json:"stationName"`
	BasePrice     *float64 `json:"basePrice"`
	ServiceFee    *float64 `json:"serviceFee"`
	Governorate   *string  `json:"governorate"`
	GovernorateAr *string  `json:"governorateAr"`
	Delegation    *string  `json:"delegation"`
	DelegationAr  *string  `json:"delegationAr"`
	IsActive      *bool    `json:"isActive"`
}

type CreateVehicleRequest struct {
	LicensePlate string  `json:"licensePlate" binding:"required"`
	Capacity     int     `json:"capacity" binding:"required"`
	PhoneNumber  *string `json:"phoneNumber"`
}

type UpdateVehicleRequest struct {
	LicensePlate    *string `json:"licensePlate"`
	Capacity        *int    `json:"capacity"`
	PhoneNumber     *string `json:"phoneNumber"`
	IsActive        *bool   `json:"isActive"`
	IsAvailable     *bool   `json:"isAvailable"`
	IsBanned        *bool   `json:"isBanned"`
	DefaultDestID   *string `json:"defaultDestinationId"`
	DefaultDestName *string `json:"defaultDestinationName"`
}

// Authorized routes (vehicle <-> station) requests
type AddAuthorizedRouteRequest struct {
	StationID   string  `json:"stationId" binding:"required"`
	StationName *string `json:"stationName"`
	Priority    int     `json:"priority" binding:"required"`
	IsDefault   bool    `json:"isDefault"`
}

type UpdateAuthorizedRouteRequest struct {
	Priority  *int  `json:"priority"`
	IsDefault *bool `json:"isDefault"`
}

// ===== Queue Entries =====

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
	BookedSeats        int        `json:"bookedSeats"`
	BasePrice          float64    `json:"basePrice"`
	EstimatedDeparture *time.Time `json:"estimatedDeparture,omitempty"`
	ActualDeparture    *time.Time `json:"actualDeparture,omitempty"`
	// Day pass status fields
	HasDayPass         bool       `json:"hasDayPass"`
	DayPassStatus      string     `json:"dayPassStatus"` // "no_pass", "has_pass", "recent_pass"
	DayPassPurchasedAt *time.Time `json:"dayPassPurchasedAt,omitempty"`
	HasTripsToday      bool       `json:"hasTripsToday"`
	// IsGarageBlocked: vehicle parked in garage; keeps queue_position but is skipped for next-car booking and POS list when filtered.
	IsGarageBlocked bool `json:"isGarageBlocked"`
}

type AddQueueEntryRequest struct {
	VehicleID       string  `json:"vehicleId" binding:"required"`
	DestinationID   string  `json:"destinationId" binding:"required"`
	DestinationName string  `json:"destinationName" binding:"required"`
	SubRoute        *string `json:"subRoute"`
	SubRouteName    *string `json:"subRouteName"`
	QueueType       *string `json:"queueType"`
	CreatedBy       string  `json:"createdBy"`
}

type AddQueueEntryResponse struct {
	QueueEntry    *QueueEntry          `json:"queueEntry"`
	DayPass       *DayPassCreatedEvent `json:"dayPass,omitempty"`
	DayPassStatus string               `json:"dayPassStatus"`          // "valid", "created", "none"
	DayPassValid  *DayPassCreatedEvent `json:"dayPassValid,omitempty"` // Existing valid day pass
}

type SetGarageBlockedRequest struct {
	Blocked bool `json:"blocked"`
}

type UpdateQueueEntryRequest struct {
	Status             *string    `json:"status"`
	AvailableSeats     *int       `json:"availableSeats"`
	EstimatedDeparture *time.Time `json:"estimatedDeparture"`
	SubRoute           *string    `json:"subRoute"`
	SubRouteName       *string    `json:"subRouteName"`
}

type ReorderQueueRequest struct {
	EntryIDs []string `json:"entryIds" binding:"required"`
}

type MoveQueueEntryRequest struct {
	NewPosition int `json:"newPosition" binding:"required"`
}

type TransferSeatsRequest struct {
	FromEntryID string `json:"fromEntryId" binding:"required"`
	ToEntryID   string `json:"toEntryId" binding:"required"`
	Seats       int    `json:"seats" binding:"required"`
}

type ChangeDestinationRequest struct {
	NewDestinationID   string `json:"newDestinationId" binding:"required"`
	NewDestinationName string `json:"newDestinationName" binding:"required"`
}
