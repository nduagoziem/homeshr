-- name: CreateBooking :one
INSERT INTO bookings (
    property_id, guest_id, check_in_date, check_out_date,
    total_guests, nightly_price, total_price
) VALUES (
    @property_id, @guest_id, @check_in_date, @check_out_date,
    @total_guests, @nightly_price, @total_price
)
RETURNING *;
-- guest_id is the user_id

-- name: GetBookingByID :one
SELECT * FROM bookings WHERE id = @id;

-- name: ListBookingsByGuest :many
SELECT * FROM bookings WHERE guest_id = @guest_id ORDER BY created_at DESC;

-- name: CancelBooking :one
UPDATE bookings
SET status = 'CANCELLED', updated_at = NOW()
WHERE id = @id
RETURNING *;

-- CountOverlappingBookings counts CONFIRMED reservations on a property whose
-- date range collides with [check_in_date, check_out_date). Two half-open date
-- ranges overlap iff each starts before the other ends; the checkout day is
-- free, so back-to-back stays (one guest checks out the day the next checks in)
-- do not conflict.
-- name: CountOverlappingBookings :one
SELECT COUNT(*) FROM bookings
WHERE property_id = @property_id
  AND status = 'CONFIRMED'
  AND check_in_date < @check_out_date
  AND check_out_date > @check_in_date;
