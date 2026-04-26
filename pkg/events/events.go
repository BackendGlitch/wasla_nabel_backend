package events

// Central registry of WebSocket event types.
//
// These are sent through websocket-hub (cmd/websocket-hub) and consumed by
// Electron clients (wasla/ and wasla_management/).
//
// Keep this list stable and versioned; clients may subscribe by name.

const (
	// Queue events
	QueueUpdated      = "queue_updated"
	QueueEntryAdded   = "queue_entry_added"
	QueueEntryRemoved = "queue_entry_removed"
	QueueEntryUpdated = "queue_entry_updated"
	QueueReordered    = "queue_reordered"

	// Pass events
	DayPassCreated  = "day_pass_created"
	ExitPassCreated = "exit_pass_created"

	// Booking events
	GhostBookingCreated = "ghost_booking_created"

	// Printer events (new)
	PrintJobUpdated = "print_job_updated"

	// Statistics events
	StatisticsUpdate = "statistics_update"
)

