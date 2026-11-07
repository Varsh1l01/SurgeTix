-- Booking service schema (booking_db, schema public). DDL copied verbatim from
-- the PDF p.17-21 via docs/04-data/postgres-schema.md (whitespace-identical,
-- verified). Do not weaken any constraint (CLAUDE.md invariant 4).
-- Part 1: inventory and holds.

-- +goose Up
CREATE TABLE seats (
  event_id        UUID   NOT NULL,
  seat_id         TEXT   NOT NULL,              -- e.g. 'A-12'
  section         TEXT   NOT NULL,
  row_label       TEXT   NOT NULL,
  seat_number     INT    NOT NULL,              -- adjacency for best-available
  price_paise     BIGINT NOT NULL CHECK (price_paise >= 0),
  status          TEXT   NOT NULL DEFAULT 'AVAILABLE'
                  CHECK (status IN ('AVAILABLE','HELD','BOOKED')),
  hold_id         UUID,
  hold_expires_at TIMESTAMPTZ,  -- mirror of holds.expires_at, written ONLY in the same transaction
  PRIMARY KEY (event_id, seat_id),
  CHECK ((status = 'AVAILABLE') = (hold_id IS NULL)),
  CHECK ((status = 'HELD') = (hold_expires_at IS NOT NULL))
) WITH (fillfactor = 80);                      -- room for HOT updates on status changes

CREATE INDEX seats_held_expiry ON seats (hold_expires_at) WHERE status = 'HELD';
CREATE INDEX seats_free_by_row ON seats (event_id, section, row_label, seat_number) WHERE status = 'AVAILABLE';

CREATE TABLE ga_buckets (
  event_id  UUID     NOT NULL,
  section   TEXT     NOT NULL,
  bucket    SMALLINT NOT NULL,                  -- 0..31
  capacity  INT      NOT NULL CHECK (capacity >= 0),
  remaining INT      NOT NULL CHECK (remaining >= 0 AND remaining <= capacity),
  PRIMARY KEY (event_id, section, bucket)
);

CREATE TABLE holds (
  hold_id    UUID PRIMARY KEY,
  event_id   UUID NOT NULL,
  user_id    UUID NOT NULL,
  kind       TEXT NOT NULL CHECK (kind IN ('SEATS','GA')),
  status     TEXT NOT NULL CHECK (status IN ('ACTIVE','PAYMENT_PENDING','CONVERTED','EXPIRED','RELEASED')),
  expires_at TIMESTAMPTZ NOT NULL,              -- CANONICAL hold expiry
  order_id   UUID UNIQUE,                       -- set when checkout starts
  created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX holds_expiring ON holds (expires_at) WHERE status IN ('ACTIVE','PAYMENT_PENDING');

CREATE TABLE hold_seats (                       -- normalized seats of a hold
  hold_id  UUID NOT NULL REFERENCES holds,
  event_id UUID NOT NULL,
  seat_id  TEXT NOT NULL,
  PRIMARY KEY (hold_id, seat_id),
  FOREIGN KEY (event_id, seat_id) REFERENCES seats
);

CREATE TABLE hold_ga_allocations (              -- exact quantity taken from each bucket
  hold_id     UUID     NOT NULL REFERENCES holds,
  event_id    UUID     NOT NULL,
  section     TEXT     NOT NULL,
  bucket      SMALLINT NOT NULL,
  quantity    INT      NOT NULL CHECK (quantity > 0),
  released_at TIMESTAMPTZ,                      -- set once when restored; prevents double release
  PRIMARY KEY (hold_id, section, bucket),
  FOREIGN KEY (event_id, section, bucket) REFERENCES ga_buckets
);

-- +goose Down
DROP TABLE hold_ga_allocations;
DROP TABLE hold_seats;
DROP TABLE holds;
DROP TABLE ga_buckets;
DROP TABLE seats;
