# Booking Service

*Source: PDF p.5–7 (FR-6…FR-9, FR-11, FR-16, FR-17, FR-23), p.12 (data flow), p.15 (responsibilities), p.17–21 (booking_db DDL), p.21–22 (invariants), p.23–24 (Redis, Kafka), p.25–27 (REST, gRPC), p.27–31 (flows 2, 3, 4, 5, 7), p.31–33 (guarantees, failures).*

## Purpose

Booking is the **correctness core for inventory and ticket ownership** (p.15). It is the one service that **must be strictly correct**, and it **scales on the admitted rate**, never on the crowd.

## Responsibilities

- **Seat and GA inventory** (`seats`, `ga_buckets`)
- **Holds**: specific seats, best-available, GA; 1–4 seats; **10 minutes**
- **Best-available picker**: `SKIP LOCKED` candidate selection, adjacency in memory
- **Expiry**: worker every 2 s plus inline reclaim
- **Payment hand-off**: `StartPayment` extends the hold for the payment window
- **Confirmation**: `ConfirmHold`, idempotent by `order_id`
- **Tickets and rotating QR secrets**
- **Cancellation of tickets**: `CancelTickets`, called by the refund saga
- **Transfers**, max 1 per ticket
- **Gate check-in**, exactly once

## Owned data

`booking_db`: `seats`, `ga_buckets`, `holds`, `hold_seats`, `hold_ga_allocations`, `bookings`, `tickets`, `ticket_transfers`, `idempotency_keys`, `outbox`, `processed_events`. Redis: `seatbits:{event}:{section}` (bitmap pre-filter).

