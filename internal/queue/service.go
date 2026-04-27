package queue

import (
	"context"
	"log"
	"regexp"
	"time"

	"station-backend/pkg/events"
	"station-backend/internal/statistics"
	"station-backend/internal/websocket"
)

type Service struct {
	repo        Repository
	ws          websocket.Broadcaster
	statsLogger *statistics.StatisticsLogger
}

func NewService(repo Repository, ws websocket.Broadcaster, statsLogger *statistics.StatisticsLogger) *Service {
	return &Service{repo: repo, ws: ws, statsLogger: statsLogger}
}

// Tunisian license plate pattern: 2 or 3 digits, then space? then TUN, then space? then 1..9999
var tunisLP = regexp.MustCompile(`^(\d{2,3})\s*TUN\s*(\d{1,4})$`)

func (s *Service) ValidateLicensePlate(lp string) bool {
	return tunisLP.MatchString(lp)
}

// Routes
func (s *Service) ListRoutes(ctx context.Context, includeInactive bool) ([]Route, error) {
	return s.repo.ListRoutes(ctx, includeInactive)
}

func (s *Service) CreateRoute(ctx context.Context, req CreateRouteRequest) (*Route, error) {
	return s.repo.CreateRoute(ctx, req)
}

func (s *Service) UpdateRoute(ctx context.Context, id string, req UpdateRouteRequest) (*Route, error) {
	return s.repo.UpdateRoute(ctx, id, req)
}

func (s *Service) DeleteRoute(ctx context.Context, id string) error {
	return s.repo.DeleteRoute(ctx, id)
}

// Trips
func (s *Service) CreateTripFromExit(ctx context.Context, queueEntryID string, licensePlate string, destinationName string, seatsBooked int, totalSeats int, basePrice float64) (string, error) {
	return s.repo.CreateTripFromExit(ctx, queueEntryID, licensePlate, destinationName, seatsBooked, totalSeats, basePrice)
}

// Vehicles
func (s *Service) ListVehicles(ctx context.Context, searchQuery string) ([]Vehicle, error) {
	return s.repo.ListVehicles(ctx, searchQuery)
}

func (s *Service) CreateVehicle(ctx context.Context, req CreateVehicleRequest) (*Vehicle, error) {
	if !s.ValidateLicensePlate(req.LicensePlate) {
		return nil, ErrInvalidLicensePlate
	}
	return s.repo.CreateVehicle(ctx, req)
}

func (s *Service) UpdateVehicle(ctx context.Context, id string, req UpdateVehicleRequest) (*Vehicle, error) {
	if req.LicensePlate != nil && !s.ValidateLicensePlate(*req.LicensePlate) {
		return nil, ErrInvalidLicensePlate
	}
	return s.repo.UpdateVehicle(ctx, id, req)
}

func (s *Service) DeleteVehicle(ctx context.Context, id string) error {
	return s.repo.DeleteVehicle(ctx, id)
}

// Authorized routes
func (s *Service) ListAuthorizedRoutes(ctx context.Context, vehicleID string) ([]VehicleAuthorizedStation, error) {
	return s.repo.ListAuthorizedRoutes(ctx, vehicleID)
}

func (s *Service) AddAuthorizedRoute(ctx context.Context, vehicleID string, req AddAuthorizedRouteRequest) (*VehicleAuthorizedStation, error) {
	// If setting default, we could unset others here in future; for now trust DB uniqueness policy if any
	return s.repo.AddAuthorizedRoute(ctx, vehicleID, req)
}

func (s *Service) UpdateAuthorizedRoute(ctx context.Context, authID string, req UpdateAuthorizedRouteRequest) (*VehicleAuthorizedStation, error) {
	return s.repo.UpdateAuthorizedRoute(ctx, authID, req)
}

func (s *Service) DeleteAuthorizedRoute(ctx context.Context, authID string) error {
	return s.repo.DeleteAuthorizedRoute(ctx, authID)
}

// Queue entries
func (s *Service) ListQueue(ctx context.Context, destinationID string, subRoute *string) ([]QueueEntry, error) {
	return s.repo.ListQueue(ctx, destinationID, subRoute)
}

