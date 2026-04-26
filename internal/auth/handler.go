package auth

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"station-backend/internal/models"
	"station-backend/pkg/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *Service
}

var (
	authTimingLogOnce    sync.Once
	authTimingLogEnabled bool
)

func shouldLogAuthTiming() bool {
	authTimingLogOnce.Do(func() {
		raw := strings.ToLower(strings.TrimSpace(os.Getenv("AUTH_TIMING_LOG")))
		authTimingLogEnabled = raw == "1" || raw == "true" || raw == "yes" || raw == "on"
	})
	return authTimingLogEnabled
}

func NewHandler(service *Service) *Handler {
	return &Handler{service: service}
}

// ===== Staff CRUD =====
func (h *Handler) ListStaff(c *gin.Context) {
	list, err := h.service.ListStaff(c.Request.Context())
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to list staff", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Staff fetched", list)
}

func (h *Handler) GetStaff(c *gin.Context) {
	id := c.Param("id")
	s, err := h.service.GetStaffByID(c.Request.Context(), id)
	if err != nil {
		utils.NotFoundResponse(c, "Staff not found")
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Staff fetched", s)
}

func (h *Handler) CreateStaff(c *gin.Context) {
	var in struct {
		CIN         string `json:"cin"`
		PhoneNumber string `json:"phoneNumber"`
		FirstName   string `json:"firstName"`
		LastName    string `json:"lastName"`
		Role        string `json:"role"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	s := models.Staff{CIN: in.CIN, PhoneNumber: in.PhoneNumber, FirstName: in.FirstName, LastName: in.LastName, Role: in.Role, IsActive: true}
	out, err := h.service.CreateStaff(c.Request.Context(), s)
	if err != nil {
		utils.BadRequestResponse(c, err.Error())
		return
	}
	utils.SuccessResponse(c, http.StatusCreated, "Staff created", out)
}

func (h *Handler) UpdateStaff(c *gin.Context) {
	id := c.Param("id")
	var in struct {
		PhoneNumber string `json:"phoneNumber"`
		FirstName   string `json:"firstName"`
		LastName    string `json:"lastName"`
		Role        string `json:"role"`
		IsActive    *bool  `json:"isActive"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		utils.BadRequestResponse(c, "Invalid request")
		return
	}
	s := models.Staff{PhoneNumber: in.PhoneNumber, FirstName: in.FirstName, LastName: in.LastName, Role: in.Role}
	if in.IsActive != nil {
		s.IsActive = *in.IsActive
	}
	out, err := h.service.UpdateStaff(c.Request.Context(), id, s)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to update staff", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Staff updated", out)
}

func (h *Handler) DeleteStaff(c *gin.Context) {
	id := c.Param("id")
	if err := h.service.DeleteStaff(c.Request.Context(), id); err != nil {
		utils.InternalServerErrorResponse(c, "Failed to delete staff", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Staff deleted", nil)
}

func (h *Handler) Login(c *gin.Context) {
	reqStarted := time.Now()
	var req models.LoginRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		utils.BadRequestResponse(c, "Invalid request format")
		return
	}

	// Validate CIN
	validateStarted := time.Now()
	staff, err := h.service.ValidateStaff(c.Request.Context(), req.CIN)
	validateDuration := time.Since(validateStarted)
	if err != nil {
		if shouldLogAuthTiming() {
			log.Printf("auth_timing stage=login status=unauthorized cin=%s validate_ms=%d total_ms=%d", req.CIN, validateDuration.Milliseconds(), time.Since(reqStarted).Milliseconds())
		}
		utils.UnauthorizedResponse(c, "Invalid CIN or inactive account")
		return
	}

	// Generate JWT token
	tokenStarted := time.Now()
	token, err := h.service.GenerateToken(staff)
	tokenDuration := time.Since(tokenStarted)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Token generation failed", err)
		return
	}

	// Store session in Redis asynchronously — the signed JWT is already
	// self-contained so we can respond to the client immediately and let
	// the Redis write finish in the background.
	redisStarted := time.Now()
	go func(staffID, tok string) {
		bgCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		if storeErr := h.service.StoreSession(bgCtx, staffID, tok); storeErr != nil {
			log.Printf("auth_warning: async session store failed staff_id=%s err=%v", staffID, storeErr)
		}
		if shouldLogAuthTiming() {
			log.Printf("auth_timing stage=redis_store_async staff_id=%s duration_ms=%d", staffID, time.Since(redisStarted).Milliseconds())
		}
	}(staff.ID, token)

	response := models.LoginResponse{
		Token: token,
		Staff: *staff,
	}

	if shouldLogAuthTiming() {
		log.Printf("auth_timing stage=login status=ok cin=%s staff_id=%s validate_ms=%d jwt_ms=%d total_ms=%d",
			req.CIN,
			staff.ID,
			validateDuration.Milliseconds(),
			tokenDuration.Milliseconds(),
			time.Since(reqStarted).Milliseconds(),
		)
	}

	utils.SuccessResponse(c, http.StatusOK, "Login successful", response)
}

func (h *Handler) RefreshToken(c *gin.Context) {
	// Get staff ID from context (set by AuthRequired middleware)
	staffID, exists := c.Get("staff_id")
	if !exists {
		utils.UnauthorizedResponse(c, "Staff ID not found in context")
		return
	}

	// Validate current session
	valid, err := h.service.ValidateSession(staffID.(string))
	if err != nil {
		utils.InternalServerErrorResponse(c, "Session validation failed", err)
		return
	}

	if !valid {
		utils.UnauthorizedResponse(c, "Invalid session")
		return
	}

	// Generate new token
	staff := &models.Staff{ID: staffID.(string)}
	token, err := h.service.GenerateToken(staff)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Token generation failed", err)
		return
	}

	// Update session in Redis
	err = h.service.StoreSession(c.Request.Context(), staff.ID, token)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Session update failed", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Token refreshed successfully", gin.H{
		"token": token,
	})
}

func (h *Handler) Logout(c *gin.Context) {
	// Get staff ID from context
	staffID, exists := c.Get("staff_id")
	if !exists {
		utils.UnauthorizedResponse(c, "Staff ID not found in context")
		return
	}

	// Remove session from Redis
	err := h.service.Logout(staffID.(string))
	if err != nil {
		utils.InternalServerErrorResponse(c, "Logout failed", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Logout successful", nil)
}
