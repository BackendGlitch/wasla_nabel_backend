package printer

import (
	"time"
)

// PrinterConfig represents the configuration for a thermal printer
type PrinterConfig struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IP        string `json:"ip"`
	Port      int    `json:"port"`
	Width     int    `json:"width"`
	Timeout   int    `json:"timeout"`
	Model     string `json:"model"`
	Enabled   bool   `json:"enabled"`
	IsDefault bool   `json:"isDefault"`
}

// PrintJob represents a print job to be sent to the printer
type PrintJob struct {
	Content        string `json:"content"`
	Align          string `json:"align,omitempty"` // "left", "center", "right"
	Bold           bool   `json:"bold,omitempty"`
	Underline      bool   `json:"underline,omitempty"`
	Size           string `json:"size,omitempty"` // "normal", "double_height", "double_width", "quad"
	Cut            bool   `json:"cut,omitempty"`
	OpenCashDrawer bool   `json:"openCashDrawer,omitempty"`
}

// PrinterStatus represents the current status of a printer
type PrinterStatus struct {
	Connected bool   `json:"connected"`
	Error     string `json:"error,omitempty"`
}

// PrintJobType represents the type of print job
type PrintJobType string

const (
	PrintJobTypeBookingTicket  PrintJobType = "booking_ticket"
	PrintJobTypeEntryTicket    PrintJobType = "entry_ticket"
	PrintJobTypeExitTicket     PrintJobType = "exit_ticket"
	PrintJobTypeDayPassTicket  PrintJobType = "day_pass_ticket"
	PrintJobTypeExitPassTicket PrintJobType = "exit_pass_ticket"
	PrintJobTypeTalon          PrintJobType = "talon"
	PrintJobTypeStandardTicket PrintJobType = "standard_ticket"
	PrintJobTypeReceipt        PrintJobType = "receipt"
	PrintJobTypeQRCode         PrintJobType = "qr_code"
)

// QueuedPrintJob represents a print job in the queue
type QueuedPrintJob struct {
	ID         string       `json:"id"`
	JobType    PrintJobType `json:"jobType"`
	Content    string       `json:"content"`
	StaffName  string       `json:"staffName,omitempty"`
	Priority   int          `json:"priority"` // 0 = highest priority, 255 = lowest
	CreatedAt  time.Time    `json:"createdAt"`
	RetryCount int          `json:"retryCount"`
}

// PrintQueueStatus represents the status of the print queue
type PrintQueueStatus struct {
	QueueLength   int        `json:"queueLength"`
	IsProcessing  bool       `json:"isProcessing"`
	LastPrintedAt *time.Time `json:"lastPrintedAt,omitempty"`
	FailedJobs    int        `json:"failedJobs"`
}

// FrontendPrinterConfig represents printer configuration sent from frontend
type FrontendPrinterConfig struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// TicketData represents the data for printing tickets
type TicketData struct {
	// Optional: for audit logging (booking tickets)
	BookingID string `json:"bookingId,omitempty"`
	// Optional: logical printer identifier (falls back to ip:port or "default")
	PrinterID string `json:"printerId,omitempty"`
	// Optional: idempotency key to dedupe retries (per user action)
	IdempotencyKey  string  `json:"idempotencyKey,omitempty"`
	LicensePlate    string  `json:"licensePlate"`
	DestinationName string  `json:"destinationName"`
	SeatNumber      int     `json:"seatNumber"`
	TotalAmount     float64 `json:"totalAmount"`
	CreatedBy       string  `json:"createdBy"`
	// Human-readable name when createdBy is a staff id (e.g. from booking records).
	CreatedByName string    `json:"createdByName,omitempty"`
	CreatedAt     time.Time `json:"createdAt"`
	StationName   string    `json:"stationName"`
	RouteName     string    `json:"routeName"`
	// Vehicle and pricing information
	VehicleCapacity int     `json:"vehicleCapacity,omitempty"` // Vehicle capacity for total amount calculation
	BasePrice       float64 `json:"basePrice,omitempty"`       // Base price per seat from route
	ServiceFee      float64 `json:"serviceFee,omitempty"`      // Service fee per seat from route
	// Exit pass count for today
	ExitPassCount int `json:"exitPassCount,omitempty"` // Current count of exit passes for today
	// Company branding
	CompanyName string `json:"companyName,omitempty"`
	CompanyLogo string `json:"companyLogo,omitempty"`
	// Booking slip Arabic header overrides (optional).
	CompanyArabic       string   `json:"companyArabic,omitempty"`
	StationArabic       string   `json:"stationArabic,omitempty"`
	StationRoutesArabic []string `json:"stationRoutesArabic,omitempty"`
	// Staff information
	StaffFirstName string `json:"staffFirstName,omitempty"`
	StaffLastName  string `json:"staffLastName,omitempty"`
	// When true, talon shows a small "*" top-right (vehicle has not yet completed a trip today — any destination).
	FirstTripOfDay bool `json:"firstTripOfDay,omitempty"`
	// Printer configuration from frontend
	PrinterConfig *FrontendPrinterConfig `json:"printerConfig,omitempty"`
	// Day pass ticket: mirrors queue.DayPassCreatedEvent (omit when absent).
	PurchaseDate time.Time `json:"purchaseDate,omitempty"`
	ValidFrom    time.Time `json:"validFrom,omitempty"`
	ValidUntil   time.Time `json:"validUntil,omitempty"`
}

// ExitPassAndRemoveRequest represents the request for printing exit pass and removing from queue
type ExitPassAndRemoveRequest struct {
	QueueEntryID    string                 `json:"queueEntryId" binding:"required"`
	LicensePlate    string                 `json:"licensePlate" binding:"required"`
	DestinationName string                 `json:"destinationName" binding:"required"`
	BookedSeats     int                    `json:"bookedSeats" binding:"required"`
	TotalSeats      int                    `json:"totalSeats" binding:"required"`
	BasePrice       float64                `json:"basePrice" binding:"required"`
	ServiceFee      float64                `json:"serviceFee,omitempty"` // Service fee per seat from route
	CreatedBy       string                 `json:"createdBy" binding:"required"`
	StationName     string                 `json:"stationName"`
	RouteName       string                 `json:"routeName"`
	ExitPassCount   int                    `json:"exitPassCount"`
	CompanyName     string                 `json:"companyName"`
	CompanyLogo     string                 `json:"companyLogo"`
	StaffFirstName  string                 `json:"staffFirstName"`
	StaffLastName   string                 `json:"staffLastName"`
	PrinterConfig   *FrontendPrinterConfig `json:"printerConfig,omitempty"`
}
