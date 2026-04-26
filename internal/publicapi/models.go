package publicapi

import "time"

type InitStationInfoRequest struct {
	Name     string `json:"name" binding:"required"`
	Location string `json:"location" binding:"required"`
}

type StationInfoResponse struct {
	StationID         string `json:"station_id"`
	Name              string `json:"name"`
	Location          string `json:"location"`
	PublicIP          string `json:"public_ip"`
	DomainSlug        string `json:"domain_slug,omitempty"`
	DomainFQDN        string `json:"domain_fqdn,omitempty"`
	DomainURL         string `json:"domain_url,omitempty"`
	DomainStatus      string `json:"domain_status,omitempty"`
	DomainError       string `json:"domain_error,omitempty"`
	UptimeSec         int64  `json:"uptime_sec,omitempty"`
	AlreadyConfigured bool   `json:"already_configured,omitempty"`
	Message           string `json:"message,omitempty"`
}

type RouteAvailability struct {
	DestinationID       string `json:"destination_id"`
	DestinationName     string `json:"destination_name"`
	AvailableSeatsTotal int    `json:"available_seats_total"`
	VehiclesCount       int    `json:"vehicles_count"`
}

type RouteVehicle struct {
	QueueID        string `json:"queue_id"`
	VehicleID      string `json:"vehicle_id"`
	AvailableSeats int    `json:"available_seats"`
	Status         string `json:"status"`
}

type RouteDetailsResponse struct {
	DestinationID string         `json:"destination_id"`
	Vehicles      []RouteVehicle `json:"vehicles"`
}

type CreateBookingRequest struct {
	DestinationID  string `json:"destination_id" binding:"required"`
	SeatsBooked    int    `json:"seats_booked" binding:"required,min=1"`
	BookingSource  string `json:"booking_source"`
	BookingType    string `json:"booking_type"`
	UserRef        string `json:"user_ref"`
	IdempotencyKey string `json:"idempotency_key"`
}

type BookingResponse struct {
	BookingID           string     `json:"booking_id"`
	QueueID             string     `json:"queue_id,omitempty"`
	VehicleLicensePlate string     `json:"vehicle_license_plate,omitempty"`
	DestinationID       string     `json:"destination_id,omitempty"`
	SeatsBooked         int        `json:"seats_booked"`
	BookingStatus       string     `json:"booking_status"`
	PaymentStatus       string     `json:"payment_status"`
	PaymentMethod       string     `json:"payment_method,omitempty"`
	PaymentProcessedAt  *time.Time `json:"payment_processed_at,omitempty"`
	ExpiresAt           *time.Time `json:"expires_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
}

type ConfirmBookingRequest struct {
	PaymentStatus      string     `json:"payment_status" binding:"required"`
	PaymentMethod      string     `json:"payment_method" binding:"required"`
	PaymentProcessedAt *time.Time `json:"payment_processed_at"`
}

type ConfirmBookingResponse struct {
	BookingID           string     `json:"booking_id"`
	BookingStatus       string     `json:"booking_status"`
	PaymentStatus       string     `json:"payment_status"`
	PaymentMethod       string     `json:"payment_method,omitempty"`
	PaymentProcessedAt  *time.Time `json:"payment_processed_at,omitempty"`
	VehicleLicensePlate string     `json:"vehicle_license_plate,omitempty"`
}

type CancelBookingRequest struct {
	CancelledBy        string `json:"cancelled_by"`
	CancellationReason string `json:"cancellation_reason"`
}
