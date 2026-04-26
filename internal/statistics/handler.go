package statistics

import (
	"net/http"
	"strconv"
	"time"

	"station-backend/internal/websocket"
	"station-backend/pkg/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service     *Service
	realtimeHub *RealTimeStatsHub
}

func NewHandler(service *Service, realtimeHub *RealTimeStatsHub) *Handler {
	return &Handler{service: service, realtimeHub: realtimeHub}
}

// GetStaffDailyIncome godoc
// @Summary Get daily income for a staff member
// @Description Get daily income summary for a specific staff member
// @Tags statistics
// @Accept json
// @Produce json
// @Param staffId path string true "Staff ID"
// @Param date query string false "Date (YYYY-MM-DD format, defaults to today)"
// @Success 200 {object} utils.Response{data=StaffIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 404 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/staff/{staffId}/daily [get]
func (h *Handler) GetStaffDailyIncome(c *gin.Context) {
	staffID := c.Param("staffId")
	if staffID == "" {
		utils.BadRequestResponse(c, "Staff ID is required")
		return
	}

	// Parse date parameter, default to today
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
			return
		}
	} else {
		date = time.Now()
	}

	income, err := h.service.GetStaffDailyIncome(c.Request.Context(), staffID, date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get staff daily income", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Staff daily income retrieved", income)
}

// GetStationDailyIncome godoc
// @Summary Get daily income for a station
// @Description Get daily income summary for a specific station
// @Tags statistics
// @Accept json
// @Produce json
// @Param stationId path string true "Station ID"
// @Param date query string false "Date (YYYY-MM-DD format, defaults to today)"
// @Success 200 {object} utils.Response{data=StationIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 404 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/station/{stationId}/daily [get]
func (h *Handler) GetStationDailyIncome(c *gin.Context) {
	stationID := c.Param("stationId")
	if stationID == "" {
		utils.BadRequestResponse(c, "Station ID is required")
		return
	}

	// Parse date parameter, default to today
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
			return
		}
	} else {
		date = time.Now()
	}

	income, err := h.service.GetStationDailyIncome(c.Request.Context(), stationID, date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get station daily income", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Station daily income retrieved", income)
}

// GetStaffIncomeRange godoc
// @Summary Get income range for a staff member
// @Description Get income summary for a staff member over a date range
// @Tags statistics
// @Accept json
// @Produce json
// @Param staffId path string true "Staff ID"
// @Param startDate query string true "Start date (YYYY-MM-DD format)"
// @Param endDate query string true "End date (YYYY-MM-DD format)"
// @Success 200 {object} utils.Response{data=[]StaffIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/staff/{staffId}/range [get]
func (h *Handler) GetStaffIncomeRange(c *gin.Context) {
	staffID := c.Param("staffId")
	if staffID == "" {
		utils.BadRequestResponse(c, "Staff ID is required")
		return
	}

	startDateStr := c.Query("startDate")
	endDateStr := c.Query("endDate")

	if startDateStr == "" || endDateStr == "" {
		utils.BadRequestResponse(c, "Start date and end date are required")
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid start date format. Use YYYY-MM-DD")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid end date format. Use YYYY-MM-DD")
		return
	}

	income, err := h.service.GetStaffIncomeRange(c.Request.Context(), staffID, startDate, endDate)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get staff income range", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Staff income range retrieved", income)
}

// GetStationIncomeRange godoc
// @Summary Get income range for a station
// @Description Get income summary for a station over a date range
// @Tags statistics
// @Accept json
// @Produce json
// @Param stationId path string true "Station ID"
// @Param startDate query string true "Start date (YYYY-MM-DD format)"
// @Param endDate query string true "End date (YYYY-MM-DD format)"
// @Success 200 {object} utils.Response{data=[]StationIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/station/{stationId}/range [get]
func (h *Handler) GetStationIncomeRange(c *gin.Context) {
	stationID := c.Param("stationId")
	if stationID == "" {
		utils.BadRequestResponse(c, "Station ID is required")
		return
	}

	startDateStr := c.Query("startDate")
	endDateStr := c.Query("endDate")

	if startDateStr == "" || endDateStr == "" {
		utils.BadRequestResponse(c, "Start date and end date are required")
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid start date format. Use YYYY-MM-DD")
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid end date format. Use YYYY-MM-DD")
		return
	}

	income, err := h.service.GetStationIncomeRange(c.Request.Context(), stationID, startDate, endDate)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get station income range", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Station income range retrieved", income)
}

