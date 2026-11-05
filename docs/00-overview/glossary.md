# Glossary

*Source: terms as used throughout the PDF. Definitions are restricted to what the PDF states.*

## Domain terms

| Term | Meaning in SurgeTix |
|---|---|
| **Event** | A single ticketed occasion (e.g. a stadium concert) with one sale. Owned by Catalog. |
| **Venue** | The place an event happens. Owned by Catalog. |
| **Seat layout** | The static seat map of a venue/event (`seat_layouts` in Catalog), copied into Booking's `seats` when a sale is published. |
| **Reserved section** | A section with rows, numbered seats and price tiers. Buyers pick specific seats or use best-available. |
| **General admission (GA) section** | A section without assigned seats. Buyers hold a *quantity*. Inventory is spread over 32 counter buckets. |
| **Price tier** | A price grouping of reserved seats. Best-available can target a section *or* a tier (see [AMB-09](../10-planning/ambiguities.md#amb-09)). |
| **Sale** | The time-boxed selling of an event's inventory. It has a start time, per-user limit and refund policy, and can be paused, resumed or killed. |
| **Pre-sale arrival** | A user who joins the waiting room before sale start. They are placed in a shuffle set and get a random ticket number at start. |
| **Waiting room** | Per-event virtual queue that meters users into the booking path. |
| **Queue ticket** | A signed token `{user, event, number, exp}` proving a user's place in line. |
| **Ticket number** | A user's position number in the queue. |
| **Admitted-up-to (`admitted_upto`)** | The single public counter: the highest queue number allowed into the booking path. It only increases. |
| **Access token** | A 15-minute token bound to user and event, issued on admission. Required for seat-map, hold and checkout calls. A second-chance offer issues a 5-minute token scoped to a section. |
| **Hold** | A time-limited reservation (10 minutes) of 1–4 seats or N GA tickets for one user. Kinds: `SEATS`, `GA`. |
| **Best-available** | Server-side selection of N adjacent free seats in a section or tier, skipping rows locked by others. |
| **Order** | Checkout's record of a purchase of exactly one hold (`orders.hold_id UNIQUE`). |
| **Payment intent** | FakePay-side object created per order, keyed by `order_id`. |
| **Booking** | Booking-service record created by `ConfirmHold`. It makes confirmation idempotent by `order_id`. |
| **Ticket** | An issued entitlement (seat or GA) with a rotating-QR secret, an owner and a status (`VALID`, `CANCELLED`, `CHECKED_IN`). |
| **Cancellation** | Buyer-initiated return of all or some tickets before the event's cutoff. |
| **Cutoff** | The latest time a cancellation is allowed under the event's refund policy. |
| **Refund policy** | Per-event rule mapping time-before-event to refund percentage (e.g. 100 % until 7 days, 50 % until 24 h, then none). Defaults are an open question ([OQ-1](../10-planning/open-questions.md)). |
| **Second-chance queue** | After sell-out, users who opt in wait in order to be offered returned inventory. |
| **Second-chance offer** | A reserved hold plus a 5-minute section-scoped access token given to the next user in the second-chance queue. |
| **Transfer** | A free move of ticket ownership to another verified account. Max 1 per ticket, disabled within 24 h of the event. |
| **Check-in** | The one-time transition of a ticket to `CHECKED_IN` at the gate. |
| **Nightly audit** | A reconciliation job: every `BOOKED` seat has one valid ticket, every paid order has tickets or a refund, and ledger totals match FakePay. |
| **Naive baseline** | A deliberately broken check-then-book build used as the comparison target in every load test (T6). |
| **Kill switch** | Per-sale emergency control (FR-20, operability NFR). |

## Technical terms

| Term | Meaning |
|---|---|
| **Funnel** | The layered architecture: cheap layers absorb the crowd, and only a bounded stream reaches the correctness core. |
| **Conditional update** | `UPDATE … WHERE <expected current state>`. The update succeeds only if the row is still in the expected state. Used for every seat/order/ticket transition. |
| **`SKIP LOCKED`** | Postgres row-locking mode in which concurrent transactions skip rows already locked by others. Used to pick best-available candidates, GA buckets, expiry batches and outbox rows. |
| **HOT update** | Postgres heap-only tuple update. `seats` uses `fillfactor = 80` to leave room for them. |
| **Idempotency key** | Client-supplied `Idempotency-Key` header. A retry with the same key returns the stored response, and the same key with a different body returns `422`. |
| **Outbox** | A per-service table written in the same transaction as state changes. A relay publishes its rows to Kafka, so events are never lost. |
| **Outbox relay** | The process that claims `outbox` rows and publishes them to Kafka. |
| **`processed_events`** | Consumer-side dedup table keyed by `event_id`. |
| **Saga** | A multi-step, durable, retryable business process (`CHECKOUT`, `REFUND`) orchestrated by Checkout over gRPC with step state in `sagas` / `saga_steps`. |
| **Reconciler** | A background loop that resolves stuck orders (`PAID_PENDING_CONFIRMATION` > 1 min) and pending payments/refunds (> 2 min). |
| **Definitive rejection** | `ConfirmHold` returning `REJECTED` because the hold is no longer this order's. It is the only confirmation outcome that triggers a refund. |
| **`UNKNOWN` step** | A saga step whose outcome is unknown (timeout, `UNAVAILABLE`, unknown error). It is retried and **never** refunded. |
| **Admission controller** | The leader process that raises `admitted_upto` at an adaptive rate. |
| **Lease / epoch / fencing token** | Leadership mechanism. The leader holds `wr:{event}:lease = owner:epoch` (TTL 5 s), and every counter write checks the epoch atomically in Lua, so a stale leader cannot write. |
| **AIMD** | Additive increase, multiplicative decrease. The admission rate rises while healthy and halves when not. |
| **Hash tag `{event}`** | Redis Cluster mechanism that forces all `wr:{event}:*` keys into one slot so Lua scripts can touch them atomically. |
| **Proof-of-work (PoW)** | A challenge costing about 1 s of client CPU, required to join the queue. |
| **Seat bitmap (`seatbits`)** | Redis bitmap used as a fast pre-filter for clearly taken seats. Postgres stays the truth. |
| **Seat-map snapshot** | A cached per-section availability string in Redis (TTL 2 s + jitter). |
| **SSE** | Server-Sent Events, used for one-way live seat-map deltas. |
| **RED metrics** | Rate, Errors, Duration, per service. |
| **PGO** | Go profile-guided optimisation. All services are built with it. |
| **RFC 9457** | *Problem Details for HTTP APIs*, the error body format. |
| **TOTP-style HMAC** | QR code = HMAC(ticket secret, current 30-second window). |
| **Paise** | 1/100 of an Indian rupee. All money columns are integer paise. |
| **k6** | The load-generation tool for all scenarios. |
| **Surge Control Room** | The Grafana dashboard that records baseline and SurgeTix runs. |
