# Gateway Service

*Source: PDF p.10–12 (role, data flow), p.13 (tech stack: Gateway, Edge cache), p.15 (responsibilities), p.23 (`rl:` keys), p.25–26 (public REST), p.27 (deadlines), p.28 (hold flow step 1), p.33–34 (failure handling).*

## Purpose

The Gateway is the **single public HTTP entry point**: *"Every browser request enters through the gateway"* (p.11). It runs **stateless edge logic** and translates **REST to gRPC**. FakePay's webhooks also come in through it.

## Implementation constraints (from the Tech stack)

- Built on **Go `net/http` with pattern routing** (p.13). It is **not** Kong/Envoy/Traefik. Those are named only as *"the production swap"*.
- Sits **behind the edge cache**: **NGINX `proxy_cache` with a 1 s microcache** locally, or a CDN such as Cloudflare if hosted (p.13).

## Responsibilities

| Responsibility | Detail | Source |
|---|---|---|
| **Routing / REST → gRPC** | Maps public REST/SSE routes to backend gRPC calls ([public-rest-api](../05-api/public-rest-api.md)). | p.13, p.25–26 |
| **JWT verification** | Verifies RS256 JWTs. Identity's **JWT keys are cached by the Gateway** (JWKS), so the hot path makes no Identity call. | p.11, p.15 |
| **Access-token verification** | Admission tokens bound to user and event on seat-map, hold, release and checkout routes (`waitingroom.v1.VerifyAccessToken`). | p.15, p.26 |
| **Rate limits** | **Per-user and per-IP** token buckets in Redis (`rl:{scope}:{key}`, Hash, TTL). | p.10, p.23 |
| **Request IDs / tracing** | Assigns request IDs and starts the trace gateway → service → Postgres/Redis → Kafka → notifier. | p.15, p.34 |
| **Cacheable queue-status route** | Serves `GET /v1/events/{eventID}/queue/state` with `Cache-Control: max-age=1` for the edge cache. If the edge is down, falls back to an **in-process 1 s cache**. | p.15, p.25, p.33 |
| **Deadline budget** | Each request gets **1 s**. Internal calls inherit the remainder. The diagram shows `HoldSeats` called with a 1 s deadline. | p.12, p.27 |
| **Webhook ingress** | Forwards `POST /v1/webhooks/fakepay` to Checkout, which verifies the signature. | p.11, p.29 |

## Route → backend map

*Derived from the REST table (p.25–26), the gRPC table (p.26–27) and the data-flow diagram (p.12). Rows marked † are not explicitly paired with an RPC in the PDF.*

| Public route | Backend call |
|---|---|
| `POST /v1/auth/*`, `GET /v1/auth/google/*` | Identity † |
| `GET /v1/events/{id}` | `catalog.v1.GetEvent` |
| `GET …/queue/challenge` | Waiting Room † |
| `POST …/queue` | `waitingroom.v1.Join` |
| `GET …/queue/state` | `waitingroom.v1.GetState` (edge-cached) |
| `POST …/queue/admit` | `waitingroom.v1.Admit` |
| `POST …/second-chance` | Waiting Room † |
| `GET …/seats` | `catalog.v1.GetSeatMap` (Redis snapshot) |
| `GET …/seats/stream` | SSE. Owner unclear ([AMB-08](../10-planning/ambiguities.md#amb-08)) |
| `POST …/holds` | `booking.v1.HoldSeats` / `HoldBestAvailable` / `HoldGA` |
| `DELETE /v1/holds/{id}` | `booking.v1.ReleaseHold` |
| `POST /v1/holds/{id}/checkout` | `checkout.v1.StartCheckout` |
| `GET /v1/orders/{id}` | `checkout.v1.GetOrder` |
| `GET …/cancellation-quote` | `checkout.v1.QuoteCancellation` |
| `POST …/cancellations` | `checkout.v1.Cancel` |
| `GET /v1/tickets/{id}/qr` | Booking † |
| `POST /v1/tickets/{id}/transfers` | `booking.v1.TransferTicket` |
| `POST /v1/scan` | `booking.v1.CheckIn` |
| `POST /v1/webhooks/fakepay` | Checkout † |
| `/v1/admin/*` | Catalog (`PublishSale`) and Waiting Room (pause/resume) † ([AMB-15](../10-planning/ambiguities.md#amb-15)) |

## Scaling and availability

- **Scales on:** request rate (p.15).
- **≥ 2 replicas** on Kubernetes (NFR-AVL).
- Stateless. The only shared state is the Redis rate-limit buckets.

## Failure behaviour

| Failure | Gateway behaviour | Source |
|---|---|---|
| CDN / edge cache down | Serve `queue/state` from the in-process 1 s cache. Clients back off on `429`. | p.33 |
| Hot event's Redis shard saturated | Joins shed with **`429` + `Retry-After`**. The edge cache keeps status reads off Redis. | p.33 |
| Redis down | Admitted users continue with **signed** tokens. Rate-limit behaviour when Redis is down is unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)). | p.33 |
| Bot floods | Per-IP limits (plus PoW at the Waiting Room). | p.34 |

## Security rules

- **JWT on all buyer endpoints.** Unauthenticated: `auth/*`, `google/*`, `queue/state` (public) and the HMAC-signed webhook.
- Admission tokens are **bound to user and event**. A token for event A must not authorise anything on event B.
- `/v1/scan` requires a **staff JWT**, and `/v1/admin/*` an **admin JWT** ([AMB-14](../10-planning/ambiguities.md#amb-14)).

## Observability

RED metrics per route; `queue/state` cache hit ratio; **origin requests per 1,000 queue polls** (p.34).

## Naive baseline route (phase 2 only)

With build tag `unsafe_baseline` **and** `SURGETIX_ENABLE_UNSAFE_BASELINE=true`, the Gateway forwards exactly one route, `POST /v1/baseline/events/{eventID}/holds`, to Booking over **internal HTTP** (`GATEWAY_BOOKING_URL`) ([ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035), AMB-40). It applies the 1 s gateway deadline (`504` on expiry, `502` if Booking is down) and keeps up to 1,024 idle connections to Booking. There are no auth, rate limits or idempotency in front of it; those arrive in phase 4. gRPC replaces the HTTP hop in phase 4. Code: `internal/gateway/baseline`.

## Open items

[AMB-08](../10-planning/ambiguities.md#amb-08) · [AMB-12](../10-planning/ambiguities.md#amb-12) (rate-limit values, JWKS cache TTL) · [AMB-14](../10-planning/ambiguities.md#amb-14) · [AMB-15](../10-planning/ambiguities.md#amb-15)
