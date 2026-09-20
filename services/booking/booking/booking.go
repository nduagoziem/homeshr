// Package booking holds the reservation domain logic: validating stay dates,
// pricing them, serializing concurrent attempts on a property with a Redis
// lock, rejecting overlaps, and recording confirmed bookings. It maps to and
// from the sqlc db types and exposes plain-Go domain structs to the transport
// layer.
package booking

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/nduagoziem/homeshr/services/booking/internal/cache"
	"github.com/nduagoziem/homeshr/services/booking/internal/db"
)

const (
	// dateLayout is the ISO-8601 calendar-day format used on the wire.
	dateLayout = "2006-01-02"
	// statusConfirmed is the status of an active reservation.
	statusConfirmed = "CONFIRMED"

	defaultLockTTL  = 10 * time.Second
	defaultLockWait = 3 * time.Second
)

// Booking is the domain view of a reservation: string ids, real dates/times,
// and integer minor-unit prices.
type Booking struct {
	ID           string
	PropertyID   string
	GuestID      string
	CheckInDate  time.Time
	CheckOutDate time.Time
	TotalGuests  int32
	NightlyPrice int64
	TotalPrice   int64
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Service provides booking operations backed by Postgres and Redis.
type Service struct {
	queries  *db.Queries
	locker   *cache.Cache
	lockTTL  time.Duration
	lockWait time.Duration
}

// ServiceConfig carries the dependencies and settings required by Service.
type ServiceConfig struct {
	Queries  *db.Queries
	Locker   *cache.Cache
	LockTTL  time.Duration // how long a held property lock survives before auto-expiry
	LockWait time.Duration // how long to wait to acquire a contended property lock
}

// NewService constructs a booking Service, applying defaults for unset TTLs.
func NewService(cfg ServiceConfig) *Service {
	ttl := cfg.LockTTL
	if ttl <= 0 {
		ttl = defaultLockTTL
	}
	wait := cfg.LockWait
	if wait <= 0 {
		wait = defaultLockWait
	}
	return &Service{
		queries:  cfg.Queries,
		locker:   cfg.Locker,
		lockTTL:  ttl,
		lockWait: wait,
	}
}

// CreateBookingParams is the domain input for a reservation request. Dates are
// "YYYY-MM-DD" strings; GuestID comes from the JWT-verified x-user-id header.
type CreateBookingParams struct {
	PropertyID   string
	GuestID      string
	CheckInDate  string
	CheckOutDate string
	TotalGuests  int32
	NightlyPrice int64
}

// CreateBooking validates and prices the stay, then—while holding a per-property
// lock so two concurrent requests cannot both pass the check—rejects any
// overlap and records a CONFIRMED booking.
func (s *Service) CreateBooking(ctx context.Context, p CreateBookingParams) (Booking, error) {
	if s.queries == nil {
		return Booking{}, ErrQueriesRequired
	}
	if s.locker == nil {
		return Booking{}, ErrLockRequired
	}

	propertyID, err := parseUUID(p.PropertyID)
	if err != nil {
		return Booking{}, ErrInvalidProperty
	}
	guestID, err := parseUUID(p.GuestID)
	if err != nil {
		return Booking{}, ErrInvalidGuest
	}

	checkIn, checkOut, err := parseStay(p.CheckInDate, p.CheckOutDate)
	if err != nil {
		return Booking{}, err
	}
	if p.TotalGuests < 1 {
		return Booking{}, ErrInvalidGuestCount
	}
	if p.NightlyPrice < 0 {
		return Booking{}, ErrInvalidPrice
	}

	nights := int64(checkOut.Sub(checkIn) / (24 * time.Hour))
	totalPrice := p.NightlyPrice * nights

	// Serialize booking attempts on this property. Without the lock, two
	// concurrent requests could both read zero overlaps and both insert.
	lock, err := s.locker.AcquireLock(ctx, cache.PropertyLockKey(p.PropertyID), s.lockTTL, s.lockWait)
	if err != nil {
		return Booking{}, err
	}
	// Release with a fresh context so an already-cancelled request context does
	// not skip freeing the lock.
	defer lock.Release(context.Background())

	count, err := s.queries.CountOverlappingBookings(ctx, db.CountOverlappingBookingsParams{
		PropertyID:   propertyID,
		CheckInDate:  pgtype.Date{Time: checkIn, Valid: true},
		CheckOutDate: pgtype.Date{Time: checkOut, Valid: true},
	})
	if err != nil {
		return Booking{}, err
	}
	if count > 0 {
		return Booking{}, ErrDatesUnavailable
	}

	row, err := s.queries.CreateBooking(ctx, db.CreateBookingParams{
		PropertyID:   propertyID,
		GuestID:      guestID,
		CheckInDate:  pgtype.Date{Time: checkIn, Valid: true},
		CheckOutDate: pgtype.Date{Time: checkOut, Valid: true},
		TotalGuests:  p.TotalGuests,
		NightlyPrice: p.NightlyPrice,
		TotalPrice:   totalPrice,
	})
	if err != nil {
		return Booking{}, err
	}

	return fromDB(row), nil
}

// GetBooking returns a booking, but only to the guest who owns it.
func (s *Service) GetBooking(ctx context.Context, id, guestID string) (Booking, error) {
	if s.queries == nil {
		return Booking{}, ErrQueriesRequired
	}

	bookingID, err := parseUUID(id)
	if err != nil {
		return Booking{}, ErrBookingNotFound
	}

	row, err := s.queries.GetBookingByID(ctx, bookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrBookingNotFound
	}
	if err != nil {
		return Booking{}, err
	}

	if err := assertOwner(row.GuestID, guestID); err != nil {
		return Booking{}, err
	}
	return fromDB(row), nil
}

// ListUserBookings returns every booking made by the calling guest, newest first.
func (s *Service) ListUserBookings(ctx context.Context, guestID string) ([]Booking, error) {
	if s.queries == nil {
		return nil, ErrQueriesRequired
	}

	gid, err := parseUUID(guestID)
	if err != nil {
		return nil, ErrInvalidGuest
	}

	rows, err := s.queries.ListBookingsByGuest(ctx, gid)
	if err != nil {
		return nil, err
	}

	bookings := make([]Booking, 0, len(rows))
	for _, r := range rows {
		bookings = append(bookings, fromDB(r))
	}
	return bookings, nil
}

// CancelBooking cancels a CONFIRMED booking owned by the caller, freeing its
// dates for future reservations.
func (s *Service) CancelBooking(ctx context.Context, id, guestID string) (Booking, error) {
	if s.queries == nil {
		return Booking{}, ErrQueriesRequired
	}

	bookingID, err := parseUUID(id)
	if err != nil {
		return Booking{}, ErrBookingNotFound
	}

	row, err := s.queries.GetBookingByID(ctx, bookingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrBookingNotFound
	}
	if err != nil {
		return Booking{}, err
	}

	if err := assertOwner(row.GuestID, guestID); err != nil {
		return Booking{}, err
	}
	if row.Status != statusConfirmed {
		return Booking{}, ErrBookingIsCancelled
	}

	updated, err := s.queries.CancelBooking(ctx, bookingID)
	if err != nil {
		return Booking{}, err
	}
	return fromDB(updated), nil
}

// CheckAvailability reports whether a property is free for the given range. It
// is a public, read-only lookup and takes no lock.
func (s *Service) CheckAvailability(ctx context.Context, propertyID, checkInStr, checkOutStr string) (bool, error) {
	if s.queries == nil {
		return false, ErrQueriesRequired
	}

	pid, err := parseUUID(propertyID)
	if err != nil {
		return false, ErrInvalidProperty
	}

	checkIn, checkOut, err := parseStay(checkInStr, checkOutStr)
	if err != nil {
		return false, err
	}

	count, err := s.queries.CountOverlappingBookings(ctx, db.CountOverlappingBookingsParams{
		PropertyID:   pid,
		CheckInDate:  pgtype.Date{Time: checkIn, Valid: true},
		CheckOutDate: pgtype.Date{Time: checkOut, Valid: true},
	})
	if err != nil {
		return false, err
	}
	return count == 0, nil
}

// parseStay parses and validates a check-in/check-out pair: both must be valid
// calendar days, the stay must be at least one night, and it must not start in
// the past (comparing days in UTC).
func parseStay(checkInStr, checkOutStr string) (time.Time, time.Time, error) {
	checkIn, err := time.Parse(dateLayout, checkInStr)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidDates
	}
	checkOut, err := time.Parse(dateLayout, checkOutStr)
	if err != nil {
		return time.Time{}, time.Time{}, ErrInvalidDates
	}

	if !checkOut.After(checkIn) {
		return time.Time{}, time.Time{}, ErrInvalidDates
	}

	today := time.Now().UTC().Truncate(24 * time.Hour)
	if checkIn.Before(today) {
		return time.Time{}, time.Time{}, ErrInvalidDates
	}
	return checkIn, checkOut, nil
}

