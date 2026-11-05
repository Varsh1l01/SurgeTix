# API Conventions

*Source: PDF p.24–25 (API design), p.27 (deadlines, retries), p.28 (idempotency claim semantics), p.8 (security, idempotency NFRs), p.13 (gRPC + buf).*

> *Browsers talk REST (plus SSE) to the gateway; services talk gRPC to each other with propagated deadlines. Every write takes an `Idempotency-Key`; errors use RFC 9457 problem details.* (p.24–25)

## Versioning and layout

- Public REST paths are prefixed **`/v1/`**.
- Internal gRPC uses **one proto package per service**, versioned `.v1` (`identity.v1`, `catalog.v1`, `waitingroom.v1`, `booking.v1`, `checkout.v1`).
- Protobuf contracts are linted and checked for **breaking changes with `buf`** (p.13).
- Admin custom actions use a colon verb suffix: `POST …/sale:pause`, `POST …/sale:resume` (p.26).

## Authentication schemes

| Scheme | Used on | Issued by | Bound to |
|---|---|---|---|
| **None** | `auth/*`, `google/*`, `queue/state` (public, cacheable) | — | — |
| **JWT** (RS256, JWKS) | Buyer endpoints: event details, queue join/admit, orders, tickets | Identity | User |
| **Refresh token** | `POST /v1/auth/refresh` | Identity | User |
| **Queue ticket** (signed) | `POST …/queue/admit`, `POST …/second-chance` | Waiting Room | User + event + number + `exp` |
| **Access token** (15 min; 5 min section-scoped for second-chance) | Seat map, SSE, holds, release, checkout | Waiting Room | User + event (+ section) |
| **Staff JWT** | `POST /v1/scan` | Identity ([AMB-14](../10-planning/ambiguities.md#amb-14)) | Staff user |
| **Admin JWT** | `/v1/admin/*` | Identity ([AMB-14](../10-planning/ambiguities.md#amb-14)) | Admin user |
| **HMAC signature** | `POST /v1/webhooks/fakepay` | FakePay | Request body |

## Idempotency

*FR-12: every state-changing endpoint accepts an `Idempotency-Key`.* NFR-IDEM lists hold, checkout, cancel, refund, transfer, payment webhook and event consumption, each producing **exactly one effect**.

**Record:** `idempotency_keys (scope, user_id, key)` primary key, `request_hash`, `status IN ('IN_PROGRESS','COMPLETED')`, stored `response_code`/`response_body`, `expires_at`.

**Claim semantics (p.28):**

| Existing record | Response |
|---|---|
| None | Claim as `IN_PROGRESS`, execute, store the response, mark `COMPLETED` |
| `COMPLETED`, same body hash | **Return the stored response** |
| `IN_PROGRESS` | **`409` — retry later** |
| Same key, **different body** | **`422`** |

- `scope` is the operation, e.g. `'POST /holds'`.
- Webhooks are deduped on **`provider_ref`** (unique), and refunds also on `refunds.idempotency_key` (unique).
- Kafka consumption is deduped on **`processed_events.event_id`**.
- Defence in depth: business unique keys stop duplicate effects **even if dedup is bypassed** (p.22).
- Retention (`expires_at`) is unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)).

## Errors — RFC 9457

All error responses are `application/problem+json` per RFC 9457. The PDF specifies the status codes but not the problem `type` URIs or extension members ([AMB-12](../10-planning/ambiguities.md#amb-12)).

| Status | Meaning in SurgeTix | Examples |
|---|---|---|
| `401` | Bad credentials / token | auth endpoints |
| `404` | Unknown resource | `GET /events/{id}` |
| `409` | Conflict with current state | seat taken (with alternatives), already queued, idempotency `IN_PROGRESS`, cancellation past cutoff, transfer limit reached, ticket already scanned |
| `422` | Idempotency key reused with a different body (p.28). `422` is also listed for holds without further meaning (p.25) | holds |
| `425` | Too Early: not yet admitted | `POST /queue/admit` |
| `429` | Rate-limited / shed load, with `Retry-After` | queue joins when the shard is saturated; clients back off |
| `5xx` / timeout | Counted in the buyer-facing error-rate metric (target ≥ 10× lower than baseline) | — |

## Deadlines and retries (p.27)

- **The Gateway gives each request a 1 s budget.** Internal calls **inherit the remainder** (gRPC deadline propagation).
- **Only `UNAVAILABLE` is retried**, with **jittered backoff**, at the RPC layer.
- Saga steps have their own durable retry schedule (`saga_steps.attempts`, `next_attempt_at`). A timeout during `ConfirmHold` marks the step `UNKNOWN` and is retried by the saga, not by the RPC client. How these two retry layers relate is [AMB-33](../10-planning/ambiguities.md#amb-33).

## Caching

- `GET /v1/events/{eventID}/queue/state` returns **`Cache-Control: max-age=1`**. It is identical for every user (no per-user data), so the edge (NGINX `proxy_cache` / CDN) can share it.
- Seat-map responses come from Redis snapshots (TTL 2 s + jitter). Staleness ≤ 2 s.

## Money and identifiers

- Money is **integer paise** (`*_paise`), never floats.
- Entity IDs are **UUIDs**. Seat IDs are **text** such as `'A-12'`.
- Responses that carry an amount (`201 {…, amount}`, `{refund_paise, deadline}`) use paise.

## Streaming

- **SSE** for live seat-map deltas (`GET …/seats/stream`, `text/event-stream`). Deltas only, never full maps.
- gRPC server-stream `booking.v1.WatchSeats` exists internally ([AMB-08](../10-planning/ambiguities.md#amb-08)).

## SQL safety

All SQL is **parameterized** (NFR-SEC).
