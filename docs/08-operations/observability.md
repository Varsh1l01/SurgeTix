# Observability

*Source: PDF p.34 (Observability and load testing → Instrumentation), p.8 (observability NFR), p.15 (observability stack), p.7 (efficiency NFR).*

> *Every claim on the resume comes from one comparison: the same k6 scenarios run against the naive baseline and against SurgeTix, with the Grafana "Surge Control Room" recording both.* (p.34)

## Stack (p.15)

| Concern | Tool |
|---|---|
| Metrics | **Prometheus** |
| Dashboards | **Grafana**, incl. the **Surge Control Room** |
| Traces | **OpenTelemetry SDK → OTel Collector → Tempo** (Jaeger acceptable instead of Tempo) |
| Logs | **`slog` JSON**; **Loki optional** |
| Profiling | **pprof** |

Rejected: ELK (heavy on memory), Datadog/New Relic (paid).

**Current state (phase 2, 2026-11-06):** every service serves Prometheus-format **`GET /metrics`** (Go runtime and process CPU/memory, `internal/platform/metrics`). Application routes get RED metrics (`http_requests_total{route,method,code}`, `http_request_duration_seconds{route,method}`), and Booking exports pgx pool statistics (`pgxpool_*`, including acquire wait). The naive baseline exports `holds_total`, `seats_remaining` and `double_booked_seats` with `build="unsafe_baseline"`. **Prometheus, Grafana, tracing and Loki are not deployed** until phase 8 ([ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)).

## Metrics

### RED per service
Rate, Errors, Duration for every service and route (NFR-OBS).

### Domain metrics (named in the PDF)

| Metric | Meaning |
|---|---|
| `queue_tickets_issued` | Queue joins issued |
| `admitted_upto` | Current admission counter |
| `admission_rate` | AIMD rate |
| `seats_remaining` | Inventory left |
| `holds_total{mode,result}` | Holds by mode (seats / best-available / GA) and result |
| `refunds_total{reason,status}` | Refunds by reason and status |
| `second_chance_offers_total{result}` | Offers by outcome |
| `double_booked_seats` | Audit query; **must stay 0** |
| `oversold_ga` | Audit query; **must stay 0** |

### Infrastructure metrics (named in the PDF)

DB pool wait · Redis hit ratio · edge cache hit ratio · Kafka lag · outbox backlog · **saga steps stuck > 5 min**.

DB pool wait and booking p99 are also **inputs to the AIMD admission controller** ([flow 1](../06-flows/01-queue-join-and-admission.md)).

### Efficiency metrics (p.34)

| Metric | Target / use |
|---|---|
| **Postgres round trips per seat sold** | ≤ 4 (hold through confirmation) |
| **Origin requests per 1,000 queue polls** | Shows edge-cache effectiveness (≥ 95 % cache share) |
| **CPU-seconds and allocations per booking** (pprof) | Hot-path tuning; PGO |
| **Estimated cloud cost per 1,000 tickets sold** | Efficiency headline. The cost model is unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)) |

## Traces

**One trace per request**: gateway → service → Postgres/Redis → Kafka → notifier, **including each saga step** (p.34). Trace context must therefore propagate across:
- gRPC metadata;
- the outbox row → Kafka headers → consumer. **Derived:** the outbox schema has no header column, so the trace context goes in `payload` or a new column ([AMB-12](../10-planning/ambiguities.md#amb-12)).

## Logs

`slog` JSON with **`trace_id`, `user_id`, `event_id`, `order_id`** (p.34).

## Alerts named in the PDF

| Condition | Source |
|---|---|
| Order stuck in `PAID_PENDING_CONFIRMATION`, Booking unreachable **> 15 min** | p.30 |
| Refund saga failing **> 1 hour** | p.30, p.33 |
| Saga steps stuck **> 5 min** (metric) | p.34 |
| Nightly audit mismatch → **page the owner** | p.34 |
| `double_booked_seats` / `oversold_ga` > 0 | p.34 |

The alert routing and paging tool are not specified ([AMB-17](../10-planning/ambiguities.md#amb-17)).

## Surge Control Room (Grafana)

- Records both the baseline and the SurgeTix runs side by side (p.34).
- Covers the FR-20 control-room data: **queue length, admission rate, seats left, errors**.
- The **pause, resume and kill switch** controls live in the admin API, and the PDF also lists an "admin control room" in the React app (p.13). How the two relate is [AMB-35](../10-planning/ambiguities.md#amb-35).
