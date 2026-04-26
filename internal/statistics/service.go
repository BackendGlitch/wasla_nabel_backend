package statistics

import (
	"context"
	"fmt"
	"time"

	"station-backend/internal/pricing"
)

type Service struct {
	repo Repository
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo}
}

// LogSeatBooking logs a seat booking transaction
func (s *Service) LogSeatBooking(ctx context.Context, staffID, bookingID, stationID string, seats int) error {
	seatIncome := float64(seats) * pricing.ServiceFeePerSeatTND

	req := LogTransactionRequest{
		StaffID:         staffID,
		TransactionType: "SEAT_BOOKING",
		TransactionID:   bookingID,
		Amount:          seatIncome,
		Quantity:        seats,
		StationID:       stationID,
	}

	return s.repo.LogTransaction(ctx, req)
}

// LogDayPassSale logs a day pass sale transaction
func (s *Service) LogDayPassSale(ctx context.Context, staffID, dayPassID, stationID string) error {
	// Day pass income: 2 TND per day pass
	req := LogTransactionRequest{
		StaffID:         staffID,
		TransactionType: "DAY_PASS_SALE",
		TransactionID:   dayPassID,
		Amount:          2.0,
		Quantity:        1,
		StationID:       stationID,
	}

	return s.repo.LogTransaction(ctx, req)
}

// GetStaffDailyIncome gets daily income summary for a specific staff member
func (s *Service) GetStaffDailyIncome(ctx context.Context, staffID string, date time.Time) (*StaffIncomeSummary, error) {
	return s.repo.GetStaffDailyIncome(ctx, staffID, date)
}

// GetStationDailyIncome gets daily income summary for a specific station
func (s *Service) GetStationDailyIncome(ctx context.Context, stationID string, date time.Time) (*StationIncomeSummary, error) {
	return s.repo.GetStationDailyIncome(ctx, stationID, date)
}

// GetStaffIncomeRange gets income summary for a staff member over a date range
func (s *Service) GetStaffIncomeRange(ctx context.Context, staffID string, startDate, endDate time.Time) ([]StaffIncomeSummary, error) {
	if startDate.After(endDate) {
		return nil, fmt.Errorf("start date cannot be after end date")
	}

	// Limit date range to 1 year
	if endDate.Sub(startDate) > 365*24*time.Hour {
		return nil, fmt.Errorf("date range cannot exceed 1 year")
	}

	return s.repo.GetStaffIncomeRange(ctx, staffID, startDate, endDate)
}

// GetStationIncomeRange gets income summary for a station over a date range
func (s *Service) GetStationIncomeRange(ctx context.Context, stationID string, startDate, endDate time.Time) ([]StationIncomeSummary, error) {
	if startDate.After(endDate) {
		return nil, fmt.Errorf("start date cannot be after end date")
	}

	// Limit date range to 1 year
	if endDate.Sub(startDate) > 365*24*time.Hour {
		return nil, fmt.Errorf("date range cannot exceed 1 year")
	}

	return s.repo.GetStationIncomeRange(ctx, stationID, startDate, endDate)
}

// GetAllStaffIncomeForDate gets income summary for all staff members for a specific date
func (s *Service) GetAllStaffIncomeForDate(ctx context.Context, date time.Time) ([]StaffIncomeSummary, error) {
	return s.repo.GetAllStaffIncomeForDate(ctx, date)
}

// GetAllStaffIncomeForMonth gets income summary for all staff members for a specific month
func (s *Service) GetAllStaffIncomeForMonth(ctx context.Context, year, month int) ([]StaffIncomeSummary, error) {
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month (must be 1-12)")
	}
	if year < 2020 || year > 2099 {
		return nil, fmt.Errorf("invalid year")
	}
	return s.repo.GetAllStaffIncomeForMonth(ctx, year, month)
}

// GetAllStationIncomeForDate gets income summary for all stations for a specific date
func (s *Service) GetAllStationIncomeForDate(ctx context.Context, date time.Time) ([]StationIncomeSummary, error) {
	return s.repo.GetAllStationIncomeForDate(ctx, date)
}

// GetAllStationIncomeForMonth gets income summary for all stations for a specific month
func (s *Service) GetAllStationIncomeForMonth(ctx context.Context, year, month int) ([]StationIncomeSummary, error) {
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month (must be 1-12)")
	}
	if year < 2020 || year > 2099 {
		return nil, fmt.Errorf("invalid year")
	}
	return s.repo.GetAllStationIncomeForMonth(ctx, year, month)
}

// GetDailyOverview returns all major statistics for one date.
func (s *Service) GetDailyOverview(ctx context.Context, date time.Time) (*DailyOverview, error) {
	actual, err := s.repo.GetActualIncomeForDate(ctx, date)
	if err != nil {
		return nil, err
	}

	staff, err := s.repo.GetAllStaffIncomeForDate(ctx, date)
	if err != nil {
		return nil, err
	}

	stations, err := s.repo.GetAllStationIncomeForDate(ctx, date)
	if err != nil {
		return nil, err
	}

	return &DailyOverview{
		Date:         date,
		Actual:       actual,
		Staff:        staff,
		Stations:     stations,
		StaffCount:   len(staff),
		StationCount: len(stations),
	}, nil
}

