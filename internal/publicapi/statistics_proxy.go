package publicapi

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"
	"time"
)

type upstreamError struct {
	Error string `json:"error"`
}

func NewStatisticsProxyFromEnv() (http.Handler, error) {
	upstreamURL := strings.TrimSpace(os.Getenv("PUBLIC_STATS_PROXY_URL"))
	if upstreamURL == "" {
		port := strings.TrimSpace(os.Getenv("STATISTICS_SERVICE_PORT"))
		if port == "" {
			port = "8006"
		}
		upstreamURL = "http://localhost:" + port
	}

	target, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PUBLIC_STATS_PROXY_URL: %w", err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("invalid PUBLIC_STATS_PROXY_URL: %q", upstreamURL)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = newProxyTransport(parseDurationSecondsDefault(os.Getenv("PUBLIC_STATS_PROXY_TIMEOUT_SECONDS"), 15*time.Second))
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(upstreamError{Error: "STATISTICS_UPSTREAM_UNAVAILABLE"})
	}

	return proxy, nil
}

func newProxyTransport(timeout time.Duration) http.RoundTripper {
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return &http.Transport{
			ResponseHeaderTimeout: timeout,
			MaxIdleConns:          20,
			MaxIdleConnsPerHost:   10,
			IdleConnTimeout:       90 * time.Second,
		}
	}

	clone := base.Clone()
	clone.ResponseHeaderTimeout = timeout
	// Keep connections alive to upstream services so the reverse proxy
	// doesn't pay TCP + HTTP handshake on every proxied request.
	clone.MaxIdleConns = 20
	clone.MaxIdleConnsPerHost = 10
	clone.IdleConnTimeout = 90 * time.Second
	clone.DisableKeepAlives = false
	return clone
}
