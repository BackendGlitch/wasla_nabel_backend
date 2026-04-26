package publicapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func performJSONRequest(r http.Handler, method, path string, payload interface{}) *httptest.ResponseRecorder {
	body, _ := json.Marshal(payload)
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func TestCreateBookingRequiresIdempotencyKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	r := gin.New()
	r.POST("/bookings", h.CreateBooking)

	w := performJSONRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
		"destination_id": "station-teboulba",
		"seats_booked":   2,
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateBookingInsufficientSeats(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{
		createFn: func(ctx context.Context, req CreateBookingRequest, holdTTL time.Duration) (*BookingResponse, bool, error) {
			return nil, false, ErrInsufficientSeats
		},
	}

	svc := NewService(repo, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	r := gin.New()
	r.POST("/bookings", h.CreateBooking)

	w := performJSONRequest(r, http.MethodPost, "/bookings", map[string]interface{}{
		"destination_id":  "station-teboulba",
		"seats_booked":    99,
		"idempotency_key": "idem-test-1",
	})

	if w.Code != http.StatusConflict {
		t.Fatalf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestConfirmBookingRejectsNonPaidStatus(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	r := gin.New()
	r.POST("/bookings/:id/confirm", h.ConfirmBooking)

	w := performJSONRequest(r, http.MethodPost, "/bookings/bkg1/confirm", map[string]interface{}{
		"payment_status": "FAILED",
		"payment_method": "CLICK",
	})

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestConfirmBookingReturnsLicensePlateWithoutQueueID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{
		confirmFn: func(ctx context.Context, bookingID string, req ConfirmBookingRequest) (*BookingResponse, error) {
			now := time.Now()
			return &BookingResponse{
				BookingID:           bookingID,
				QueueID:             "q_123",
				VehicleLicensePlate: "123 TUN 4567",
				BookingStatus:       "ACTIVE",
				PaymentStatus:       "PAID",
				PaymentMethod:       req.PaymentMethod,
				PaymentProcessedAt:  &now,
				CreatedAt:           now,
			}, nil
		},
	}
	svc := NewService(repo, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	r := gin.New()
	r.POST("/bookings/:id/confirm", h.ConfirmBooking)

	w := performJSONRequest(r, http.MethodPost, "/bookings/bkg_test/confirm", map[string]interface{}{
		"payment_status": "PAID",
		"payment_method": "CLICK",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}

	if _, hasQueueID := resp["queue_id"]; hasQueueID {
		t.Fatalf("did not expect queue_id in confirm response: %v", resp)
	}
	if resp["vehicle_license_plate"] != "123 TUN 4567" {
		t.Fatalf("expected vehicle_license_plate to be present, got: %v", resp["vehicle_license_plate"])
	}
}

func TestGetInfoNotInitialized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{
		getInfoFn: func(ctx context.Context) (*StationInfoResponse, error) {
			return nil, ErrStationNotInitialized
		},
	}
	svc := NewService(repo, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	r := gin.New()
	r.GET("/info", h.GetInfo)

	req := httptest.NewRequest(http.MethodGet, "/info", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestInitializeInfoAlreadyConfigured(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &fakeRepo{
		initFn: func(ctx context.Context, name string, location string) (*StationInfoResponse, error) {
			return nil, ErrStationAlreadyConfigured
		},
		getInfoFn: func(ctx context.Context) (*StationInfoResponse, error) {
			return &StationInfoResponse{
				StationID: "st_existing",
				Name:      "Sousse Station",
				Location:  "Sahloul",
			}, nil
		},
	}
	svc := NewService(repo, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	r := gin.New()
	r.POST("/info", h.InitializeInfo)

	w := performJSONRequest(r, http.MethodPost, "/info", map[string]interface{}{
		"name":     "Sousse Station",
		"location": "Sahloul",
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	var resp StationInfoResponse
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse json: %v", err)
	}
	if !resp.AlreadyConfigured {
		t.Fatalf("expected already_configured=true, got false")
	}
	if resp.Message == "" {
		t.Fatalf("expected non-empty message")
	}
	if resp.StationID != "st_existing" {
		t.Fatalf("expected existing station id, got %s", resp.StationID)
	}
}

func TestProxyAuthLoginRewritesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	var proxiedPath string
	h.SetAuthProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.POST("/api/v1/auth/login", h.ProxyAuthLogin)

	w := performJSONRequest(r, http.MethodPost, "/api/v1/auth/login", map[string]interface{}{
		"cin": "14045739",
	})
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if proxiedPath != "/api/v1/auth/login" {
		t.Fatalf("expected proxied path to stay auth login, got %q", proxiedPath)
	}
}

func TestProxyStatisticsRewritesStationTodayPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	var proxiedPath string
	var proxiedQuery string
	h.SetStatisticsProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedPath = r.URL.Path
		proxiedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	stats := r.Group("/api/v1/statistics")
	{
		stats.Any("", h.ProxyStatistics)
		stats.Any("/*path", h.ProxyStatistics)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/statistics/station/today", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if proxiedPath != "/api/v1/statistics/income/day" {
		t.Fatalf("expected rewritten path, got %q", proxiedPath)
	}

	q, err := url.ParseQuery(proxiedQuery)
	if err != nil {
		t.Fatalf("failed to parse proxied query: %v", err)
	}
	if q.Get("date") == "" {
		t.Fatalf("expected date query to be injected, got query=%q", proxiedQuery)
	}
}

func TestProxyStatisticsOverviewDayDefaultsDate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	var proxiedPath string
	var proxiedQuery string
	h.SetStatisticsProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedPath = r.URL.Path
		proxiedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.GET("/api/v1/public-statistics/overview/day", h.ProxyStatisticsOverviewDay)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public-statistics/overview/day", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if proxiedPath != "/api/v1/statistics/overview/day" {
		t.Fatalf("expected rewritten path, got %q", proxiedPath)
	}

	q, err := url.ParseQuery(proxiedQuery)
	if err != nil {
		t.Fatalf("failed to parse proxied query: %v", err)
	}
	if q.Get("date") == "" {
		t.Fatalf("expected date query to be injected, got query=%q", proxiedQuery)
	}
}

func TestProxyStatisticsOverviewMonthDefaultsYearAndMonth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	var proxiedPath string
	var proxiedQuery string
	h.SetStatisticsProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedPath = r.URL.Path
		proxiedQuery = r.URL.RawQuery
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.GET("/api/v1/public-statistics/overview/month", h.ProxyStatisticsOverviewMonth)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public-statistics/overview/month", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if proxiedPath != "/api/v1/statistics/overview/month" {
		t.Fatalf("expected rewritten path, got %q", proxiedPath)
	}

	q, err := url.ParseQuery(proxiedQuery)
	if err != nil {
		t.Fatalf("failed to parse proxied query: %v", err)
	}
	if q.Get("year") == "" {
		t.Fatalf("expected year query to be injected, got query=%q", proxiedQuery)
	}
	if q.Get("month") == "" {
		t.Fatalf("expected month query to be injected, got query=%q", proxiedQuery)
	}
}

func TestProxyStatisticsWebSocketRewritesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	var proxiedPath string
	h.SetStatisticsProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.GET("/api/v1/public-statistics/ws", h.ProxyStatisticsWebSocket)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public-statistics/ws", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if proxiedPath != "/api/v1/statistics/ws" {
		t.Fatalf("expected rewritten path, got %q", proxiedPath)
	}
}

func TestProxyStaffCRUDRewritesPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	type proxiedRequest struct {
		method string
		path   string
	}
	var requests []proxiedRequest
	h.SetAuthProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, proxiedRequest{
			method: r.Method,
			path:   r.URL.Path,
		})
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.GET("/api/v1/public-staff", h.ProxyStaffList)
	r.POST("/api/v1/public-staff", h.ProxyStaffCreate)
	r.GET("/api/v1/public-staff/:id", h.ProxyStaffGet)
	r.PUT("/api/v1/public-staff/:id", h.ProxyStaffUpdate)
	r.DELETE("/api/v1/public-staff/:id", h.ProxyStaffDelete)

	reqs := []struct {
		method       string
		path         string
		expectedPath string
	}{
		{http.MethodGet, "/api/v1/public-staff", "/api/v1/staff/"},
		{http.MethodPost, "/api/v1/public-staff", "/api/v1/staff/"},
		{http.MethodGet, "/api/v1/public-staff/staff_1", "/api/v1/staff/staff_1"},
		{http.MethodPut, "/api/v1/public-staff/staff_1", "/api/v1/staff/staff_1"},
		{http.MethodDelete, "/api/v1/public-staff/staff_1", "/api/v1/staff/staff_1"},
	}

	for _, tc := range reqs {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s %s expected 204, got %d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	if len(requests) != len(reqs) {
		t.Fatalf("expected %d proxied calls, got %d", len(reqs), len(requests))
	}
	for i, tc := range reqs {
		if requests[i].path != tc.expectedPath {
			t.Fatalf("call %d expected path %q got %q", i, tc.expectedPath, requests[i].path)
		}
		if requests[i].method != tc.method {
			t.Fatalf("call %d expected method %q got %q", i, tc.method, requests[i].method)
		}
	}
}

func TestProxyVehiclesCRUDRewritesPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	type proxiedRequest struct {
		method string
		path   string
	}
	var requests []proxiedRequest
	h.SetQueueProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, proxiedRequest{
			method: r.Method,
			path:   r.URL.Path,
		})
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.GET("/api/v1/public-vehicles", h.ProxyVehiclesList)
	r.GET("/api/v1/public-vehicles/search", h.ProxyVehiclesSearch)
	r.POST("/api/v1/public-vehicles", h.ProxyVehiclesCreate)
	r.PUT("/api/v1/public-vehicles/:id", h.ProxyVehiclesUpdate)
	r.DELETE("/api/v1/public-vehicles/:id", h.ProxyVehiclesDelete)

	reqs := []struct {
		method       string
		path         string
		expectedPath string
	}{
		{http.MethodGet, "/api/v1/public-vehicles", "/api/v1/vehicles"},
		{http.MethodGet, "/api/v1/public-vehicles/search?q=12", "/api/v1/vehicles/search"},
		{http.MethodPost, "/api/v1/public-vehicles", "/api/v1/vehicles"},
		{http.MethodPut, "/api/v1/public-vehicles/veh_1", "/api/v1/vehicles/veh_1"},
		{http.MethodDelete, "/api/v1/public-vehicles/veh_1", "/api/v1/vehicles/veh_1"},
	}

	for _, tc := range reqs {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s %s expected 204, got %d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	if len(requests) != len(reqs) {
		t.Fatalf("expected %d proxied calls, got %d", len(reqs), len(requests))
	}
	for i, tc := range reqs {
		if requests[i].path != tc.expectedPath {
			t.Fatalf("call %d expected path %q got %q", i, tc.expectedPath, requests[i].path)
		}
		if requests[i].method != tc.method {
			t.Fatalf("call %d expected method %q got %q", i, tc.method, requests[i].method)
		}
	}
}

