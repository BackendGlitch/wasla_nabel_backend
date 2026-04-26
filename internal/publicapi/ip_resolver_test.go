package publicapi

import (
	"context"
	"net"
	"testing"
)

func TestResolveStationIPPrefersTailscale(t *testing.T) {
	resolved := resolveStationIPWithResolvers(
		context.Background(),
		"203.0.113.10",
		func() string { return "100.80.20.5" },
		func(context.Context) string { return "8.8.8.8" },
	)

	if resolved != "100.80.20.5" {
		t.Fatalf("expected tailscale IP, got %s", resolved)
	}
}

func TestResolveStationIPFallsBackToPublic(t *testing.T) {
	resolved := resolveStationIPWithResolvers(
		context.Background(),
		"203.0.113.10",
		func() string { return "" },
		func(context.Context) string { return "8.8.8.8" },
	)

	if resolved != "8.8.8.8" {
		t.Fatalf("expected public IP fallback, got %s", resolved)
	}
}

func TestResolveStationIPFallsBackToClientIP(t *testing.T) {
	resolved := resolveStationIPWithResolvers(
		context.Background(),
		"127.0.0.1",
		func() string { return "" },
		func(context.Context) string { return "" },
	)

	if resolved != "127.0.0.1" {
		t.Fatalf("expected client fallback IP, got %s", resolved)
	}
}

func TestFirstRoutableIPv4SkipsLoopback(t *testing.T) {
	_, loopbackCIDR, _ := net.ParseCIDR("127.0.0.1/8")
	_, tailscaleCIDR, _ := net.ParseCIDR("100.90.10.2/32")

	ip := firstRoutableIPv4([]net.Addr{loopbackCIDR, tailscaleCIDR})
	if ip != "100.90.10.2" {
		t.Fatalf("expected 100.90.10.2, got %s", ip)
	}
}
