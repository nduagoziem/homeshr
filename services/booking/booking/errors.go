package booking

import "errors"

var (
	// ErrDatesUnavailable means a CONFIRMED booking already overlaps the
	// requested range for that property.
	ErrDatesUnavailable = errors.New("requested dates are not available")
	// ErrInvalidDates means the check-in/check-out pair is malformed, in the
	// past, or not a positive-length stay.
	ErrInvalidDates = errors.New("invalid booking dates")
	// ErrInvalidProperty means the property id is not a valid UUID.
	ErrInvalidProperty = errors.New("invalid property id")
	// ErrInvalidGuest means the guest id (from the verified JWT) is missing or
	// not a valid UUID.
	ErrInvalidGuest = errors.New("invalid guest id")
	// ErrInvalidGuestCount means total_guests is below one.
	ErrInvalidGuestCount = errors.New("total guests must be at least 1")
	// ErrInvalidPrice means the nightly price is negative.
	ErrInvalidPrice = errors.New("nightly price must not be negative")
	// ErrBookingNotFound means no booking exists with the given id.
	ErrBookingNotFound = errors.New("booking not found")
	// ErrUnauthorized means the caller is not the guest who owns the booking.
	ErrUnauthorized = errors.New("not authorized to access this booking")
	// ErrBookingNotActive means the booking is not in a cancellable state
	// (e.g. it was already cancelled).
	ErrBookingIsCancelled = errors.New("booking is already cancelled")
	// ErrQueriesRequired means the service was constructed without a db handle.
	ErrQueriesRequired = errors.New("booking service requires db queries")
	// ErrLockRequired means the service was constructed without a Redis lock.
	ErrLockRequired = errors.New("booking service requires a redis locker")
)
