package publicapi

import (
	"errors"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service    *Service
	authProxy  http.Handler
	statsProxy http.Handler
	queueProxy http.Handler
}

var (
	publicTimingLogOnce    sync.Once
	publicTimingLogEnabled bool
)

func shouldLogPublicTiming() bool {
	publicTimingLogOnce.Do(func() {
		raw := strings.ToLower(strings.TrimSpace(os.Getenv("PUBLIC_PROXY_TIMING_LOG")))
		publicTimingLogEnabled = raw == "1" || raw == "true" || raw == "yes" || raw == "on"
	})
	return publicTimingLogEnabled
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) SetStatisticsProxy(proxy http.Handler) {
	h.statsProxy = proxy
}

func (h *Handler) SetAuthProxy(proxy http.Handler) {
	h.authProxy = proxy
}

func (h *Handler) SetQueueProxy(proxy http.Handler) {
	h.queueProxy = proxy
}

func (h *Handler) InitializeInfo(c *gin.Context) {
	var req InitStationInfoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST"})
		return
	}

	resolvedIP := resolveStationIP(c.Request.Context(), c.ClientIP())
	info, err := h.service.InitializeStationInfo(c.Request.Context(), req.Name, req.Location, resolvedIP)
	if err != nil {
		if errors.Is(err, ErrStationAlreadyConfigured) {
			existingInfo, existingErr := h.service.GetStationInfo(c.Request.Context(), resolvedIP)
			if existingErr != nil {
				c.JSON(http.StatusOK, gin.H{
					"already_configured": true,
					"message":            "Station is already configured and cannot be configured again",
				})
				return
			}
			existingInfo.AlreadyConfigured = true
			existingInfo.Message = "Station is already configured and cannot be configured again"
			c.JSON(http.StatusOK, existingInfo)
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, info)
}

func (h *Handler) GetInfo(c *gin.Context) {
	resolvedIP := resolveStationIP(c.Request.Context(), c.ClientIP())
	info, err := h.service.GetStationInfo(c.Request.Context(), resolvedIP)
	if err != nil {
		if errors.Is(err, ErrStationNotInitialized) {
			c.JSON(http.StatusNotFound, gin.H{"error": "STATION_NOT_INITIALIZED"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		return
	}

	c.JSON(http.StatusOK, info)
}

func (h *Handler) ListRoutes(c *gin.Context) {
	routes, err := h.service.ListRouteAvailability(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, routes)
}

func (h *Handler) GetRoute(c *gin.Context) {
	destinationID := c.Param("id")
	if destinationID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_DESTINATION_ID"})
		return
	}

	details, err := h.service.GetRouteDetails(c.Request.Context(), destinationID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		return
	}
	c.JSON(http.StatusOK, details)
}

func (h *Handler) CreateBooking(c *gin.Context) {
	var req CreateBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST"})
		return
	}
	if strings.TrimSpace(req.IdempotencyKey) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "IDEMPOTENCY_KEY_REQUIRED"})
		return
	}

	booking, existing, err := h.service.CreateBookingHold(c.Request.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, ErrInsufficientSeats):
			c.JSON(http.StatusConflict, gin.H{"error": "INSUFFICIENT_SEATS"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		}
		return
	}

	if existing {
		c.JSON(http.StatusOK, booking)
		return
	}
	c.JSON(http.StatusCreated, booking)
}

func (h *Handler) GetBooking(c *gin.Context) {
	bookingID := c.Param("id")
	booking, err := h.service.GetBooking(c.Request.Context(), bookingID)
	if err != nil {
		switch {
		case errors.Is(err, ErrBookingNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "BOOKING_NOT_FOUND"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		}
		return
	}
	c.JSON(http.StatusOK, booking)
}

func (h *Handler) ConfirmBooking(c *gin.Context) {
	bookingID := c.Param("id")
	var req ConfirmBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST"})
		return
	}
	if strings.ToUpper(strings.TrimSpace(req.PaymentStatus)) != "PAID" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_PAYMENT_STATUS"})
		return
	}

	booking, err := h.service.ConfirmBooking(c.Request.Context(), bookingID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrBookingNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "BOOKING_NOT_FOUND"})
		case errors.Is(err, ErrBookingExpired):
			c.JSON(http.StatusConflict, gin.H{"error": "BOOKING_EXPIRED"})
		case errors.Is(err, ErrBookingStateConflict):
			c.JSON(http.StatusConflict, gin.H{"error": "BOOKING_STATUS_CONFLICT"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		}
		return
	}

	c.JSON(http.StatusOK, ConfirmBookingResponse{
		BookingID:           booking.BookingID,
		BookingStatus:       booking.BookingStatus,
		PaymentStatus:       booking.PaymentStatus,
		PaymentMethod:       booking.PaymentMethod,
		PaymentProcessedAt:  booking.PaymentProcessedAt,
		VehicleLicensePlate: booking.VehicleLicensePlate,
	})
}