// GetAllStaffIncomeForDate godoc
// @Summary Get all staff income for a date
// @Description Get income summary for all staff members for a specific date
// @Tags statistics
// @Accept json
// @Produce json
// @Param date query string false "Date (YYYY-MM-DD format, defaults to today)"
// @Success 200 {object} utils.Response{data=[]StaffIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/staff/all [get]
func (h *Handler) GetAllStaffIncomeForDate(c *gin.Context) {
	// Parse date parameter, default to today
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
			return
		}
	} else {
		date = time.Now()
	}

	income, err := h.service.GetAllStaffIncomeForDate(c.Request.Context(), date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get all staff income", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "All staff income retrieved", income)
}

// GetAllStationIncomeForDate godoc
// @Summary Get all station income for a date
// @Description Get income summary for all stations for a specific date
// @Tags statistics
// @Accept json
// @Produce json
// @Param date query string false "Date (YYYY-MM-DD format, defaults to today)"
// @Success 200 {object} utils.Response{data=[]StationIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/station/all [get]
func (h *Handler) GetAllStationIncomeForDate(c *gin.Context) {
	// Parse date parameter, default to today
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
			return
		}
	} else {
		date = time.Now()
	}

	income, err := h.service.GetAllStationIncomeForDate(c.Request.Context(), date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get all station income", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "All station income retrieved", income)
}

// GetAllStaffIncomeForMonth godoc
// @Summary Get all staff income for a month
// @Description Get income summary for all staff members for a specific month
// @Tags statistics
// @Accept json
// @Produce json
// @Param year query int true "Year"
// @Param month query int true "Month (1-12)"
// @Success 200 {object} utils.Response{data=[]StaffIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/staff/all-month [get]
func (h *Handler) GetAllStaffIncomeForMonth(c *gin.Context) {
	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if yearStr == "" || monthStr == "" {
		utils.BadRequestResponse(c, "Year and month are required")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2020 || year > 2099 {
		utils.BadRequestResponse(c, "Invalid year")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		utils.BadRequestResponse(c, "Invalid month (must be 1-12)")
		return
	}

	income, err := h.service.GetAllStaffIncomeForMonth(c.Request.Context(), year, month)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get all staff income for month", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "All staff income for month retrieved", income)
}

// GetAllStationIncomeForMonth godoc
// @Summary Get all station income for a month
// @Description Get income summary for all stations for a specific month
// @Tags statistics
// @Accept json
// @Produce json
// @Param year query int true "Year"
// @Param month query int true "Month (1-12)"
// @Success 200 {object} utils.Response{data=[]StationIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/station/all-month [get]
func (h *Handler) GetAllStationIncomeForMonth(c *gin.Context) {
	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if yearStr == "" || monthStr == "" {
		utils.BadRequestResponse(c, "Year and month are required")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2020 || year > 2099 {
		utils.BadRequestResponse(c, "Invalid year")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		utils.BadRequestResponse(c, "Invalid month (must be 1-12)")
		return
	}

	income, err := h.service.GetAllStationIncomeForMonth(c.Request.Context(), year, month)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get all station income for month", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "All station income for month retrieved", income)
}

// GetOverviewForDay godoc
// @Summary Get full statistics overview for a day
// @Description Get actual income + all staff + all stations for a specific day
// @Tags statistics
// @Accept json
// @Produce json
// @Param date query string false "Date (YYYY-MM-DD format, defaults to today)"
// @Success 200 {object} utils.Response{data=DailyOverview}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/overview/day [get]
func (h *Handler) GetOverviewForDay(c *gin.Context) {
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
			return
		}
	} else {
		date = time.Now()
	}

	overview, err := h.service.GetDailyOverview(c.Request.Context(), date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get daily overview", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Daily overview retrieved", overview)
}

// GetOverviewForMonth godoc
// @Summary Get full statistics overview for a month
// @Description Get actual income + all staff + all stations for a specific month
// @Tags statistics
// @Accept json
// @Produce json
// @Param year query int false "Year (defaults to current year)"
// @Param month query int false "Month (1-12, defaults to current month)"
// @Success 200 {object} utils.Response{data=MonthlyOverview}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/overview/month [get]
func (h *Handler) GetOverviewForMonth(c *gin.Context) {
	now := time.Now()
	year := now.Year()
	month := int(now.Month())

	yearStr := c.Query("year")
	if yearStr != "" {
		parsedYear, err := strconv.Atoi(yearStr)
		if err != nil || parsedYear < 2020 || parsedYear > 2099 {
			utils.BadRequestResponse(c, "Invalid year")
			return
		}
		year = parsedYear
	}

	monthStr := c.Query("month")
	if monthStr != "" {
		parsedMonth, err := strconv.Atoi(monthStr)
		if err != nil || parsedMonth < 1 || parsedMonth > 12 {
			utils.BadRequestResponse(c, "Invalid month (must be 1-12)")
			return
		}
		month = parsedMonth
	}

	overview, err := h.service.GetMonthlyOverview(c.Request.Context(), year, month)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get monthly overview", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Monthly overview retrieved", overview)
}

