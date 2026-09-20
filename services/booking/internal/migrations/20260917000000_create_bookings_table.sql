-- +goose Up
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE bookings (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  property_id UUID NOT NULL,
  guest_id UUID NOT NULL,
  check_in_date DATE NOT NULL,
  check_out_date DATE NOT NULL,
  total_guests INT NOT NULL DEFAULT 1,
  -- Prices are stored in integer minor units (e.g. cents) to avoid rounding.
  nightly_price BIGINT NOT NULL,
  total_price BIGINT NOT NULL,
  status VARCHAR(32) NOT NULL DEFAULT 'CONFIRMED',
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Availability/overlap checks always filter by property; listings filter by guest.
CREATE INDEX idx_bookings_property_id ON bookings(property_id);
CREATE INDEX idx_bookings_guest_id ON bookings(guest_id);

-- +goose Down
DROP INDEX IF EXISTS idx_bookings_guest_id;
DROP INDEX IF EXISTS idx_bookings_property_id;
DROP TABLE IF EXISTS bookings;