func (s *Service) AddQueueEntry(ctx context.Context, req AddQueueEntryRequest) (*AddQueueEntryResponse, error) {
	e, dayPassEvent, existingDayPass, dayPassStatus, err := s.repo.AddQueueEntry(ctx, req)
	if err != nil {
		return nil, err
	}

	response := &AddQueueEntryResponse{
		QueueEntry:    e,
		DayPass:       dayPassEvent,
		DayPassValid:  existingDayPass,
		DayPassStatus: dayPassStatus,
	}

	// Log statistics for day pass creation
	if s.statsLogger != nil && req.CreatedBy != "" && dayPassEvent != nil {
		// Use destination ID as station ID for now
		stationID := req.DestinationID
		s.statsLogger.LogDayPassTransactionAsync(req.CreatedBy, dayPassEvent.DayPassID, stationID)
	}

	if s.ws != nil {
		// Broadcast day pass creation event if a day pass was created
		if dayPassEvent != nil {
			s.ws.BroadcastToStation(req.DestinationID, events.DayPassCreated, dayPassEvent)
		}

		// include refreshed queue snapshot
		if list, lerr := s.repo.ListQueue(ctx, req.DestinationID, nil); lerr == nil {
			s.ws.BroadcastToStation(req.DestinationID, events.QueueEntryAdded, map[string]interface{}{
				"entry": e,
				"queue": list,
			})
		} else {
			s.ws.BroadcastToStation(req.DestinationID, events.QueueEntryAdded, e)
		}
	}

	return response, nil
}

func (s *Service) GetVehicleDayPass(ctx context.Context, vehicleID string) (*DayPassCreatedEvent, error) {
	return s.repo.GetVehicleDayPass(ctx, vehicleID)
}

func (s *Service) UpdateQueueEntry(ctx context.Context, id string, req UpdateQueueEntryRequest) (*QueueEntry, error) {
	e, err := s.repo.UpdateQueueEntry(ctx, id, req)
	if err == nil && s.ws != nil {
		if list, lerr := s.repo.ListQueue(ctx, e.DestinationID, nil); lerr == nil {
			s.ws.BroadcastToStation(e.DestinationID, events.QueueEntryUpdated, map[string]interface{}{
				"entry": e,
				"queue": list,
			})
		} else {
			s.ws.BroadcastToStation(e.DestinationID, events.QueueEntryUpdated, e)
		}
	}
	return e, err
}

func (s *Service) DeleteQueueEntry(ctx context.Context, id string) error {
	// Get the destination ID before deleting to check if queue becomes empty
	var destinationID string
	row := s.repo.(*RepositoryImpl).db.QueryRow(ctx, `SELECT destination_id FROM vehicle_queue WHERE id=$1`, id)
	if err := row.Scan(&destinationID); err != nil {
		return err
	}

	if err := s.repo.DeleteQueueEntry(ctx, id); err != nil {
		return err
	}

	// Check if the destination queue is now empty
	var remainingCount int
	err := s.repo.(*RepositoryImpl).db.QueryRow(ctx,
		`SELECT COUNT(*) FROM vehicle_queue WHERE destination_id=$1`, destinationID).Scan(&remainingCount)
	if err != nil {
		remainingCount = 0
	}

	// Broadcast removal event with additional context
	if s.ws != nil {
		eventData := map[string]interface{}{
			"id":            id,
			"destinationId": destinationID,
			"queueEmpty":    remainingCount == 0,
		}
		s.ws.BroadcastToStation("*", events.QueueEntryRemoved, eventData)
	}
	return nil
}

func (s *Service) ReorderQueue(ctx context.Context, destinationID string, entryIDs []string) error {
	if err := s.repo.ReorderQueue(ctx, destinationID, entryIDs); err != nil {
		return err
	}
	// Broadcast minimal info to listeners (clients can refetch queue)
	if s.ws != nil {
		if list, lerr := s.repo.ListQueue(ctx, destinationID, nil); lerr == nil {
			s.ws.BroadcastToStation(destinationID, events.QueueReordered, map[string]interface{}{
				"entryIds": entryIDs,
				"queue":    list,
			})
		} else {
			s.ws.BroadcastToStation(destinationID, events.QueueReordered, map[string]interface{}{"entryIds": entryIDs})
		}
	}
	return nil
}

func (s *Service) MoveEntry(ctx context.Context, destinationID string, entryID string, newPos int) error {
	if err := s.repo.MoveEntry(ctx, entryID, destinationID, newPos); err != nil {
		return err
	}
	if s.ws != nil {
		s.ws.BroadcastToStation(destinationID, events.QueueReordered, map[string]interface{}{"movedId": entryID, "newPosition": newPos})
	}
	return nil
}

