# PostgreSQL Schema

*Source: PDF p.17–22 (DDL transcribed verbatim; only line wrapping from the PDF was repaired), p.22 (invariants, ownership).*

> *The `seats` table is the source of truth for inventory; a seat's status changes only through conditional updates, and a unique constraint on tickets is the final guard.* (p.17)

**One database per service.** Locally these are separate databases on one PostgreSQL 17 instance. **No service reads another's tables** (p.17). Migrations use **goose**, and queries use **pgx + sqlc**, with hand-written SQL on the hot paths ([AMB-36](../10-planning/ambiguities.md#amb-36)).

Schemas for **Identity** (`users`, `credentials`, `oauth_accounts`, `signing_keys`) and **Catalog** (`events`, `venues`, `seat_layouts`) are **named but not specified** in the PDF. They are not reproduced here ([AMB-13](../10-planning/ambiguities.md#amb-13)).

---

## booking_db

```sql
-- booking_db ---------------------------------------------------------
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
```

### booking_db constraint notes

| Constraint | What it guarantees |
|---|---|
| `seats` PK `(event_id, seat_id)` | A seat exists once per event |
| `CHECK ((status='AVAILABLE') = (hold_id IS NULL))` | An available seat has no hold; held **and booked** seats keep their `hold_id` |
| `CHECK ((status='HELD') = (hold_expires_at IS NOT NULL))` | Only held seats carry an expiry mirror; booking a seat must clear it |
| `fillfactor = 80` | Room for HOT updates on frequent status changes |
| `seats_held_expiry` (partial) | Fast scan of held seats by expiry |
| `seats_free_by_row` (partial) | Best-available ordering by row/seat over free seats only |
| `ga_buckets.remaining` `CHECK (>= 0 AND <= capacity)` | **A negative GA count is impossible** |
| `holds.order_id UNIQUE` | A hold belongs to at most one order |
| `holds_expiring` (partial) | Expiry worker scan |
| `hold_ga_allocations` PK + `released_at` | Exact per-bucket quantities; restore **once** |
| `bookings.hold_id UNIQUE`, `bookings.order_id UNIQUE` | **`ConfirmHold` idempotent by order**; one booking per hold |
| `one_live_ticket_per_seat` (partial unique) | **At most one non-cancelled ticket per seat.** This is the final double-booking guard; cancelling frees the seat for resale |
| `tickets.transfer_count BETWEEN 0 AND 1` + `ticket_transfers.ticket_id UNIQUE` | **Max one transfer per ticket** (enforced twice) |
| `CHECK ((status='CHECKED_IN') = (checked_in_at IS NOT NULL))` | Check-in timestamp consistent with status |
| `to_user <> from_user` | No self-transfer |

---

## checkout_db

```sql
-- checkout_db --------------------------------------------------------
CREATE TABLE orders (
  order_id       UUID PRIMARY KEY,
  hold_id        UUID NOT NULL UNIQUE,           -- one order per hold
  user_id        UUID NOT NULL,
  amount_paise   BIGINT NOT NULL CHECK (amount_paise > 0),
  refunded_paise BIGINT NOT NULL DEFAULT 0 CHECK (refunded_paise BETWEEN 0 AND amount_paise),
  status         TEXT NOT NULL CHECK (status IN ('PENDING_PAYMENT','PAID_PENDING_CONFIRMATION','CONFIRMED',
                   'REFUND_PENDING','PARTIALLY_REFUNDED','REFUNDED','FAILED')),
  version        INT NOT NULL DEFAULT 0,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE payments (
  payment_id   UUID PRIMARY KEY,
  order_id     UUID NOT NULL UNIQUE REFERENCES orders,   -- one capture per order
  provider_ref TEXT UNIQUE,
  status       TEXT NOT NULL CHECK (status IN ('CREATED','SUCCEEDED','FAILED')),
  amount_paise BIGINT NOT NULL CHECK (amount_paise > 0)
);

CREATE TABLE refunds (
  refund_id       UUID PRIMARY KEY,
  order_id        UUID NOT NULL REFERENCES orders,
  amount_paise    BIGINT NOT NULL CHECK (amount_paise >= 0),
  reason          TEXT NOT NULL CHECK (reason IN ('CANCELLED','CONFIRM_REJECTED','EVENT_CANCELLED')),
  status          TEXT NOT NULL CHECK (status IN ('REQUESTED','TICKETS_VOIDED','SENT','COMPLETED','FAILED')),
  provider_ref    TEXT UNIQUE,
  idempotency_key TEXT NOT NULL UNIQUE
);

CREATE TABLE refund_items (                     -- a ticket is refunded at most once
  refund_id    UUID NOT NULL REFERENCES refunds,
  ticket_id    UUID NOT NULL PRIMARY KEY,
  amount_paise BIGINT NOT NULL CHECK (amount_paise >= 0)
);
CREATE UNIQUE INDEX one_full_refund_per_rejected_order ON refunds (order_id) WHERE reason = 'CONFIRM_REJECTED';

CREATE TABLE sagas (
  saga_id  UUID PRIMARY KEY,
  kind     TEXT NOT NULL CHECK (kind IN ('CHECKOUT','REFUND')),
  order_id UUID NOT NULL REFERENCES orders,
  state    TEXT NOT NULL,
  version  INT  NOT NULL DEFAULT 0,             -- optimistic concurrency between workers
  UNIQUE (kind, order_id, saga_id)
);

CREATE TABLE saga_steps (
  saga_id         UUID NOT NULL REFERENCES sagas,
  step            TEXT NOT NULL,               -- e.g. 'CONFIRM_HOLD', 'SEND_REFUND'
  status          TEXT NOT NULL CHECK (status IN ('PENDING','RUNNING','SUCCEEDED','UNKNOWN','FAILED')),
  attempts        INT  NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ,
  last_error      TEXT,
  PRIMARY KEY (saga_id, step)                  -- unique step identity
);
```

### checkout_db constraint notes

| Constraint | What it guarantees |
|---|---|
| `orders.hold_id UNIQUE` | One order per hold |
| `refunded_paise BETWEEN 0 AND amount_paise` | **Refunds can never exceed the amount paid** |
| `orders.version` | Conditional status transitions (see the order state machine) |
| `payments.order_id UNIQUE` | **One capture per order**: no double charge |
| `payments.provider_ref UNIQUE` | Duplicate webhooks deduped |
| `refunds.provider_ref UNIQUE`, `refunds.idempotency_key UNIQUE` | Duplicate refund webhooks/requests deduped |
| `refund_items.ticket_id` PK | **A ticket is refunded at most once** |
| `one_full_refund_per_rejected_order` | At most one `CONFIRM_REJECTED` refund per order |
| `sagas.version` | Optimistic concurrency between saga workers |
| `saga_steps` PK `(saga_id, step)` | Unique step identity; durable progress |

> `sagas.state` is free `TEXT`. Its values are not enumerated ([AMB-25](../10-planning/ambiguities.md#amb-25)). `UNIQUE (kind, order_id, saga_id)` includes the PK, so it does **not** limit an order to one saga of each kind. Whether that is intended is part of [AMB-25](../10-planning/ambiguities.md#amb-25).

---

## Shared tables (in every service that writes state)

```sql
-- in every service that writes state --------------------------------
CREATE TABLE idempotency_keys (
  scope         TEXT  NOT NULL,                -- operation, e.g. 'POST /holds'
  user_id       UUID  NOT NULL,
  key           TEXT  NOT NULL,
  request_hash  BYTEA NOT NULL,                -- same key + different body → 422
  status        TEXT  NOT NULL CHECK (status IN ('IN_PROGRESS','COMPLETED')),
  response_code INT,
  response_body JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at    TIMESTAMPTZ NOT NULL,
  PRIMARY KEY (scope, user_id, key)
);

CREATE TABLE outbox (
  id              UUID PRIMARY KEY,
  topic           TEXT  NOT NULL,
  key             TEXT  NOT NULL,
  payload         BYTEA NOT NULL,
  status          TEXT  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','IN_FLIGHT','SENT','DEAD')),
  attempts        INT   NOT NULL DEFAULT 0,
  next_attempt_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  locked_by       TEXT,
  locked_until    TIMESTAMPTZ,
  created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
  sent_at         TIMESTAMPTZ
);
CREATE INDEX outbox_ready ON outbox (next_attempt_at) WHERE status IN ('PENDING','IN_FLIGHT');

CREATE TABLE processed_events (event_id UUID PRIMARY KEY, processed_at TIMESTAMPTZ NOT NULL DEFAULT now());
```

**Notes**
- The idempotency key's retention (`expires_at`) duration is unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)).
- The outbox relay claims rows via `locked_by` / `locked_until` (lease-style). The retry backoff and the threshold for `DEAD` are unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)).
- Which services "write state" is not listed explicitly. Booking and Checkout clearly do; Identity and Catalog write Postgres but publish no topics ([AMB-13](../10-planning/ambiguities.md#amb-13)).

---

## Invariants (p.21–22)

1. **`holds.expires_at` is the one canonical expiry.** `seats.hold_expires_at` is a mirror written **only in the same transaction**: on create, extend at checkout, release and expire.
2. **`processed_events` stops a consumer applying an event twice.**
3. **Business-level unique keys** (`bookings.order_id`, `payments.order_id`, `refund_items.ticket_id`, the `hold_ga_allocations` primary key) **stop duplicate effects even if dedup is bypassed.**

## Cross-service data movement

- Catalog's static `seat_layouts` is **copied into Booking's `seats` when a sale is published** (p.22).
- Everything else crosses service boundaries via gRPC or Kafka. For example, `orders` has no `event_id` and relies on Booking/Catalog for event context ([AMB-23](../10-planning/ambiguities.md#amb-23)).
