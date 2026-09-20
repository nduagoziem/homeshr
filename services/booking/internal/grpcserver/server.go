// Package grpcserver adapts the booking domain service to the generated gRPC
// BookingServiceServer interface. It performs no business logic of its own: it
// maps requests/responses between proto types and the booking package, unpacks
// the JWT-verified identity Envoy forwards as the x-user-id metadata header, and
// translates domain errors into gRPC status codes (which Envoy, configured with
// convert_grpc_status, turns back into HTTP+JSON for the client).
package grpcserver

import (
	"context"
	"errors"
	"log"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/nduagoziem/homeshr/services/booking/booking"
	"github.com/nduagoziem/homeshr/services/booking/internal/cache"
	bookingpb "github.com/nduagoziem/homeshr/services/booking/proto"
)

// dateLayout is the ISO-8601 calendar-day format used for dates on the wire.
const dateLayout = "2006-01-02"

// Server implements bookingpb.BookingServiceServer.
type Server struct {
	bookingpb.UnimplementedBookingServiceServer
	bookings *booking.Service
}

// New returns a gRPC server backed by the given booking domain service.
func New(b *booking.Service) *Server {
	return &Server{bookings: b}
}

// CreateBooking records a reservation for the authenticated caller.
func (s *Server) CreateBooking(ctx context.Context, req *bookingpb.CreateBookingRequest) (*bookingpb.Booking, error) {
	guestID := metadataValue(ctx, "x-user-id")
	if guestID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing verified identity")
	}

	b, err := s.bookings.CreateBooking(ctx, booking.CreateBookingParams{
		PropertyID:   req.GetPropertyId(),
		GuestID:      guestID,
		CheckInDate:  req.GetCheckInDate(),
		CheckOutDate: req.GetCheckOutDate(),
		TotalGuests:  req.GetTotalGuests(),
		NightlyPrice: req.GetNightlyPrice(),
	})
	if err != nil {
		return nil, mapErr(err)
	}
	return pbBooking(b), nil
}

// GetBooking returns one of the caller's bookings.
func (s *Server) GetBooking(ctx context.Context, req *bookingpb.GetBookingRequest) (*bookingpb.Booking, error) {
	guestID := metadataValue(ctx, "x-user-id")
	if guestID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing verified identity")
	}

	b, err := s.bookings.GetBooking(ctx, req.GetId(), guestID)
	if err != nil {
		return nil, mapErr(err)
	}
	return pbBooking(b), nil
}

// ListUserBookings lists the caller's bookings.
func (s *Server) ListUserBookings(ctx context.Context, _ *bookingpb.ListUserBookingsRequest) (*bookingpb.ListUserBookingsResponse, error) {
	guestID := metadataValue(ctx, "x-user-id")
	if guestID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing verified identity")
	}

	list, err := s.bookings.ListUserBookings(ctx, guestID)
	if err != nil {
		return nil, mapErr(err)
	}

	res := &bookingpb.ListUserBookingsResponse{
		Bookings: make([]*bookingpb.Booking, 0, len(list)),
	}
	for _, b := range list {
		res.Bookings = append(res.Bookings, pbBooking(b))
	}
	return res, nil
}

// CancelBooking cancels one of the caller's bookings.
func (s *Server) CancelBooking(ctx context.Context, req *bookingpb.CancelBookingRequest) (*bookingpb.Booking, error) {
	guestID := metadataValue(ctx, "x-user-id")
	if guestID == "" {
		return nil, status.Error(codes.Unauthenticated, "missing verified identity")
	}

	b, err := s.bookings.CancelBooking(ctx, req.GetId(), guestID)
	if err != nil {
		return nil, mapErr(err)
	}
	return pbBooking(b), nil
}

// CheckAvailability reports whether a property is free for a range. It is public
// and requires no identity.
func (s *Server) CheckAvailability(ctx context.Context, req *bookingpb.CheckAvailabilityRequest) (*bookingpb.CheckAvailabilityResponse, error) {
	available, err := s.bookings.CheckAvailability(ctx, req.GetPropertyId(), req.GetCheckInDate(), req.GetCheckOutDate())
	if err != nil {
		return nil, mapErr(err)
	}
	return &bookingpb.CheckAvailabilityResponse{
		PropertyId:   req.GetPropertyId(),
		CheckInDate:  req.GetCheckInDate(),
		CheckOutDate: req.GetCheckOutDate(),
		Available:    available,
	}, nil
}

// pbBooking maps a domain Booking to the proto message.
func pbBooking(b booking.Booking) *bookingpb.Booking {
	return &bookingpb.Booking{
		Id:           b.ID,
		PropertyId:   b.PropertyID,
		GuestId:      b.GuestID,
		CheckInDate:  b.CheckInDate.Format(dateLayout),
		CheckOutDate: b.CheckOutDate.Format(dateLayout),
		TotalGuests:  b.TotalGuests,
		NightlyPrice: b.NightlyPrice,
		TotalPrice:   b.TotalPrice,
		Status:       b.Status,
		CreatedAt:    b.CreatedAt.Format(time.RFC3339),
		UpdatedAt:    b.UpdatedAt.Format(time.RFC3339),
	}
}

// metadataValue returns the first value for key in the incoming gRPC metadata.
func metadataValue(ctx context.Context, key string) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if vals := md.Get(key); len(vals) > 0 {
		return vals[0]
	}
	return ""
}

// mapErr translates domain errors into gRPC status errors. Unknown errors are
// reported as Internal with a generic message so implementation details do not
// leak to clients.
func mapErr(err error) error {
	switch {
	case errors.Is(err, booking.ErrDatesUnavailable):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, booking.ErrInvalidDates),
		errors.Is(err, booking.ErrInvalidProperty),
		errors.Is(err, booking.ErrInvalidGuest),
		errors.Is(err, booking.ErrInvalidGuestCount),
		errors.Is(err, booking.ErrInvalidPrice):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, booking.ErrBookingNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, booking.ErrUnauthorized):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, booking.ErrBookingNotActive):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, cache.ErrLockNotAcquired):
		return status.Error(codes.Aborted, "the property is busy, please retry")
	default:
		// Log the real cause server-side; return a generic message so
		// implementation details do not leak to clients.
		log.Printf("grpcserver: unhandled error, returning Internal: %v", err)
		return status.Error(codes.Internal, "internal error")
	}
}
