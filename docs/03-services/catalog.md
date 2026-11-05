# Catalog Service

*Source: PDF p.5 (FR-2, FR-3), p.12 (data flow), p.15 (responsibilities), p.22 (owned tables, publish), p.23 (`seatmap:` key), p.24 (Kafka consumer), p.26–27 (admin REST, gRPC).*

## Purpose

Catalog owns **what is being sold**: events, venues, layouts, and sale and refund configuration. It also serves the **read-heavy seat map**. Its scaling reason is *"read-heavy; cached aggressively"* (p.15).

## Responsibilities

| Responsibility | Detail |
|---|---|
| **Events and venues** | Created by the organizer: venue, reserved sections (rows, seats, price tiers), GA sections, sale start, per-user limit, refund policy (FR-2). |
| **Static seat layout** | `seat_layouts` is the static map. It is **copied into Booking's `seats` when a sale is published** (p.22) via `PublishSale`. |
| **Sale and refund policy config** | Provides `GetRefundPolicy` for Checkout's cancellation quote. |
| **Seat-map snapshots** | Maintains `seatmap:{event}:{section}` in Redis (String, **TTL 2 s + jitter**) for page loads. Refreshed from `booking.inventory` events (*"refresh"* arrow, p.10). |
| **Serving the seat map** | `GET /v1/events/{eventID}/seats` → *"Catalog serves a Redis snapshot"* (p.12). |

## Owned data

- Postgres: `events`, `venues`, `seat_layouts` (p.22). **Columns are not specified** ([AMB-13](../10-planning/ambiguities.md#amb-13)).
- Redis: `seatmap:{event}:{section}`.

Booking, not Catalog, is the **inventory source of truth**. Catalog's snapshot is an eventually consistent view, **≤ 2 s stale** (NFR-CONS).

## Public API (via Gateway)

| Method and path | Auth | Purpose | Responses |
|---|---|---|---|
| `GET /v1/events/{eventID}` | JWT | Event details, sale status, refund policy | `200`, `404` |
| `GET /v1/events/{eventID}/seats` | access token | Seat map (cached snapshot) | `200` |
| `POST /v1/admin/events` | admin JWT | Create event | `201` |
| `PUT /v1/admin/events/…/sale` | admin JWT | Configure sale | `200` |

The PDF writes the sale paths as `PUT .../sale` and pause/resume as `POST .../sale:pause` and `.../sale:resume` (p.26), so the full path prefix is elided. Pause/resume ownership is discussed under [AMB-15](../10-planning/ambiguities.md#amb-15).

## Internal gRPC — `catalog.v1`

| RPC | Purpose |
|---|---|
| `GetEvent` | Event details |
| `GetSeatMap` | Seat-map snapshot |
| `GetRefundPolicy` | Policy for cancellation quotes |
| `PublishSale` | Publish a sale: copy layout into Booking `seats` (mechanism: [AMB-13](../10-planning/ambiguities.md#amb-13)) |

## Events consumed

| Topic | Events | Use |
|---|---|---|
| `booking.inventory` | `InventoryChanged` (seat or bucket deltas), `InventoryReturned`, `SoldOut` | Rebuild/refresh seat-map snapshots |

Consumers use `processed_events` and ignore stale aggregate versions (p.24).

## Performance targets

- Seat-map read **p99 < 100 ms** (NFR-LAT).
- ~**2,000 seat-map reads/s** at the reference sale (p.9).

## Failure behaviour

| Failure | Behaviour | Source |
|---|---|---|
| Redis down | Seat map **falls back to throttled DB reads**. Which DB, and the throttle values, are unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)) | p.33 |
| Kafka down | Snapshots go stale until events flow again. Booking's outbox buffers the events | p.33 |

## Open items

[AMB-09](../10-planning/ambiguities.md#amb-09) (price tiers) · [AMB-10](../10-planning/ambiguities.md#amb-10) (per-user limit) · [AMB-13](../10-planning/ambiguities.md#amb-13) (schema, publish mechanism) · [AMB-15](../10-planning/ambiguities.md#amb-15) (pause/resume) · [OQ-1](../10-planning/open-questions.md) (refund-policy defaults)
