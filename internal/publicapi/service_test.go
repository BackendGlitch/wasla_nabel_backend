package publicapi

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDomainManager struct {
	calls atomic.Int32
	last  struct {
		stationID string
		name      string
	}
	err error
}

func (f *fakeDomainManager) EnsureStationDomain(_ context.Context, stationID string, stationName string) (*StationDomainInfo, error) {
	f.calls.Add(1)
	f.last.stationID = stationID
	f.last.name = stationName
	if f.err != nil {
		return nil, f.err
	}
	return &StationDomainInfo{
		Slug:      "sousse-station",
		FQDN:      "sousse-station.backendglitch.com",
		URL:       "https://sousse-station.backendglitch.com",
		TunnelID:  "tun_1",
		UpdatedAt: time.Now(),
	}, nil
}

func TestServiceGetStationInfoAddsRuntimeFields(t *testing.T) {
	repo := &fakeRepo{
		getInfoFn: func(ctx context.Context) (*StationInfoResponse, error) {
			return &StationInfoResponse{
				StationID: "st_1",
				Name:      "Sousse Station",
				Location:  "Sahloul",
			}, nil
		},
	}

	startedAt := time.Now().Add(-125 * time.Second)
	svc := NewService(repo, 10*time.Minute, 50*time.Millisecond, startedAt)

	info, err := svc.GetStationInfo(context.Background(), "10.0.0.15")
	if err != nil {
		t.Fatalf("GetStationInfo returned error: %v", err)
	}

	if info.PublicIP != "10.0.0.15" {
		t.Fatalf("expected PublicIP 10.0.0.15, got %s", info.PublicIP)
	}
	if info.UptimeSec < 124 || info.UptimeSec > 130 {
		t.Fatalf("expected uptime around 125s, got %d", info.UptimeSec)
	}
}

func TestExpiryWorkerRunsPeriodicExpiration(t *testing.T) {
	var calls atomic.Int32
	repo := &fakeRepo{
		expireFn: func(ctx context.Context) (int, error) {
			calls.Add(1)
			return 0, nil
		},
	}

	svc := NewService(repo, 10*time.Minute, 20*time.Millisecond, time.Now())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	svc.StartExpiryWorker(ctx)
	time.Sleep(70 * time.Millisecond)
	cancel()
	time.Sleep(10 * time.Millisecond)

	if calls.Load() < 2 {
		t.Fatalf("expected expiry worker to run at least twice, got %d", calls.Load())
	}
}

func TestEnsureDomainReadyCallsDomainManagerWhenStationExists(t *testing.T) {
	repo := &fakeRepo{
		getInfoFn: func(ctx context.Context) (*StationInfoResponse, error) {
			return &StationInfoResponse{
				StationID: "st_123",
				Name:      "Sousse Station",
				Location:  "Sahloul",
			}, nil
		},
	}
	domain := &fakeDomainManager{}

	svc := NewService(repo, 10*time.Minute, 20*time.Millisecond, time.Now())
	svc.SetDomainManager(domain)
	svc.EnsureDomainReady(context.Background())

	if domain.calls.Load() != 1 {
		t.Fatalf("expected domain manager to be called once, got %d", domain.calls.Load())
	}
	if domain.last.stationID != "st_123" {
		t.Fatalf("expected station id st_123, got %s", domain.last.stationID)
	}
	if domain.last.name != "Sousse Station" {
		t.Fatalf("expected station name Sousse Station, got %s", domain.last.name)
	}
}

func TestEnsureDomainReadySkipsWhenStationNotInitialized(t *testing.T) {
	repo := &fakeRepo{
		getInfoFn: func(ctx context.Context) (*StationInfoResponse, error) {
			return nil, ErrStationNotInitialized
		},
	}
	domain := &fakeDomainManager{}

	svc := NewService(repo, 10*time.Minute, 20*time.Millisecond, time.Now())
	svc.SetDomainManager(domain)
	svc.EnsureDomainReady(context.Background())

	if domain.calls.Load() != 0 {
		t.Fatalf("expected domain manager not to be called, got %d", domain.calls.Load())
	}
}
