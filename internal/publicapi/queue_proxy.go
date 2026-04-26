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

func NewQueueProxyFromEnv() (http.Handler, error) {
	upstreamURL := strings.TrimSpace(os.Getenv("PUBLIC_QUEUE_PROXY_URL"))
	if upstreamURL == "" {
		port := strings.TrimSpace(os.Getenv("QUEUE_SERVICE_PORT"))
		if port == "" {
			port = "8002"
		}
		upstreamURL = "http://localhost:" + port
	}

	target, err := url.Parse(upstreamURL)
	if err != nil {
		return nil, fmt.Errorf("invalid PUBLIC_QUEUE_PROXY_URL: %w", err)
	}
	if target.Scheme == "" || target.Host == "" {
		return nil, fmt.Errorf("invalid PUBLIC_QUEUE_PROXY_URL: %q", upstreamURL)
	}

	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.Transport = newProxyTransport(parseDurationSecondsDefault(os.Getenv("PUBLIC_QUEUE_PROXY_TIMEOUT_SECONDS"), 15*time.Second))
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, _ error) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(upstreamError{Error: "QUEUE_UPSTREAM_UNAVAILABLE"})
	}

	return proxy, nil
}