func TestProxyRoutesListRewritesPath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	var proxiedPath string
	h.SetQueueProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		proxiedPath = r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.GET("/api/v1/public-routes", h.ProxyRoutesList)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/public-routes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d body=%s", w.Code, w.Body.String())
	}
	if proxiedPath != "/api/v1/routes" {
		t.Fatalf("expected rewritten path, got %q", proxiedPath)
	}
}

func TestProxyVehicleAuthorizedRoutesRewritesPaths(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := NewService(&fakeRepo{}, 10*time.Minute, 30*time.Second, time.Now())
	h := NewHandler(svc)

	type proxiedRequest struct {
		method string
		path   string
	}
	var requests []proxiedRequest
	h.SetQueueProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, proxiedRequest{
			method: r.Method,
			path:   r.URL.Path,
		})
		w.WriteHeader(http.StatusNoContent)
	}))

	r := gin.New()
	r.GET("/api/v1/public-vehicles/:id/authorized-routes", h.ProxyVehicleAuthorizedRoutesList)
	r.POST("/api/v1/public-vehicles/:id/authorized-routes", h.ProxyVehicleAuthorizedRoutesAdd)
	r.PUT("/api/v1/public-vehicles/:id/authorized-routes/:authId", h.ProxyVehicleAuthorizedRoutesUpdate)
	r.DELETE("/api/v1/public-vehicles/:id/authorized-routes/:authId", h.ProxyVehicleAuthorizedRoutesDelete)

	reqs := []struct {
		method       string
		path         string
		expectedPath string
	}{
		{http.MethodGet, "/api/v1/public-vehicles/veh_1/authorized-routes", "/api/v1/vehicles/veh_1/authorized-routes"},
		{http.MethodPost, "/api/v1/public-vehicles/veh_1/authorized-routes", "/api/v1/vehicles/veh_1/authorized-routes"},
		{http.MethodPut, "/api/v1/public-vehicles/veh_1/authorized-routes/auth_1", "/api/v1/vehicles/veh_1/authorized-routes/auth_1"},
		{http.MethodDelete, "/api/v1/public-vehicles/veh_1/authorized-routes/auth_1", "/api/v1/vehicles/veh_1/authorized-routes/auth_1"},
	}

	for _, tc := range reqs {
		req := httptest.NewRequest(tc.method, tc.path, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNoContent {
			t.Fatalf("%s %s expected 204, got %d body=%s", tc.method, tc.path, w.Code, w.Body.String())
		}
	}

	if len(requests) != len(reqs) {
		t.Fatalf("expected %d proxied calls, got %d", len(reqs), len(requests))
	}
	for i, tc := range reqs {
		if requests[i].path != tc.expectedPath {
			t.Fatalf("call %d expected path %q got %q", i, tc.expectedPath, requests[i].path)
		}
		if requests[i].method != tc.method {
			t.Fatalf("call %d expected method %q got %q", i, tc.method, requests[i].method)
		}
	}
}
