package publicapi

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

var publicIPProviders = []string{
	"https://api.ipify.org",
	"https://ifconfig.me/ip",
	"https://ipv4.icanhazip.com",
}

func resolveStationIP(ctx context.Context, fallback string) string {
	return resolveStationIPWithResolvers(ctx, fallback, findTailscaleIPv4, findPublicIPv4)
}

func resolveStationIPWithResolvers(
	ctx context.Context,
	fallback string,
	tailscaleResolver func() string,
	publicResolver func(context.Context) string,
) string {
	if tailscaleResolver != nil {
		if ip := strings.TrimSpace(tailscaleResolver()); ip != "" {
			return ip
		}
	}

	if publicResolver != nil {
		if ip := strings.TrimSpace(publicResolver(ctx)); ip != "" {
			return ip
		}
	}

	return strings.TrimSpace(fallback)
}

func findTailscaleIPv4() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		if !strings.HasPrefix(strings.ToLower(iface.Name), "tailscale") {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		if ip := firstRoutableIPv4(addrs); ip != "" {
			return ip
		}
	}

	return ""
}

func findPublicIPv4(ctx context.Context) string {
	client := &http.Client{Timeout: 2 * time.Second}

	for _, provider := range publicIPProviders {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, provider, nil)
		if err != nil {
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			continue
		}

		body, readErr := io.ReadAll(io.LimitReader(resp.Body, 64))
		_ = resp.Body.Close()
		if readErr != nil {
			continue
		}
		if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
			continue
		}

		ip := strings.TrimSpace(string(body))
		parsed := net.ParseIP(ip)
		if parsed == nil {
			continue
		}
		if ip4 := parsed.To4(); ip4 != nil && ip4.IsGlobalUnicast() {
			return ip4.String()
		}
	}

	return ""
}

func firstRoutableIPv4(addrs []net.Addr) string {
	for _, addr := range addrs {
		var ip net.IP
		switch v := addr.(type) {
		case *net.IPNet:
			ip = v.IP
		case *net.IPAddr:
			ip = v.IP
		default:
			parsed := net.ParseIP(strings.TrimSpace(addr.String()))
			if parsed != nil {
				ip = parsed
			}
		}

		if ip == nil || ip.IsLoopback() {
			continue
		}

		ip4 := ip.To4()
		if ip4 == nil {
			continue
		}
		if !ip4.IsGlobalUnicast() {
			continue
		}

		return ip4.String()
	}

	return ""
}
