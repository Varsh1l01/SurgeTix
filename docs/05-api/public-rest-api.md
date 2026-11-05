# Public REST API (via Gateway)

*Source: PDF p.25–26 (public REST table). Request/response shapes are given only as far as the PDF specifies them. Anything beyond that is unspecified, not omitted.*

Conventions (auth, idempotency, errors, deadlines): [api-conventions.md](api-conventions.md). Every **state-changing** call accepts an `Idempotency-Key` header (FR-12).

## Endpoint index

| # | Method and path | Auth | Purpose | Key responses | Backend |
|---|---|---|---|---|---|
| 1 | `POST /v1/auth/signup`, `/login`, `/refresh`, `/verify-email` | none / refresh token | Email/password auth and verification | `200` tokens, `401` | Identity |
| 2 | `GET /v1/auth/google/start`, `/callback` | none | Google OAuth with PKCE | `302` | Identity |
| 3 | `GET /v1/events/{eventID}` | JWT | Event details, sale status, refund policy | `200`, `404` | Catalog |
| 4 | `GET /v1/events/{eventID}/queue/challenge` | JWT | Proof-of-work challenge (nonce, difficulty) | `200` | Waiting Room |
| 5 | `POST /v1/events/{eventID}/queue` | JWT | Join with a solved challenge; returns a signed queue ticket | `201 {ticket, number}`, `409` already queued | Waiting Room |
| 6 | `GET /v1/events/{eventID}/queue/state` | none | Public, cacheable: `{admitted_upto, rate, sold_out}`; the client computes its own position | `200`, `Cache-Control: max-age=1` | Waiting Room (edge-cached) |
| 7 | `POST /v1/events/{eventID}/queue/admit` | JWT + queue ticket | Exchange an admitted ticket for an access token | `200` token, `425` not yet | Waiting Room |
| 8 | `POST /v1/events/{eventID}/second-chance` | JWT + queue ticket | Stay in line after sell-out | `201` | Waiting Room |
| 9 | `GET /v1/events/{eventID}/seats` | access token | Seat map (cached snapshot) | `200` | Catalog |
| 10 | `GET /v1/events/{eventID}/seats/stream` | access token | Live seat-map deltas (SSE) | event stream | [AMB-08](../10-planning/ambiguities.md#amb-08) |
| 11 | `POST /v1/events/{eventID}/holds` | access token | Hold `{seat_ids}`, or `{best_available: {section or tier, quantity}}`, or `{ga: {section, quantity}}` | `201 {hold_id, seats, expires_at, amount}`, `409`, `422` | Booking |
| 12 | `DELETE /v1/holds/{holdID}` | access token | Release a hold | `204` | Booking |
| 13 | `POST /v1/holds/{holdID}/checkout` | access token | Create order and payment intent | `201 {order_id, payment_url}` | Checkout |
| 14 | `GET /v1/orders/{orderID}` | JWT | Order, tickets, refunds | `200` | Checkout |
| 15 | `GET /v1/orders/{orderID}/cancellation-quote` | JWT | Refund amount under the event's policy | `200 {refund_paise, deadline}` | Checkout |
| 16 | `POST /v1/orders/{orderID}/cancellations` | JWT | Cancel all or some tickets `{ticket_ids}` | `202 {refund_id}`, `409` past cutoff | Checkout |
| 17 | `GET /v1/tickets/{ticketID}/qr` | JWT (owner) | Current rotating QR payload | `200` | Booking |
| 18 | `POST /v1/tickets/{ticketID}/transfers` | JWT (owner) | Transfer to `{email}` of a verified account | `201`, `409` limit reached | Booking (+ Identity lookup) |
| 19 | `POST /v1/scan` | staff JWT | Validate a QR payload and check in | `200 {valid, seat}`, `409` already used | Booking |
| 20 | `POST /v1/webhooks/fakepay` | HMAC signature | Payment and refund results | `204` | Checkout |
| 21 | `POST /v1/admin/events`, `PUT …/sale`, `POST …/sale:pause`, `…/sale:resume` | admin JWT | Create event, configure, control a live sale | `200`, `201` | Catalog / Waiting Room ([AMB-15](../10-planning/ambiguities.md#amb-15)) |

## Endpoint details

### Queue

**`GET …/queue/challenge`** returns a nonce and a difficulty. The client spends about 1 s of CPU (difficulty: [OQ-2](../10-planning/open-questions.md)).

**`POST …/queue`** takes the solved challenge. The server verifies it, burns the nonce (`pow:{nonce}`), claims `wr:{event}:user:{user}` with `SET NX` and returns `201 {ticket, number}`. `409` means the user is already queued. Requires an email-verified account (FR-1).

**`GET …/queue/state`** returns `{admitted_upto, rate, sold_out}`. The client computes:
- `position = my_number − admitted_upto`
- `ETA = position ÷ rate`
- Poll interval: 5 s near the front, 30 s far back.

**`POST …/queue/admit`** is called **once** when `admitted_upto ≥ my_number`. Returns a 15-minute access token bound to user and event. `425` means not yet admitted.

**`POST …/second-chance`** opts into the second-chance queue after `sold_out` flips.

### Holds

**`POST …/holds`** takes exactly one of three body shapes:

| Mode | Body (as written in the PDF) | Notes |
|---|---|---|
| Chosen seats | `{seat_ids}` | 1–4 seats, all or nothing |
| Best-available | `{best_available: {section or tier, quantity}}` | Server picks adjacent seats ([AMB-09](../10-planning/ambiguities.md#amb-09) on "tier") |
| General admission | `{ga: {section, quantity}}` | Never oversold |

- **`201`** `{hold_id, seats, expires_at, amount}`. For GA, the PDF says the response returns "the seats **or allocation**" (p.29).
- **`409`** lists unavailable seats **with suggested alternatives** ([AMB-26](../10-planning/ambiguities.md#amb-26)).
- **`422`** for an invalid body or an idempotency key reused with a different body.

**`DELETE /v1/holds/{holdID}`** releases early (FR-23). Returns `204`.

### Checkout, orders, cancellation

**`POST /v1/holds/{holdID}/checkout`** returns `201 {order_id, payment_url}`. The buyer then pays on the FakePay page at `payment_url`. Latency target p99 < 300 ms (excludes paying).

**`GET /v1/orders/{orderID}`** returns the order, tickets and refunds.

**`GET …/cancellation-quote`** returns `{refund_paise, deadline}` under the event's policy.

**`POST …/cancellations`** with `{ticket_ids}` (all or some) returns `202 {refund_id}`. `409` means past the cutoff.

### Tickets and gate

**`GET /v1/tickets/{ticketID}/qr`** returns the current rotating payload (HMAC of the secret and the 30 s window). The secret itself is never sent.

**`POST /v1/tickets/{ticketID}/transfers`** with `{email}`. The recipient must be a verified account. `409` means the limit was reached (1 per ticket). Transfers are disabled within 24 h of the event (FR-16), but the PDF does not state the response code for that case ([AMB-12](../10-planning/ambiguities.md#amb-12)).

**`POST /v1/scan`** (staff) returns `200 {valid, seat}`. `409` means already used. Accepts the current and the previous 30 s window. The first scan wins.

### Webhook

**`POST /v1/webhooks/fakepay`** carries payment and refund results with an HMAC signature. Returns `204`. Duplicates are deduped on `provider_ref`.

### Admin

Create event, configure the sale, pause, resume. A per-sale **kill switch** is required (FR-20, NFR-OPS), but **no endpoint is listed for it** ([AMB-15](../10-planning/ambiguities.md#amb-15)).

## Not specified by the PDF

Full JSON schemas, pagination, the token response shape, SSE event names and payloads, the QR payload format, and the scan request body. See [AMB-12](../10-planning/ambiguities.md#amb-12). These will be fixed in protobuf/OpenAPI definitions during phase 4 and are subject to approval.
