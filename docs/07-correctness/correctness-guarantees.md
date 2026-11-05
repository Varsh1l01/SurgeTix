# Correctness Guarantees

*Source: PDF p.31–32 ("Correctness guarantees and failure handling", "How a double-booking is made impossible"), p.21–22 (invariants), p.7 (consistency NFR).*

> *Four mechanisms carry every guarantee: atomic conditional updates, unique constraints, idempotency keys, and the outbox. Redis and Kafka can fail without ever corrupting seat or money state.* (p.31)

## The four mechanisms

| Mechanism | Where it is used |
|---|---|
| **Atomic conditional updates** | Every seat, GA bucket, hold, order, refund, ticket and saga transition (`UPDATE … WHERE status = $from`, check rows affected) |
| **Unique constraints** (incl. partial) | `one_live_ticket_per_seat`, `bookings.order_id`, `payments.order_id`, `provider_ref`, `refund_items.ticket_id`, `ticket_transfers.ticket_id`, `one_full_refund_per_rejected_order`, `orders.hold_id`, `holds.order_id` |
| **Idempotency keys** | `idempotency_keys (scope, user_id, key)` on every write, plus `processed_events` for consumers |
| **Outbox** | Every event is written in the business transaction, so a state change and its event commit or fail together |

## The seven guarantees (p.31–32)

### G-1. A seat hold is all or nothing
A hold is one
```sql
UPDATE seats ... WHERE event_id = $1 AND seat_id = ANY($2)
  AND (status = 'AVAILABLE' OR (status = 'HELD' AND hold_expires_at < now()))
```
inside a transaction. **If rows affected ≠ seats requested, the transaction rolls back.**

### G-2. Competing holds serialise
Postgres row locks serialise competing updates. **The loser re-checks the `WHERE` clause against the new row and matches nothing.** Best-available uses `SKIP LOCKED` **only to choose candidates**. The conditional update still decides.

### G-3. GA is never oversold, and is restored exactly
GA decrements each bucket only `WHERE remaining >= quantity`. **`CHECK (remaining >= 0)` makes a negative count impossible**, and every unit taken is recorded per bucket, so release restores **exactly what was taken, once** (`released_at`).

### G-4. Confirmation is idempotent and ownership-checked
Confirmation is **idempotent by `order_id`** and **only succeeds while the hold belongs to that order**. An expired and re-sold hold returns a **definitive REJECTED**.

### G-5. At most one live ticket per seat
A **partial unique index** allows at most one live ticket per seat. **Cancelling a ticket frees the seat for resale without breaking the rule.**

### G-6. Money
- Payment success is **persisted before confirmation is attempted**.
- A refund is started **only on a definitive rejection or a cancellation, never on a timeout**.
- **Each ticket can be refunded once** (`refund_items`).
- **Refunds can't exceed payments** (`refunded_paise ≤ amount_paise`).

### G-7. Leadership
Every admission-counter write **checks the current lease epoch inside the same Lua script**, so a stale controller cannot write.

## Supporting invariants (p.21–22)

- `holds.expires_at` is the **one canonical expiry**. `seats.hold_expires_at` is a mirror written **only in the same transaction**.
- `processed_events` stops a consumer applying an event twice.
- Business-level unique keys stop duplicate effects **even if dedup is bypassed**.

## Guarantee → evidence

| Guarantee | Tests | Runtime evidence |
|---|---|---|
| G-1, G-2, G-5 | **T1** (5,000 buyers on 500 seats × 100 runs), T3, T8 | `double_booked_seats` = 0 after every run |
| G-3 | **T7** | `oversold_ga` = 0 |
| G-4, G-6 | **T12**, T5, T9 | Nightly audit: 0 wrong refunds, 0 paid-without-ticket-or-refund |
| G-7 | **T13** | 0 counter writes from the old epoch |

> *These are guarantees of the data model and the code paths above; the test suite (T1, T7, T12, T13) checks them repeatedly, but tests are evidence, not a proof of every possible execution.* (p.32)

## Rules for every future change

*Derived directly from the guarantees. Recorded in [`/CLAUDE.md`](../../CLAUDE.md).*

1. Never change seat/order/ticket/refund/hold state without a `WHERE status = <expected>` guard and a rows-affected check.
2. Never let Redis, the seat-map cache or Kafka **grant** inventory or money. They may only reject, inform or trigger.
3. Never issue a refund on a timeout, `UNAVAILABLE` or unknown error.
4. Never publish to Kafka outside the outbox.
5. Never remove or weaken a `CHECK`, `UNIQUE` or partial unique index listed above without an approved ADR.
6. Every new state-changing endpoint takes an `Idempotency-Key`.
