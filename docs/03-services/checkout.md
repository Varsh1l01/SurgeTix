# Checkout Service

*Source: PDF p.6–7 (FR-10, FR-13, FR-14, FR-22), p.12 (data flow), p.16 (responsibilities), p.19–21 (checkout_db DDL), p.22 (order state machine), p.24 (`checkout.orders`), p.26–27 (REST, gRPC), p.29–30 (flows 3 and 5), p.32–34 (failures).*

## Purpose

Checkout owns **the money path**: orders, payments, refunds and the sagas that tie them to Booking. It is separate so that the money path is *"isolated from inventory failures"*, and it **scales on orders** (p.16).

## Responsibilities

- **Orders** — one per hold (`orders.hold_id UNIQUE`).
- **Checkout saga** — `StartPayment` → payment intent → webhook → payment saved → `ConfirmHold`.
- **Refund saga** — `CancelTickets` → FakePay refund → webhook → `COMPLETED`.
- **Cancellations and quotes** — event refund policy from `catalog.v1.GetRefundPolicy`.
- **Payments** — FakePay intents keyed by `order_id`, deduped on `provider_ref`.
- **FakePay webhooks** — **signature verification** (HMAC).
- **Reconcilers** — orders stuck in `PAID_PENDING_CONFIRMATION` > 1 min, and payments/refunds pending > 2 min.
- **Nightly audit** (FR-22).

## Owned data

`checkout_db`: `orders`, `payments`, `refunds`, `refund_items`, `sagas`, `saga_steps`, `idempotency_keys`, `outbox`, `processed_events`. See [postgres-schema.md](../04-data/postgres-schema.md#checkout_db).

## Public API (via Gateway)

| Method and path | Auth | Purpose | Responses |
|---|---|---|---|
| `POST /v1/holds/{holdID}/checkout` | access token | Create order and payment intent | `201 {order_id, payment_url}` |
| `GET /v1/orders/{orderID}` | JWT | Order, tickets, refunds | `200` |
| `GET /v1/orders/{orderID}/cancellation-quote` | JWT | Refund amount under the event's policy | `200 {refund_paise, deadline}` |
| `POST /v1/orders/{orderID}/cancellations` `{ticket_ids}` | JWT | Cancel all or some tickets | `202 {refund_id}`, `409` past cutoff |
| `POST /v1/webhooks/fakepay` | HMAC signature | Payment and refund results | `204` |

## Internal gRPC — `checkout.v1`

`StartCheckout`, `GetOrder`, `QuoteCancellation`, `Cancel`.

## Checkout saga — outcome table (p.29)

| `ConfirmHold` response | Checkout action |
|---|---|
| **CONFIRMED** (new or already existing) | Order → `CONFIRMED`; outbox `OrderConfirmed` |
| **REJECTED** — the hold is no longer this order's (expired and released) | Order → `REFUND_PENDING`; start the refund saga with reason `CONFIRM_REJECTED` |
| **Timeout, `UNAVAILABLE`, or any unknown error** | Step → `UNKNOWN`; order stays `PAID_PENDING_CONFIRMATION`; retry with backoff. **Never refund on a timeout** |

**Order of durability (p.29):** in **one transaction**, Checkout records the payment `SUCCEEDED` (deduped on `provider_ref`), moves the order to `PAID_PENDING_CONFIRMATION` and creates a pending `CONFIRM_HOLD` saga step. *Payment success is durable before any confirmation attempt.*

**Reconciler (p.30):** for orders stuck in `PAID_PENDING_CONFIRMATION` > 1 min, call `Booking.GetBookingByOrder(order_id)`. A booking found → confirm. A definitive rejection → refund. Still unreachable → keep retrying and **alert after 15 minutes**.

## Refund saga (p.30)

1. Quote: apply the event policy, return amount + deadline.
2. `POST /cancellations` (Idempotency-Key) → `refunds` row `REQUESTED` + the first saga step.
3. `Booking.CancelTickets` → refund `TICKETS_VOIDED`.
4. FakePay refund **keyed by `refund_id`** → signed webhook → `COMPLETED`; `orders.refunded_paise += amount`, **never exceeding the amount paid** (`CHECK`).
5. Notifier emails the receipt. If FakePay keeps failing: retry with backoff, **alert after 1 hour**; tickets stay void.

## Saga execution model

- Sagas are **orchestrated by Checkout over gRPC** with **durable step state** (`sagas`, `saga_steps`). *Every step is safe to retry* (p.27).
- `sagas.version` gives **optimistic concurrency between workers**. `saga_steps` has `(saga_id, step)` as its primary key (a unique step identity) plus `attempts`, `next_attempt_at` and `last_error`.
- **A worker crash mid-saga:** another replica resumes from the last completed step (p.33).
- Step names given as examples: `CONFIRM_HOLD`, `SEND_REFUND`. The full list and the `sagas.state` values are unspecified ([AMB-25](../10-planning/ambiguities.md#amb-25)).

## Nightly audit (FR-22)

Checks:
1. Every `BOOKED` seat has **exactly one valid ticket**.
2. Every paid order has **tickets or a refund**.
3. **Ledger totals match FakePay.**

On mismatch: **page the owner**. The report lists the exact orders, seats and payments involved (p.34). Exported metrics: `double_booked_seats`, `oversold_ga` (must stay 0).

> Checks 1 and 2 span `booking_db` and `checkout_db`, but services may not read each other's tables. How the audit gets Booking data is unspecified ([AMB-17](../10-planning/ambiguities.md#amb-17)), as is what "ledger" means.

## Events produced

`checkout.orders` (key `order_id`): OrderCreated, PaymentSucceeded, PaymentFailed, OrderConfirmed, RefundRequested, RefundIssued, RefundFailed. Consumed by Notifier and analytics.

## FakePay resilience

Timeout + **circuit breaker**; the hold is extended for the payment window; failures release seats (p.34). Test T5: 3 s latency and 20 % failures, then down for 5 minutes.

## Performance targets

Checkout API **p99 < 300 ms**; confirmation **p99 < 500 ms**; refund **p99 < 60 s** (healthy) or **within 15 min of provider recovery**; peak ~**200 orders/s**.

## Open items

[AMB-04](../10-planning/ambiguities.md#amb-04) · [AMB-05](../10-planning/ambiguities.md#amb-05) · [AMB-11](../10-planning/ambiguities.md#amb-11) · [AMB-17](../10-planning/ambiguities.md#amb-17) · [AMB-23](../10-planning/ambiguities.md#amb-23) · [AMB-25](../10-planning/ambiguities.md#amb-25) · [AMB-31](../10-planning/ambiguities.md#amb-31) · [AMB-32](../10-planning/ambiguities.md#amb-32) · [AMB-33](../10-planning/ambiguities.md#amb-33) · [OQ-1](../10-planning/open-questions.md)