// GetMonthlyOverview returns all major statistics for one month.
func (s *Service) GetMonthlyOverview(ctx context.Context, year, month int) (*MonthlyOverview, error) {
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month (must be 1-12)")
	}
	if year < 2020 || year > 2099 {
		return nil, fmt.Errorf("invalid year")
	}

	actual, err := s.repo.GetActualIncomeForMonth(ctx, year, month)
	if err != nil {
		return nil, err
	}

	staff, err := s.repo.GetAllStaffIncomeForMonth(ctx, year, month)
	if err != nil {
		return nil, err
	}

	stations, err := s.repo.GetAllStationIncomeForMonth(ctx, year, month)
	if err != nil {
		return nil, err
	}

	return &MonthlyOverview{
		Year:         year,
		Month:        month,
		Date:         time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC),
		Actual:       actual,
		Staff:        staff,
		Stations:     stations,
		StaffCount:   len(staff),
		StationCount: len(stations),
	}, nil
}

// GetStaffTransactionLog gets transaction log for a specific staff member
func (s *Service) GetStaffTransactionLog(ctx context.Context, staffID string, limit int) ([]StaffTransactionLog, error) {
	return s.repo.GetStaffTransactionLog(ctx, staffID, limit)
}

// GetStationTransactionLog gets transaction log for a specific station
func (s *Service) GetStationTransactionLog(ctx context.Context, stationID string, limit int) ([]StaffTransactionLog, error) {
	return s.repo.GetStationTransactionLog(ctx, stationID, limit)
}

// GetTodayStaffIncome gets today's income for a specific staff member
func (s *Service) GetTodayStaffIncome(ctx context.Context, staffID string) (*StaffIncomeSummary, error) {
	return s.repo.GetStaffDailyIncome(ctx, staffID, time.Now())
}

// GetTodayStationIncome gets today's income for a specific station
func (s *Service) GetTodayStationIncome(ctx context.Context, stationID string) (*StationIncomeSummary, error) {
	return s.repo.GetStationDailyIncome(ctx, stationID, time.Now())
}

// GetStaffIncomeThisWeek gets this week's income for a specific staff member
func (s *Service) GetStaffIncomeThisWeek(ctx context.Context, staffID string) ([]StaffIncomeSummary, error) {
	now := time.Now()
	startOfWeek := now.AddDate(0, 0, -int(now.Weekday()))
	startOfWeek = time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, startOfWeek.Location())

	return s.repo.GetStaffIncomeRange(ctx, staffID, startOfWeek, now)
}

// GetStationIncomeThisWeek gets this week's income for a specific station
func (s *Service) GetStationIncomeThisWeek(ctx context.Context, stationID string) ([]StationIncomeSummary, error) {
	now := time.Now()
	startOfWeek := now.AddDate(0, 0, -int(now.Weekday()))
	startOfWeek = time.Date(startOfWeek.Year(), startOfWeek.Month(), startOfWeek.Day(), 0, 0, 0, 0, startOfWeek.Location())

	return s.repo.GetStationIncomeRange(ctx, stationID, startOfWeek, now)
}

// GetStaffIncomeThisMonth gets this month's income for a specific staff member
func (s *Service) GetStaffIncomeThisMonth(ctx context.Context, staffID string) ([]StaffIncomeSummary, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	return s.repo.GetStaffIncomeRange(ctx, staffID, startOfMonth, now)
}

// GetStationIncomeThisMonth gets this month's income for a specific station
func (s *Service) GetStationIncomeThisMonth(ctx context.Context, stationID string) ([]StationIncomeSummary, error) {
	now := time.Now()
	startOfMonth := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())

	return s.repo.GetStationIncomeRange(ctx, stationID, startOfMonth, now)
}

// GetActualIncomeForDate gets actual income for a specific date
func (s *Service) GetActualIncomeForDate(ctx context.Context, date time.Time) (*ActualIncomeSummary, error) {
	return s.repo.GetActualIncomeForDate(ctx, date)
}

// GetActualIncomeForPeriod gets actual income for a time period
func (s *Service) GetActualIncomeForPeriod(ctx context.Context, startTime, endTime time.Time) (*ActualIncomeSummary, error) {
	if startTime.After(endTime) {
		return nil, fmt.Errorf("start time cannot be after end time")
	}
	return s.repo.GetActualIncomeForPeriod(ctx, startTime, endTime)
}

// GetActualIncomeForMonth gets actual income for a specific month
func (s *Service) GetActualIncomeForMonth(ctx context.Context, year, month int) (*ActualIncomeSummary, error) {
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month (must be 1-12)")
	}
	if year < 2020 || year > 2099 {
		return nil, fmt.Errorf("invalid year")
	}
	return s.repo.GetActualIncomeForMonth(ctx, year, month)
}

func (s *Service) GetStaffDestinationBreakdown(ctx context.Context, date time.Time) ([]StaffDestinationIncomeSummary, error) {
	return s.repo.GetStaffDestinationBreakdown(ctx, date)
}

func (s *Service) GetStaffDestinationBreakdownMonth(ctx context.Context, year, month int) ([]StaffDestinationIncomeSummary, error) {
	if month < 1 || month > 12 {
		return nil, fmt.Errorf("invalid month (must be 1-12)")
	}
	if year < 2020 || year > 2099 {
		return nil, fmt.Errorf("invalid year")
	}
	return s.repo.GetStaffDestinationBreakdownMonth(ctx, year, month)
}
