package publicapi

import (
	"context"
	"time"
)

type fakeRepo struct {
	initFn       func(ctx context.Context, name string, location string) (*StationInfoResponse, error)
	getInfoFn    func(ctx context.Context) (*StationInfoResponse, error)
	listRoutesFn func(ctx context.Context) ([]RouteAvailability, error)
	getRouteFn   func(ctx context.Context, destinationID string) (*RouteDetailsResponse, error)
	createFn     func(ctx context.Context, req CreateBookingRequest, holdTTL time.Duration) (*BookingResponse, bool, error)
	getBookingFn func(ctx context.Context, bookingID string) (*BookingResponse, error)
	confirmFn    func(ctx context.Context, bookingID string, req ConfirmBookingRequest) (*BookingResponse, error)
	cancelFn     func(ctx context.Context, bookingID string, req CancelBookingRequest) (*BookingResponse, error)
	expireFn     func(ctx context.Context) (int, error)
}

func (f *fakeRepo) InitializeStationInfo(ctx context.Context, name string, location string) (*StationInfoResponse, error) {
	if f.initFn != nil {
		return f.initFn(ctx, name, location)
	}
	return &StationInfoResponse{StationID: "st_test", Name: name, Location: location}, nil
}

func (f *fakeRepo) GetStationInfo(ctx context.Context) (*StationInfoResponse, error) {
	if f.getInfoFn != nil {
		return f.getInfoFn(ctx)
	}
	return &StationInfoResponse{StationID: "st_test", Name: "Test", Location: "Local"}, nil
}

func (f *fakeRepo) ListRouteAvailability(ctx context.Context) ([]RouteAvailability, error) {
	if f.listRoutesFn != nil {
		return f.listRoutesFn(ctx)
	}
	return []RouteAvailability{}, nil
}

func (f *fakeRepo) GetRouteDetails(ctx context.Context, destinationID string) (*RouteDetailsResponse, error) {
	if f.getRouteFn != nil {
		return f.getRouteFn(ctx, destinationID)
	}
	return &RouteDetailsResponse{DestinationID: destinationID, Vehicles: []RouteVehicle{}}, nil
}

func (f *fakeRepo) CreateBookingHold(ctx context.Context, req CreateBookingRequest, holdTTL time.Duration) (*BookingResponse, bool, error) {
	if f.createFn != nil {
		return f.createFn(ctx, req, holdTTL)
	}
	return &BookingResponse{
		BookingID:     "bkg_test",
		DestinationID: req.DestinationID,
		SeatsBooked:   req.SeatsBooked,
		BookingStatus: "HELD",
		PaymentStatus: "UNPAID",
		CreatedAt:     time.Now(),
	}, false, nil
}

func (f *fakeRepo) GetBooking(ctx context.Context, bookingID string) (*BookingResponse, error) {
	if f.getBookingFn != nil {
		return f.getBookingFn(ctx, bookingID)
	}
	return &BookingResponse{BookingID: bookingID, BookingStatus: "HELD", PaymentStatus: "UNPAID", CreatedAt: time.Now()}, nil
}

func (f *fakeRepo) ConfirmBooking(ctx context.Context, bookingID string, req ConfirmBookingRequest) (*BookingResponse, error) {
	if f.confirmFn != nil {
		return f.confirmFn(ctx, bookingID, req)
	}
	return &BookingResponse{BookingID: bookingID, BookingStatus: "ACTIVE", PaymentStatus: "PAID", CreatedAt: time.Now()}, nil
}

func (f *fakeRepo) CancelBooking(ctx context.Context, bookingID string, req CancelBookingRequest) (*BookingResponse, error) {
	if f.cancelFn != nil {
		return f.cancelFn(ctx, bookingID, req)
	}
	return &BookingResponse{BookingID: bookingID, BookingStatus: "CANCELLED", PaymentStatus: "FAILED", CreatedAt: time.Now()}, nil
}

func (f *fakeRepo) ExpireHeldBookings(ctx context.Context) (int, error) {
	if f.expireFn != nil {
		return f.expireFn(ctx)
	}
	return 0, nil
}