Full DDL and constraints: [postgres-schema.md](../04-data/postgres-schema.md#booking_db). State machines: [state-machines.md](../04-data/state-machines.md).

## Internal gRPC — `booking.v1`

| RPC | Caller | Behaviour |
|---|---|---|
| `HoldSeats` | Gateway | All-or-nothing conditional update of chosen seats |
| `HoldBestAvailable` | Gateway | `SKIP LOCKED LIMIT 40` candidates → first run of N adjacent → conditional update |
| `HoldGA` | Gateway | Multi-bucket allocation with `SKIP LOCKED`, then a fallback ordered `FOR UPDATE` |
| `ReleaseHold` | Gateway | Early release (FR-23) |
| `GetHold` | Checkout | Read a hold |
| `StartPayment(hold_id, order_id)` | Checkout | Hold → `PAYMENT_PENDING`; extend canonical expiry and seat mirrors by **at most 10 more minutes** |
| `ConfirmHold(hold_id, order_id)` | Checkout saga | **Idempotent by order**: returns the existing booking if present; `CONFIRMED` / definitive `REJECTED` |
| `GetBookingByOrder(order_id)` | Checkout reconciler | Resolve `UNKNOWN` confirmations |
| `CancelTickets` | Checkout refund saga | Void tickets, return inventory, revoke QR secrets, outbox `InventoryReturned`: **one transaction** |
| `TransferTicket` | Gateway | Ownership change + QR rotation + transfer row: **one transaction** |
| `CheckIn` | Gateway (`/v1/scan`) | `UPDATE … WHERE status = 'VALID'`; first scan wins |
| `WatchSeats` (server-stream) | ? | Live seat changes. Consumer unclear ([AMB-08](../10-planning/ambiguities.md#amb-08)) |

## Public API surfaced through the Gateway

| Route | Auth | Responses |
|---|---|---|
| `POST /v1/events/{eventID}/holds` with `{seat_ids}` \| `{best_available:{section or tier, quantity}}` \| `{ga:{section, quantity}}` | access token | `201 {hold_id, seats, expires_at, amount}`, `409`, `422` |
| `DELETE /v1/holds/{holdID}` | access token | `204` |
| `GET /v1/tickets/{ticketID}/qr` | JWT (owner) | `200` current rotating QR payload |
| `POST /v1/tickets/{ticketID}/transfers` `{email}` | JWT (owner) | `201`, `409` limit reached |
| `POST /v1/scan` | staff JWT | `200 {valid, seat}`, `409` already used |

## Core algorithms (summary; full detail in flows)

### Chosen seats ([flow 2a](../06-flows/02-hold-seats.md))
```sql
UPDATE seats SET status='HELD', hold_id=$h, hold_expires_at=$exp
 WHERE event_id = $1 AND seat_id = ANY($2)
   AND (status = 'AVAILABLE' OR (status = 'HELD' AND hold_expires_at < now()));
-- rows affected ≠ requested → ROLLBACK (all or nothing)
```
The `SET` list is **derived** from the schema's `CHECK` constraints. The PDF gives only the `WHERE` clause (p.31). A seat reclaimed from an expired hold also marks that old hold `EXPIRED` in the same transaction.

### Best-available ([flow 2b](../06-flows/02-hold-seats.md))
`SELECT … WHERE status='AVAILABLE' ORDER BY row_label, seat_number FOR UPDATE SKIP LOCKED LIMIT 40` → pick the first run of N adjacent seats in memory → the same conditional update.

### GA across buckets ([flow 2c](../06-flows/02-hold-seats.md))
Lock buckets with stock: `WHERE remaining > 0 ORDER BY (bucket - r + 32) % 32 FOR UPDATE SKIP LOCKED` (r random). Take greedily until N is covered and write one `hold_ga_allocations` row per bucket. If short, **retry once** with plain `FOR UPDATE` in **ascending bucket order** (waits, cannot deadlock). Still short → `409`.

### Expiry ([flow 4](../06-flows/04-hold-expiry.md))
Every **2 s**, claim expired `ACTIVE`/`PAYMENT_PENDING` holds in batches of **1,000** with `FOR UPDATE SKIP LOCKED`. One transaction per batch.

### Rotating QR ([flow 7](../06-flows/07-transfer-and-gate-scan.md))
Code = TOTP-style HMAC(ticket secret, current **30-second** window). Scan accepts the current and previous window. Secrets are stored encrypted (`qr_secret_enc`) and **never sent in plain form**.

## Events produced (via outbox)

| Topic | Key | Events |
|---|---|---|
| `booking.holds` | `hold_id` | HoldCreated, HoldExtended, HoldReleased, HoldExpired, HoldConfirmed |
| `booking.inventory` | `event_id:section` (`SoldOut` keyed by `event_id`) | InventoryChanged, InventoryReturned, SoldOut |
| `booking.tickets` | `ticket_id` | TicketsIssued, TicketCancelled, TicketTransferred, TicketCheckedIn |

A hold spanning two sections emits its lifecycle **once** on `booking.holds` and **one `InventoryChanged` per section** on `booking.inventory` (p.24).

## Performance targets

Hold **p99 < 300 ms**; scan **p99 < 150 ms**; hold conflict rate **< 5 %** with best-available; **≤ 4 Postgres round trips per seat sold**; capacity **1,000 holds/s**.

## Failure behaviour

| Failure | Behaviour |
|---|---|
| Booking crashes mid-request | Transaction committed or not; a retry with the same idempotency key is safe |
| Expiry worker stops | The hold path reclaims expired seats inline; inventory is never stuck |
| Redis down | Holds skip the bitmap pre-filter |
| `ConfirmHold` response lost | Idempotent retry returns the existing booking; reconciler uses `GetBookingByOrder` |

## Naive baseline (phase 2 only)

Phase 2 adds a deliberately unsafe **check-then-book** path to Booking for testing and benchmarking only ([ADR-034](../../ARCHITECTURE_DECISIONS.md#adr-034)). It reads seat status without a lock, then books without a status predicate or rows-affected check, under `READ COMMITTED`. It uses its own tables, never the schema above, and is isolated and off by default. It is **not** the Booking design described on this page. Plan: [phase-2-naive-baseline](../10-planning/phase-2-naive-baseline.md). Open: [AMB-38](../10-planning/ambiguities.md#amb-38) … [AMB-43](../10-planning/ambiguities.md#amb-43).

## Open items

[AMB-06](../10-planning/ambiguities.md#amb-06) · [AMB-07](../10-planning/ambiguities.md#amb-07) · [AMB-08](../10-planning/ambiguities.md#amb-08) · [AMB-09](../10-planning/ambiguities.md#amb-09) · [AMB-10](../10-planning/ambiguities.md#amb-10) · [AMB-19](../10-planning/ambiguities.md#amb-19) · [AMB-26](../10-planning/ambiguities.md#amb-26) · [AMB-27](../10-planning/ambiguities.md#amb-27) · [AMB-30](../10-planning/ambiguities.md#amb-30)