// assertOwner returns ErrUnauthorized unless guestID is a valid UUID matching
// the booking's owner.
func assertOwner(owner pgtype.UUID, guestID string) error {
	gid, err := parseUUID(guestID)
	if err != nil {
		return ErrUnauthorized
	}
	if owner.Bytes != gid.Bytes {
		return ErrUnauthorized
	}
	return nil
}

func fromDB(b db.Booking) Booking {
	return Booking{
		ID:           uuidString(b.ID),
		PropertyID:   uuidString(b.PropertyID),
		GuestID:      uuidString(b.GuestID),
		CheckInDate:  b.CheckInDate.Time,
		CheckOutDate: b.CheckOutDate.Time,
		TotalGuests:  b.TotalGuests,
		NightlyPrice: b.NightlyPrice,
		TotalPrice:   b.TotalPrice,
		Status:       b.Status,
		CreatedAt:    b.CreatedAt.Time,
		UpdatedAt:    b.UpdatedAt.Time,
	}
}

func uuidString(id pgtype.UUID) string {
	if !id.Valid {
		return ""
	}
	return uuid.UUID(id.Bytes).String()
}

func parseUUID(s string) (pgtype.UUID, error) {
	id, err := uuid.Parse(s)
	if err != nil {
		return pgtype.UUID{}, err
	}
	return pgtype.UUID{Bytes: id, Valid: true}, nil
}
