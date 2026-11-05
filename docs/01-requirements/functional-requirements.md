# Functional Requirements

*Source: PDF p.5–7. Traceability columns are derived from PDF p.15–16 (service ownership), p.27–31 (flows) and p.35–36 (tests).*

**Priority meaning (PDF p.5):** everything listed is **in scope**. **P0** proves the core claim. **P1** completes the ticket lifecycle and adds the efficiency features that make the project stand out.

## Requirements

### FR-1 — Authentication (P0)
Sign up and sign in with **email/password (bcrypt)** and **Google OAuth with PKCE**. **RS256 JWTs** are verified via **JWKS**. **Email verification is required before joining a queue.**

- Owner: **Identity** (signing keys never leave it). Verification at the **Gateway**.
- Endpoints: `POST /v1/auth/signup|login|refresh|verify-email`, `GET /v1/auth/google/start|callback`.
- RPCs: `identity.v1.GetJWKS`, `VerifyToken`.
- Open: [AMB-24](../10-planning/ambiguities.md#amb-24) (Google accounts and email verification), [AMB-12](../10-planning/ambiguities.md#amb-12) (token TTLs).

### FR-2 — Event creation (P0)
The organizer creates an event with a **venue**, **reserved sections** (rows, seats, price tiers), **general-admission sections**, a **sale start time**, a **per-user limit** and a **refund policy**.

- Owner: **Catalog** (`events`, `venues`, `seat_layouts`).
- Endpoints: `POST /v1/admin/events`, `PUT …/sale`.
- RPC: `catalog.v1.PublishSale` copies the layout into Booking's `seats`.
- Open: [AMB-09](../10-planning/ambiguities.md#amb-09) (tiers), [AMB-10](../10-planning/ambiguities.md#amb-10) (limit semantics), [AMB-13](../10-planning/ambiguities.md#amb-13) (Catalog schema), [OQ-1](../10-planning/open-questions.md) (refund-policy defaults).

### FR-3 — Event details and seat map (P0)
The buyer views event details and a **seat map with near-real-time availability** (≤ 2 s stale).

- Owners: **Catalog** (details, Redis snapshot `seatmap:{event}:{section}`). **Booking** is the inventory truth.
- Endpoints: `GET /v1/events/{eventID}`, `GET /v1/events/{eventID}/seats`.

### FR-4 — Waiting room join (P0)
The buyer joins a **per-event waiting room** and receives a **signed queue ticket**. **Position and ETA are computed from one cached global counter.**

- Owner: **Waiting Room**.
- Endpoints: `GET …/queue/challenge`, `POST …/queue`, `GET …/queue/state`.
- Flow: [01-queue-join-and-admission](../06-flows/01-queue-join-and-admission.md).

### FR-5 — Adaptive admission (P0)
The admission controller admits users at a rate that **adapts to booking latency and database load**. Admitted users exchange their queue ticket for a **short-lived access token**.

- Owner: **Waiting Room** (leader with epoch-fenced lease).
- Endpoint: `POST …/queue/admit` (`200` token, `425` not yet).
- Open: [AMB-12](../10-planning/ambiguities.md#amb-12) (AIMD parameters).

### FR-6 — Hold specific seats (P0)
An admitted buyer holds **1–4 specific seats atomically (all or nothing) for 10 minutes**.

- Owner: **Booking**. `POST …/holds {seat_ids}`. RPC `booking.v1.HoldSeats`.

### FR-7 — Best-available (P0)
The buyer asks for **N seats in a section or price tier**, and the server picks **adjacent free seats, skipping locked rows**.

- Owner: **Booking**. `POST …/holds {best_available:{section|tier, quantity}}`. RPC `HoldBestAvailable`.

### FR-8 — General admission (P0)
The buyer holds **N tickets from a GA section** without choosing seats. GA is **never oversold**.

- Owner: **Booking** (`ga_buckets`, 32 buckets). `POST …/holds {ga:{section, quantity}}`. RPC `HoldGA`.

### FR-9 — Hold expiry (P0)
Holds **expire automatically** and inventory returns to sale.

- Owner: **Booking** expiry worker (every 2 s) plus **inline reclaim** on the hold path.
- Flow: [04-hold-expiry](../06-flows/04-hold-expiry.md).

### FR-10 — Checkout and payment (P0)
Check out a hold: create the order, pay via FakePay, confirm on the **signed webhook**. **Refund automatically only when confirmation is definitively rejected, never on a timeout.**

- Owner: **Checkout** (saga). Booking provides `StartPayment` and `ConfirmHold`.
- Endpoints: `POST /v1/holds/{holdID}/checkout`, `POST /v1/webhooks/fakepay`.
- Flow: [03-checkout-and-payment-saga](../06-flows/03-checkout-and-payment-saga.md).

### FR-11 — Tickets with rotating QR (P0)
A confirmed booking issues tickets with **rotating QR codes** (time-based codes derived from a per-ticket secret).

- Owner: **Booking** (`tickets.qr_secret_enc`). `GET /v1/tickets/{ticketID}/qr`.

### FR-12 — Idempotency everywhere (P0)
**Every state-changing endpoint accepts an `Idempotency-Key`.**

- All writing services (`idempotency_keys` table). See [api-conventions](../05-api/api-conventions.md#idempotency).

### FR-13 — Cancellation (P1)
The buyer cancels a booking (**whole or some tickets**) **before the event's cutoff**. The refund amount follows the event's policy (example: 100 % until 7 days out, 50 % until 24 hours, then none).

- Owner: **Checkout**. Booking performs `CancelTickets`.
- Endpoints: `GET /v1/orders/{orderID}/cancellation-quote`, `POST /v1/orders/{orderID}/cancellations`.
- Open: [OQ-1](../10-planning/open-questions.md), [AMB-32](../10-planning/ambiguities.md#amb-32).

### FR-14 — Refund saga (P1)
Refunds run as a **saga**: invalidate tickets, refund through FakePay, return inventory, notify. **Exactly once even under retries.**

- Owner: **Checkout**. Flow: [05-cancel-and-refund-saga](../06-flows/05-cancel-and-refund-saga.md).

### FR-15 — Second-chance queue (P1)
After sell-out, users can stay in line. Inventory returned by **expiry, failed payment or cancellation** is offered to them **in order**.

- Owner: **Waiting Room** (`wr:{event}:second_chance`). **Notifier** pushes offers.
- Endpoint: `POST …/second-chance`. Flow: [06-second-chance-offers](../06-flows/06-second-chance-offers.md).
- Open: [AMB-07](../10-planning/ambiguities.md#amb-07), [OQ-4](../10-planning/open-questions.md).

### FR-16 — Ticket transfer (P1)
Transfer a ticket to another **verified account**: **max 1 transfer per ticket**, **disabled within 24 hours of the event**, and the **old QR secret is revoked**.

- Owner: **Booking**. Identity provides `LookupUserByEmail`.
- Endpoint: `POST /v1/tickets/{ticketID}/transfers`.

### FR-17 — Gate scan (P1)
Online only (gates need connectivity). The staff app validates a QR code and checks the ticket in **exactly once**.

- Owner: **Booking** (`CheckIn`). Endpoint: `POST /v1/scan`.

### FR-18 — Bot defense (P1)
**Proof-of-work** challenge on queue join, **per-account and per-IP limits**, **purchase caps per account and payment method**.

- Owners: **Waiting Room** (PoW), **Gateway** (rate limits), **Booking/Checkout** (caps).
- Open: [AMB-10](../10-planning/ambiguities.md#amb-10), [OQ-2](../10-planning/open-questions.md).

### FR-19 — Live seat-map deltas (P1)
Live seat-map **deltas** are pushed to viewers over **SSE**.

- Endpoint: `GET …/seats/stream`. Open: [AMB-08](../10-planning/ambiguities.md#amb-08) (which component serves SSE).

### FR-20 — Admin control room (P1)
Shows queue length, admission rate, seats left and errors. Provides **pause, resume and kill switch** per sale.

- Endpoints: `POST …/sale:pause`, `…/sale:resume`. Grafana *Surge Control Room*.
- Open: [AMB-15](../10-planning/ambiguities.md#amb-15).

### FR-21 — Async emails (P1)
Ticket confirmation and refund emails are sent **asynchronously**.

- Owner: **Notifier** (consumes Kafka). Delivered via **Mailpit** (local SMTP inbox) per the tech stack (PDF p.15). Real delivery (SendGrid/SES) is a swappable non-goal.

### FR-22 — Nightly audit (P1)
Checks that every `BOOKED` seat has one valid ticket, every paid order has tickets or a refund, and **ledger totals match FakePay**.

- Owner: **Checkout** (PDF p.16). Open: [AMB-17](../10-planning/ambiguities.md#amb-17).

### FR-23 — Early release (P1)
The buyer releases a hold early.

- Owner: **Booking**. `DELETE /v1/holds/{holdID}` → `204`. RPC `ReleaseHold`.

## Traceability matrix

| FR | Pri | Owning service(s) | Flow | Test(s) | Build phase |
|---|---|---|---|---|---|
| FR-1 | P0 | Identity, Gateway | — | (e2e) | 4 Services & gRPC |
| FR-2 | P0 | Catalog | — | (e2e) | 4 Services & gRPC |
| FR-3 | P0 | Catalog, Booking | — | T3 | 4 Services & gRPC |
| FR-4 | P0 | Waiting Room, Gateway | 1 | T2, T10 | 5 Waiting room |
| FR-5 | P0 | Waiting Room | 1 | T2, T3, T13 | 5 Waiting room |
| FR-6 | P0 | Booking | 2 | T1 | 3 Booking core |
| FR-7 | P0 | Booking | 2 | T8 | 3 Booking core |
| FR-8 | P0 | Booking | 2 | T7 | 3 Booking core |
| FR-9 | P0 | Booking | 4 | T1, T3 | 3 Booking core |
| FR-10 | P0 | Checkout, Booking, FakePay | 3 | T3, T5, T12 | 6 Checkout saga |
| FR-11 | P0 | Booking | 7 | T11 | 6 Checkout saga / 7 Lifecycle |
| FR-12 | P0 | All writing services | 2, 3, 5, 7 | T1, T12 | 3 Booking core |
| FR-13 | P1 | Checkout, Booking | 5 | T9 | 7 Lifecycle |
| FR-14 | P1 | Checkout, Booking, FakePay | 5 | T5, T9 | 7 Lifecycle |
| FR-15 | P1 | Waiting Room, Notifier, Booking | 6 | T9 | 7 Lifecycle |
| FR-16 | P1 | Booking, Identity | 7 | (e2e) | 7 Lifecycle |
| FR-17 | P1 | Booking, Gateway | 7 | T11 | 7 Lifecycle |
| FR-18 | P1 | Waiting Room, Gateway, Booking/Checkout | 1 | T10 | 5 Waiting room |
| FR-19 | P1 | Notifier / Booking (see AMB-08) | — | (e2e) | 8 UI & dashboards |
| FR-20 | P1 | Waiting Room, Catalog, Grafana | — | (demo) | 8 UI & dashboards |
| FR-21 | P1 | Notifier | 3, 5 | (e2e) | 6 / 7 |
| FR-22 | P1 | Checkout | — | Audit after every run | 8 UI & dashboards ("audit") |
| FR-23 | P1 | Booking | — | (e2e) | 3 Booking core |

**Derived:** the "Build phase" column maps each FR to the phase whose description on PDF p.36 names it. Where the PDF does not name an FR in any phase (FR-1/2/3 under "Services and gRPC", FR-23), the mapping is a best reading and is listed in [AMB-02](../10-planning/ambiguities.md#amb-02) for confirmation. "(e2e)" means the PDF defines no numbered scenario for the FR, so coverage would come from end-to-end/integration tests.
