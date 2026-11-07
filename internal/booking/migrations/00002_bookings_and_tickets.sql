-- Booking service schema (booking_db, schema public). DDL copied verbatim from
-- the PDF p.17-21 via docs/04-data/postgres-schema.md (whitespace-identical,
-- verified). Do not weaken any constraint (CLAUDE.md invariant 4).
-- Part 2: bookings, tickets, transfers (written from phase 6/7; created now per P3-1).

-- +goose Up
CREATE TABLE bookings (                         -- makes ConfirmHold idempotent
  booking_id   UUID PRIMARY KEY,
  hold_id      UUID NOT NULL UNIQUE REFERENCES holds,
  order_id     UUID NOT NULL UNIQUE,
  user_id      UUID NOT NULL,
  confirmed_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE tickets (
  ticket_id      UUID PRIMARY KEY,
  booking_id     UUID NOT NULL REFERENCES bookings,
  event_id       UUID NOT NULL,
  seat_id        TEXT,                          -- NULL for GA
  section        TEXT NOT NULL,
  owner_user_id  UUID NOT NULL,
  status         TEXT NOT NULL CHECK (status IN ('VALID','CANCELLED','CHECKED_IN')),
  qr_secret_enc  BYTEA NOT NULL,
  transfer_count SMALLINT NOT NULL DEFAULT 0 CHECK (transfer_count BETWEEN 0 AND 1),
  checked_in_at  TIMESTAMPTZ,
  CHECK ((status = 'CHECKED_IN') = (checked_in_at IS NOT NULL)),
  FOREIGN KEY (event_id, seat_id) REFERENCES seats
);
CREATE UNIQUE INDEX one_live_ticket_per_seat ON tickets (event_id, seat_id)
  WHERE seat_id IS NOT NULL AND status <> 'CANCELLED';

CREATE TABLE ticket_transfers (
  transfer_id UUID PRIMARY KEY,
  ticket_id   UUID NOT NULL UNIQUE REFERENCES tickets,  -- at most one transfer per ticket
  from_user   UUID NOT NULL,
  to_user     UUID NOT NULL CHECK (to_user <> from_user),
  created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- +goose Down
DROP TABLE ticket_transfers;
DROP TABLE tickets;
DROP TABLE bookings;
