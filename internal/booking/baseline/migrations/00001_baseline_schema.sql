-- UNSAFE NAIVE BASELINE (ADR-034, AMB-38). Phase 2 testing and benchmarking only.
--
-- These tables live in their own schema, "baseline", inside booking_db. They are
-- NOT the real Booking schema (docs/04-data/postgres-schema.md), which arrives
-- in phase 3 and is never weakened. They are deliberately unconstrained against
-- double booking:
--   * booking_seats has NO uniqueness on seat_id, so two bookings may hold the
--     same seat (the real schema's one_live_ticket_per_seat would stop that);
--   * seats.status is changed without a conditional update by the naive code.
-- Ordinary integrity (primary keys, foreign keys, non-negative money in paise)
-- stays, because it does not prevent the race and a plausible naive
-- implementation would have it.

-- +goose Up
CREATE SCHEMA baseline;

CREATE TABLE baseline.events (
  id   UUID PRIMARY KEY,
  name TEXT NOT NULL
);

CREATE TABLE baseline.seats (
  id          UUID   PRIMARY KEY,
  event_id    UUID   NOT NULL REFERENCES baseline.events (id),
  section     TEXT   NOT NULL,
  row_label   TEXT   NOT NULL,
  seat_number INT    NOT NULL,
  price_paise BIGINT NOT NULL CHECK (price_paise >= 0),
  status      TEXT   NOT NULL DEFAULT 'AVAILABLE' CHECK (status IN ('AVAILABLE', 'BOOKED')),
  booked_at   TIMESTAMPTZ
);
CREATE INDEX seats_by_event ON baseline.seats (event_id);

CREATE TABLE baseline.bookings (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id     UUID        NOT NULL REFERENCES baseline.events (id),
  buyer_id     TEXT        NOT NULL,
  amount_paise BIGINT      NOT NULL CHECK (amount_paise >= 0),
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One row per seat in a booking. The primary key only stops the same seat
-- appearing twice in ONE booking. Nothing stops two bookings sharing a seat.
CREATE TABLE baseline.booking_seats (
  booking_id UUID NOT NULL REFERENCES baseline.bookings (id) ON DELETE CASCADE,
  seat_id    UUID NOT NULL REFERENCES baseline.seats (id),
  PRIMARY KEY (booking_id, seat_id)
);
CREATE INDEX booking_seats_by_seat ON baseline.booking_seats (seat_id);

-- +goose Down
DROP SCHEMA baseline CASCADE;