func (h *Handler) CancelBooking(c *gin.Context) {
	bookingID := c.Param("id")
	var req CancelBookingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "INVALID_REQUEST"})
		return
	}

	booking, err := h.service.CancelBooking(c.Request.Context(), bookingID, req)
	if err != nil {
		switch {
		case errors.Is(err, ErrBookingNotFound):
			c.JSON(http.StatusNotFound, gin.H{"error": "BOOKING_NOT_FOUND"})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": "INTERNAL_ERROR"})
		}
		return
	}

	c.JSON(http.StatusOK, booking)
}

func (h *Handler) ProxyAuthLogin(c *gin.Context) {
	if h.authProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "AUTH_PROXY_DISABLED"})
		return
	}

	started := time.Now()
	c.Request.URL.Path = "/api/v1/auth/login"
	c.Request.URL.RawPath = "/api/v1/auth/login"
	h.authProxy.ServeHTTP(c.Writer, c.Request)
	if shouldLogPublicTiming() {
		log.Printf("public_timing stage=proxy_auth_login status=%d duration_ms=%d", c.Writer.Status(), time.Since(started).Milliseconds())
	}
}

func (h *Handler) ProxyStatistics(c *gin.Context) {
	if h.statsProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "STATISTICS_PROXY_DISABLED"})
		return
	}

	if c.Param("path") == "/station/today" {
		query := c.Request.URL.Query()
		if strings.TrimSpace(query.Get("date")) == "" {
			query.Set("date", time.Now().Format("2006-01-02"))
		}
		c.Request.URL.Path = "/api/v1/statistics/income/day"
		c.Request.URL.RawPath = "/api/v1/statistics/income/day"
		c.Request.URL.RawQuery = query.Encode()
	}

	h.statsProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyStatisticsOverviewDay(c *gin.Context) {
	if h.statsProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "STATISTICS_PROXY_DISABLED"})
		return
	}

	query := c.Request.URL.Query()
	if strings.TrimSpace(query.Get("date")) == "" {
		query.Set("date", time.Now().Format("2006-01-02"))
	}

	c.Request.URL.Path = "/api/v1/statistics/overview/day"
	c.Request.URL.RawPath = "/api/v1/statistics/overview/day"
	c.Request.URL.RawQuery = query.Encode()

	h.statsProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyStatisticsOverviewMonth(c *gin.Context) {
	if h.statsProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "STATISTICS_PROXY_DISABLED"})
		return
	}

	now := time.Now()
	query := c.Request.URL.Query()
	if strings.TrimSpace(query.Get("year")) == "" {
		query.Set("year", strconv.Itoa(now.Year()))
	}
	if strings.TrimSpace(query.Get("month")) == "" {
		query.Set("month", strconv.Itoa(int(now.Month())))
	}

	c.Request.URL.Path = "/api/v1/statistics/overview/month"
	c.Request.URL.RawPath = "/api/v1/statistics/overview/month"
	c.Request.URL.RawQuery = query.Encode()

	h.statsProxy.ServeHTTP(c.Writer, c.Request)
}

func (h *Handler) ProxyStatisticsWebSocket(c *gin.Context) {
	if h.statsProxy == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "STATISTICS_PROXY_DISABLED"})
		return
	}

	c.Request.URL.Path = "/api/v1/statistics/ws"
	c.Request.URL.RawPath = "/api/v1/statistics/ws"

	h.statsProxy.ServeHTTP(c.Writer, c.Request)
}
