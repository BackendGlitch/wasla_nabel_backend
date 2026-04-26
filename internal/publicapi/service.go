package publicapi

import (
	"context"
	"errors"
	"log"
	"time"
)

type Service struct {
	repo           Repository
	domainManager  DomainManager
	holdTTL        time.Duration
	expireInterval time.Duration
	startedAt      time.Time
}

func NewService(repo Repository, holdTTL time.Duration, expireInterval time.Duration, startedAt time.Time) *Service {
	return &Service{
		repo:           repo,
		holdTTL:        holdTTL,
		expireInterval: expireInterval,
		startedAt:      startedAt,
	}
}

func (s *Service) SetDomainManager(manager DomainManager) {
	s.domainManager = manager
}

func (s *Service) EnsureDomainReady(ctx context.Context) {
	if s.domainManager == nil {
		return
	}

	info, err := s.repo.GetStationInfo(ctx)
	if err != nil {
		if errors.Is(err, ErrStationNotInitialized) {
			return
		}
		log.Printf("public-service: failed loading station info for domain readiness: %v", err)
		return
	}
	if info == nil {
		return
	}

	domainInfo, err := s.domainManager.EnsureStationDomain(ctx, info.StationID, info.Name)
	if err != nil {
		log.Printf("public-service: failed ensuring domain readiness: %v", err)
		return
	}
	if domainInfo != nil {
		log.Printf("public-service: domain ready at %s", domainInfo.URL)
	}
}

func (s *Service) InitializeStationInfo(ctx context.Context, name string, location string, publicIP string) (*StationInfoResponse, error) {
	info, err := s.repo.InitializeStationInfo(ctx, name, location)
	if err != nil {
		return nil, err
	}
	info.PublicIP = publicIP
	s.enrichDomainInfo(ctx, info)
	return info, nil
}

func (s *Service) GetStationInfo(ctx context.Context, publicIP string) (*StationInfoResponse, error) {
	info, err := s.repo.GetStationInfo(ctx)
	if err != nil {
		return nil, err
	}
	info.PublicIP = publicIP
	info.UptimeSec = int64(time.Since(s.startedAt).Seconds())
	s.enrichDomainInfo(ctx, info)
	return info, nil
}

func (s *Service) enrichDomainInfo(ctx context.Context, info *StationInfoResponse) {
	if info == nil || s.domainManager == nil {
		return
	}

	domainInfo, err := s.domainManager.EnsureStationDomain(ctx, info.StationID, info.Name)
	if err != nil {
		info.DomainStatus = "error"
		info.DomainError = err.Error()
		return
	}
	if domainInfo == nil {
		info.DomainStatus = "disabled"
		return
	}

	info.DomainSlug = domainInfo.Slug
	info.DomainFQDN = domainInfo.FQDN
	info.DomainURL = domainInfo.URL
	info.DomainStatus = "ready"
}

func (s *Service) ListRouteAvailability(ctx context.Context) ([]RouteAvailability, error) {
	return s.repo.ListRouteAvailability(ctx)
}

func (s *Service) GetRouteDetails(ctx context.Context, destinationID string) (*RouteDetailsResponse, error) {
	return s.repo.GetRouteDetails(ctx, destinationID)
}

func (s *Service) CreateBookingHold(ctx context.Context, req CreateBookingRequest) (*BookingResponse, bool, error) {
	return s.repo.CreateBookingHold(ctx, req, s.holdTTL)
}

func (s *Service) GetBooking(ctx context.Context, bookingID string) (*BookingResponse, error) {
	return s.repo.GetBooking(ctx, bookingID)
}

func (s *Service) ConfirmBooking(ctx context.Context, bookingID string, req ConfirmBookingRequest) (*BookingResponse, error) {
	return s.repo.ConfirmBooking(ctx, bookingID, req)
}

func (s *Service) CancelBooking(ctx context.Context, bookingID string, req CancelBookingRequest) (*BookingResponse, error) {
	return s.repo.CancelBooking(ctx, bookingID, req)
}

func (s *Service) StartExpiryWorker(ctx context.Context) {
	if s.expireInterval <= 0 {
		return
	}

	// Run once on startup before entering the periodic loop.
	if n, err := s.repo.ExpireHeldBookings(ctx); err != nil {
		log.Printf("public-service: hold expiry startup run failed: %v", err)
	} else if n > 0 {
		log.Printf("public-service: expired %d held bookings on startup", n)
	}

	ticker := time.NewTicker(s.expireInterval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				count, err := s.repo.ExpireHeldBookings(ctx)
				if err != nil {
					log.Printf("public-service: hold expiry run failed: %v", err)
					continue
				}
				if count > 0 {
					log.Printf("public-service: expired %d held bookings", count)
				}
			}
		}
	}()
}