// GetStaffTransactionLog godoc
// @Summary Get transaction log for a staff member
// @Description Get transaction log for a specific staff member
// @Tags statistics
// @Accept json
// @Produce json
// @Param staffId path string true "Staff ID"
// @Param limit query int false "Limit number of records (default: 100, max: 1000)"
// @Success 200 {object} utils.Response{data=[]StaffTransactionLog}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/staff/{staffId}/transactions [get]
func (h *Handler) GetStaffTransactionLog(c *gin.Context) {
	staffID := c.Param("staffId")
	if staffID == "" {
		utils.BadRequestResponse(c, "Staff ID is required")
		return
	}

	limitStr := c.Query("limit")
	limit := 100
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			utils.BadRequestResponse(c, "Invalid limit value")
			return
		}
	}

	logs, err := h.service.GetStaffTransactionLog(c.Request.Context(), staffID, limit)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get staff transaction log", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Staff transaction log retrieved", logs)
}

// GetStationTransactionLog godoc
// @Summary Get transaction log for a station
// @Description Get transaction log for a specific station
// @Tags statistics
// @Accept json
// @Produce json
// @Param stationId path string true "Station ID"
// @Param limit query int false "Limit number of records (default: 100, max: 1000)"
// @Success 200 {object} utils.Response{data=[]StaffTransactionLog}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/station/{stationId}/transactions [get]
func (h *Handler) GetStationTransactionLog(c *gin.Context) {
	stationID := c.Param("stationId")
	if stationID == "" {
		utils.BadRequestResponse(c, "Station ID is required")
		return
	}

	limitStr := c.Query("limit")
	limit := 100
	if limitStr != "" {
		var err error
		limit, err = strconv.Atoi(limitStr)
		if err != nil || limit <= 0 {
			utils.BadRequestResponse(c, "Invalid limit value")
			return
		}
	}

	logs, err := h.service.GetStationTransactionLog(c.Request.Context(), stationID, limit)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get station transaction log", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Station transaction log retrieved", logs)
}

// GetTodayStaffIncome godoc
// @Summary Get today's income for a staff member
// @Description Get today's income summary for a specific staff member
// @Tags statistics
// @Accept json
// @Produce json
// @Param staffId path string true "Staff ID"
// @Success 200 {object} utils.Response{data=StaffIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 404 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/staff/{staffId}/today [get]
func (h *Handler) GetTodayStaffIncome(c *gin.Context) {
	staffID := c.Param("staffId")
	if staffID == "" {
		utils.BadRequestResponse(c, "Staff ID is required")
		return
	}

	income, err := h.service.GetTodayStaffIncome(c.Request.Context(), staffID)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get today's staff income", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Today's staff income retrieved", income)
}

// GetTodayStationIncome godoc
// @Summary Get today's income for a station
// @Description Get today's income summary for a specific station
// @Tags statistics
// @Accept json
// @Produce json
// @Param stationId path string true "Station ID"
// @Success 200 {object} utils.Response{data=StationIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 404 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/station/{stationId}/today [get]
func (h *Handler) GetTodayStationIncome(c *gin.Context) {
	stationID := c.Param("stationId")
	if stationID == "" {
		utils.BadRequestResponse(c, "Station ID is required")
		return
	}

	income, err := h.service.GetTodayStationIncome(c.Request.Context(), stationID)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get today's station income", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Today's station income retrieved", income)
}

// WebSocketStats godoc
// @Summary WebSocket connection for real-time statistics
// @Description Establishes a WebSocket connection to receive real-time statistics updates
// @Tags statistics
// @Accept json
// @Produce json
// @Success 101 {object} string "WebSocket connection established"
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/ws [get]
func (h *Handler) WebSocketStats(c *gin.Context) {
	if h.realtimeHub == nil || h.realtimeHub.wsHub == nil {
		utils.InternalServerErrorResponse(c, "Real-time statistics not available", nil)
		return
	}

	// Use the existing websocket ServeWS function
	websocket.ServeWS(h.realtimeHub.wsHub, c.Writer, c.Request, "*", "statistics-client")
}

// GetActualIncome godoc
// @Summary Get actual income with destination base prices
// @Description Get actual income calculated as (destination base price + 0.2 TND) per seat
// @Tags statistics
// @Accept json
// @Produce json
// @Param date query string false "Date (YYYY-MM-DD format, defaults to today)"
// @Success 200 {object} utils.Response{data=ActualIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/income/actual [get]
func (h *Handler) GetActualIncome(c *gin.Context) {
	dateStr := c.Query("date")
	var date time.Time
	var err error

	if dateStr != "" {
		date, err = time.Parse("2006-01-02", dateStr)
		if err != nil {
			utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
			return
		}
	} else {
		date = time.Now()
	}

	income, err := h.service.GetActualIncomeForDate(c.Request.Context(), date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get actual income", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Actual income retrieved", income)
}