func (s *Service) TransferSeats(ctx context.Context, destinationID string, fromEntryID, toEntryID string, seats int) error {
	if err := s.repo.TransferSeats(ctx, fromEntryID, toEntryID, seats); err != nil {
		return err
	}
	if s.ws != nil {
		s.ws.BroadcastToStation(destinationID, events.QueueEntryUpdated, map[string]interface{}{"fromId": fromEntryID, "toId": toEntryID, "seats": seats})
	}
	return nil
}

func (s *Service) ChangeDestination(ctx context.Context, entryID, oldDestinationID, newDestinationID, newDestinationName string) error {
	if err := s.repo.ChangeDestination(ctx, entryID, newDestinationID, newDestinationName); err != nil {
		return err
	}
	if s.ws != nil {
		if listOld, lerr := s.repo.ListQueue(ctx, oldDestinationID, nil); lerr == nil {
			s.ws.BroadcastToStation(oldDestinationID, events.QueueReordered, map[string]interface{}{"queue": listOld})
		}
		if listNew, lerr := s.repo.ListQueue(ctx, newDestinationID, nil); lerr == nil {
			s.ws.BroadcastToStation(newDestinationID, events.QueueEntryAdded, map[string]interface{}{"queue": listNew})
		}
	}
	return nil
}

func (s *Service) ListDayPasses(ctx context.Context, limit int) ([]DayPass, error) {
	return s.repo.ListDayPasses(ctx, limit)
}

// Aggregates
func (s *Service) ListQueueSummaries(ctx context.Context, station string) ([]QueueSummary, error) {
	return s.repo.ListQueueSummaries(ctx, station)
}

func (s *Service) ListAllDestinations(ctx context.Context) ([]Destination, error) {
	return s.repo.ListAllDestinations(ctx)
}

func (s *Service) ListRouteSummaries(ctx context.Context) ([]RouteSummary, error) {
	return s.repo.ListRouteSummaries(ctx)
}

func (s *Service) ClearQueue(ctx context.Context, destinationID string) error {
	if err := s.repo.ClearQueue(ctx, destinationID); err != nil {
		return err
	}
	// Broadcast queue cleared event
	if s.ws != nil {
		s.ws.BroadcastToStation(destinationID, events.QueueUpdated, map[string]interface{}{
			"destinationId": destinationID,
		})
		// Also send empty queue
		s.ws.BroadcastToStation(destinationID, events.QueueEntryRemoved, map[string]interface{}{
			"destinationId": destinationID,
			"queueEmpty":    true,
		})
	}
	return nil
}

func (s *Service) ClearAllQueues(ctx context.Context) error {
	if err := s.repo.ClearAllQueues(ctx); err != nil {
		return err
	}
	// Broadcast to all stations that queues are cleared
	if s.ws != nil {
		s.ws.BroadcastToStation("*", events.QueueUpdated, map[string]interface{}{
			"message": "All queues have been cleared",
		})
	}
	return nil
}

func (s *Service) TriggerMidnightMaintenance(ctx context.Context) (expiredPasses int64, clearedQueueEntries int64, err error) {
	expiredPasses, clearedQueueEntries, err = s.repo.ExpireDayPassesAndClearRegularQueues(ctx)
	if err != nil {
		return 0, 0, err
	}
	if s.ws != nil {
		s.ws.BroadcastToStation("*", events.QueueUpdated, map[string]interface{}{
			"message":               "Manual midnight maintenance completed",
			"expiredDayPasses":      expiredPasses,
			"clearedRegularQueues":  clearedQueueEntries,
			"triggeredManually":     true,
		})
	}
	return expiredPasses, clearedQueueEntries, nil
}

func (s *Service) RunMidnightMaintenance(ctx context.Context) {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	lastRunDay := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			loc := tunisLocation()
			now := time.Now().In(loc)
			if now.Hour() != 0 {
				continue
			}
			dayKey := now.Format("2006-01-02")
			if dayKey == lastRunDay {
				continue
			}

			expired, cleared, err := s.repo.ExpireDayPassesAndClearRegularQueues(ctx)
			if err != nil {
				log.Printf("midnight maintenance failed: %v", err)
				continue
			}
			lastRunDay = dayKey
			log.Printf("midnight maintenance done: expired_day_passes=%d cleared_regular_queue_entries=%d", expired, cleared)

			if s.ws != nil {
				s.ws.BroadcastToStation("*", events.QueueUpdated, map[string]interface{}{
					"message": "Midnight maintenance completed",
					"cleared": true,
				})
			}
		}
	}
}