// GetActualIncomeForPeriod godoc
// @Summary Get actual income for a time period
// @Description Get actual income for a specific time period
// @Tags statistics
// @Accept json
// @Produce json
// @Param start query string true "Start time (ISO 8601 format)"
// @Param end query string true "End time (ISO 8601 format)"
// @Success 200 {object} utils.Response{data=ActualIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/income/period [get]
func (h *Handler) GetActualIncomeForPeriod(c *gin.Context) {
	startStr := c.Query("start")
	endStr := c.Query("end")

	if startStr == "" || endStr == "" {
		utils.BadRequestResponse(c, "Start and end times are required")
		return
	}

	startTime, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid start time format")
		return
	}

	endTime, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid end time format")
		return
	}

	income, err := h.service.GetActualIncomeForPeriod(c.Request.Context(), startTime, endTime)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get actual income for period", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Actual income for period retrieved", income)
}

// GetActualIncomeForDay godoc
// @Summary Get actual income for a specific day
// @Description Get actual income for a specific day
// @Tags statistics
// @Accept json
// @Produce json
// @Param date query string true "Date (YYYY-MM-DD format)"
// @Success 200 {object} utils.Response{data=ActualIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/income/day [get]
func (h *Handler) GetActualIncomeForDay(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		utils.BadRequestResponse(c, "Date is required")
		return
	}

	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
		return
	}

	income, err := h.service.GetActualIncomeForDate(c.Request.Context(), date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get actual income for day", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Actual income for day retrieved", income)
}

// GetActualIncomeForMonth godoc
// @Summary Get actual income for a month
// @Description Get actual income for a specific month
// @Tags statistics
// @Accept json
// @Produce json
// @Param year query int true "Year"
// @Param month query int true "Month (1-12)"
// @Success 200 {object} utils.Response{data=ActualIncomeSummary}
// @Failure 400 {object} utils.ErrorResponse
// @Failure 500 {object} utils.ErrorResponse
// @Router /statistics/income/month [get]
func (h *Handler) GetActualIncomeForMonth(c *gin.Context) {
	yearStr := c.Query("year")
	monthStr := c.Query("month")

	if yearStr == "" || monthStr == "" {
		utils.BadRequestResponse(c, "Year and month are required")
		return
	}

	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2020 || year > 2099 {
		utils.BadRequestResponse(c, "Invalid year")
		return
	}

	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		utils.BadRequestResponse(c, "Invalid month (must be 1-12)")
		return
	}

	income, err := h.service.GetActualIncomeForMonth(c.Request.Context(), year, month)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get actual income for month", err)
		return
	}

	utils.SuccessResponse(c, http.StatusOK, "Actual income for month retrieved", income)
}

func (h *Handler) GetStaffDestinationBreakdown(c *gin.Context) {
	dateStr := c.Query("date")
	if dateStr == "" {
		dateStr = time.Now().Format("2006-01-02")
	}
	date, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		utils.BadRequestResponse(c, "Invalid date format. Use YYYY-MM-DD")
		return
	}
	data, err := h.service.GetStaffDestinationBreakdown(c.Request.Context(), date)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get staff-destination breakdown", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Staff-destination breakdown retrieved", data)
}

func (h *Handler) GetStaffDestinationBreakdownMonth(c *gin.Context) {
	yearStr := c.Query("year")
	monthStr := c.Query("month")
	if yearStr == "" || monthStr == "" {
		utils.BadRequestResponse(c, "Year and month are required")
		return
	}
	year, err := strconv.Atoi(yearStr)
	if err != nil || year < 2020 || year > 2099 {
		utils.BadRequestResponse(c, "Invalid year")
		return
	}
	month, err := strconv.Atoi(monthStr)
	if err != nil || month < 1 || month > 12 {
		utils.BadRequestResponse(c, "Invalid month (1-12)")
		return
	}
	data, err := h.service.GetStaffDestinationBreakdownMonth(c.Request.Context(), year, month)
	if err != nil {
		utils.InternalServerErrorResponse(c, "Failed to get staff-destination breakdown for month", err)
		return
	}
	utils.SuccessResponse(c, http.StatusOK, "Staff-destination monthly breakdown retrieved", data)
}

// TriggerBroadcast triggers a real-time statistics update broadcast
func (h *Handler) TriggerBroadcast(c *gin.Context) {
	if h.realtimeHub == nil || h.realtimeHub.wsHub == nil {
		utils.InternalServerErrorResponse(c, "Real-time statistics not available", nil)
		return
	}

	// Force a broadcast of current statistics
	h.realtimeHub.BroadcastPeriodicUpdates()

	utils.SuccessResponse(c, http.StatusOK, "Statistics broadcast triggered", nil)
}
