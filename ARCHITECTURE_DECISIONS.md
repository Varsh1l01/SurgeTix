# Architecture Decision Records

*Source: `reference/SurgeTix Product Requirements & System Architecture (2).pdf`. Page numbers refer to that file.*

These ADRs **record decisions the PDF already makes**. None are new decisions. Status meanings:

- **Accepted (PDF)**: stated as the design in the PDF.
- **Proposed (PDF)**: the PDF suggests it but also lists it as open. Needs owner confirmation.
- **Superseded / Deprecated**: kept for history, never deleted.

New decisions (e.g. answers to [ambiguities](docs/10-planning/ambiguities.md) or [open questions](docs/10-planning/open-questions.md)) get the next free number and a date.

## Index

| ADR | Title | Status |
|---|---|---|
| [001](#adr-001) | Layered funnel: only an admitted, bounded stream reaches the booking core | Accepted (PDF) |
| [002](#adr-002) | Go 1.27 for all services, workers and FakePay | Accepted (PDF) |
| [003](#adr-003) | PostgreSQL 17 is the only source of truth for inventory and money | Accepted (PDF) |
| [004](#adr-004) | Redis 7 for queue, caches, fencing and rate limits; never the truth | Accepted (PDF) |
| [005](#adr-005) | Kafka (KRaft) + franz-go as the event log, `acks=all` | Accepted (PDF) |
| [006](#adr-006) | gRPC + Protobuf internally, `buf`, propagated deadlines | Accepted (PDF) |
| [007](#adr-007) | REST + SSE at the public edge | Accepted (PDF) |
| [008](#adr-008) | One database per service; no cross-service table reads | Accepted (PDF) |
| [009](#adr-009) | Transactional outbox for every event; idempotent consumers | Accepted (PDF) |
| [010](#adr-010) | Orchestrated sagas in Checkout with durable step state | Accepted (PDF) |
| [011](#adr-011) | Accept the per-event single-Redis-shard hot spot in v1 and measure it | Accepted (PDF) |
| [012](#adr-012) | One public cached admission counter; clients compute their own position | Accepted (PDF) |
| [013](#adr-013) | Epoch-fenced admission-controller lease in Lua | Accepted (PDF) |
| [014](#adr-014) | AIMD admission rate driven by booking p99 and DB pool wait | Accepted (PDF) |
| [015](#adr-015) | GA inventory split into 32 counter buckets | Accepted (PDF) |
| [016](#adr-016) | Best-available: `SKIP LOCKED` chooses, the conditional update decides | Accepted (PDF) |
| [017](#adr-017) | Payment durable before confirmation; never refund on a timeout | Accepted (PDF) |
| [018](#adr-018) | `Idempotency-Key` on every write; RFC 9457 errors | Accepted (PDF) |
| [019](#adr-019) | TOTP-style rotating QR + one-time conditional check-in | Accepted (PDF) |
| [020](#adr-020) | Bot defence by proof-of-work and limits, not ML | Accepted (PDF) |
| [021](#adr-021) | Six services + one worker; new features land inside existing services | Accepted (PDF) |
| [022](#adr-022) | Build the naive baseline first; prove the core before splitting | Accepted (PDF) |
| [023](#adr-023) | FakePay simulator instead of a real provider's test mode | Accepted (PDF) |
| [024](#adr-024) | `pgx` + `sqlc` (CRUD) + hand-written SQL (hot paths) + `goose` | **Proposed (PDF)** |
| [025](#adr-025) | Online-only gate scanning in v1 | Accepted (PDF) |
| [026](#adr-026) | Admissions fail closed when Redis is unavailable | Accepted (PDF) |
| [027](#adr-027) | NGINX `proxy_cache` 1 s microcache as the edge (CDN if hosted) | Accepted (PDF) |
| [028](#adr-028) | Hand-written Go `net/http` gateway (no Kong/Envoy/Traefik) | Accepted (PDF) |
| [029](#adr-029) | Test against real Postgres/Redis/Kafka (testcontainers), `-race`, fuzz | Accepted (PDF) |
| [030](#adr-030) | Compose locally; kind/k3d with CloudNativePG, Strimzi, Redis Sentinel; GitHub Actions | Accepted (PDF) |
| [031](#adr-031) | Canonical hold expiry on `holds`; seat expiry is a same-transaction mirror | Accepted (PDF) |
| [032](#adr-032) | Money as integer paise | Accepted (PDF) |
| [033](#adr-033) | Single-module Go monorepo; per-service Postgres roles | **Accepted (owner, 2026-11-06)** |
| [034](#adr-034) | Naive baseline: the same seven components without protections (OQ-7 → Option B) | **Accepted (owner, 2026-11-06)** |
| [035](#adr-035) | Phase 2 baseline implementation decisions (AMB-38 … AMB-43) | **Accepted (owner, 2026-11-06)** |

---

<a id="adr-001"></a>
## ADR-001 — Layered funnel
- **Status:** Accepted (PDF p.1, p.8, p.10)
- **Context:** Demand exceeds supply by 100×+. The hard load is queue polling, not booking.
- **Decision:** Cheap layers (CDN/edge, gateway rate limits, virtual waiting room) absorb the crowd. Only an admitted, bounded stream reaches Booking, the one strictly correct service.
- **Consequences:** Booking is sized to the admitted rate (target 1,000 holds/s), not the crowd. Queue status must be served from cache.

<a id="adr-002"></a>
## ADR-002 — Go 1.27
- **Status:** Accepted (PDF p.13)
- **Decision:** All services, workers and FakePay in Go 1.27, built with PGO and allocation-checked hot paths (p.7).
- **Alternatives rejected:** Java/Spring (heavier, slower startup); Node (one thread for CPU work such as JWT signing and PoW checks); Rust (slow to build with while learning).

<a id="adr-003"></a>
## ADR-003 — PostgreSQL 17 is the only source of truth
- **Status:** Accepted (PDF p.14, p.17, p.31)
- **Decision:** Seats, GA buckets, holds, bookings, tickets, orders, payments, refunds, sagas, idempotency and outbox live in PostgreSQL. Every state change is a conditional update; `CHECK` and (partial) unique constraints are the final guards.
- **Alternatives rejected:** MySQL (no partial indexes); MongoDB (document transactions heavier for multi-seat updates); Cassandra (no multi-row transactions); CockroachDB/Spanner (distributed-SQL latency not needed for 50K seats); GORM (hides the SQL needed for locking).

<a id="adr-004"></a>
## ADR-004 — Redis 7: fast, never the truth
- **Status:** Accepted (PDF p.14, p.23–24)
- **Decision:** Redis (go-redis, Lua) holds queue tickets and counters, the admission lease and epoch, PoW nonces, seat bitmaps, seat-map snapshots and rate limits. It may **reject** early (bitmap) but never **grant** inventory.
- **Alternatives rejected:** Memcached (no scripts or sorted sets); Postgres for every poll (tens of thousands of QPS); etcd for the lease (a whole cluster for one key). Valkey is an acceptable drop-in.

<a id="adr-005"></a>
## ADR-005 — Kafka (KRaft) + franz-go
- **Status:** Accepted (PDF p.8–9, p.14, p.24)
- **Decision:** Durable, replayable event log with per-key ordering and many consumer groups. `acks=all`; 1 broker locally, 3 on Kubernetes. Topics are keyed by the entity whose order matters.
- **Alternatives rejected:** RabbitMQ (no replay); NATS JetStream (less recognised); Redis Streams (adds load to the busy Redis); SQS/SNS (cloud-only).

<a id="adr-006"></a>
## ADR-006 — gRPC + Protobuf internally
- **Status:** Accepted (PDF p.13, p.27)
- **Decision:** One proto package per service, `buf` lint and breaking-change checks. The Gateway gives each request a 1 s budget and internal calls inherit the remainder. Only `UNAVAILABLE` is retried, with jittered backoff.
- **Alternatives rejected:** REST/JSON (no contracts, no deadline propagation); GraphQL (built for clients); Connect-RPC (fine but less recognised).

<a id="adr-007"></a>
## ADR-007 — REST + SSE at the edge
- **Status:** Accepted (PDF p.15, p.24)
- **Decision:** Browsers use REST via the Gateway, plus SSE for one-way live updates (seat-map deltas, second-chance offers).
- **Alternatives rejected:** WebSockets (two-way not needed, more connection state); polling (wasteful at this scale).

<a id="adr-008"></a>
## ADR-008 — Database per service
- **Status:** Accepted (PDF p.17)
- **Decision:** Each service owns its schema (separate databases on one instance locally). No service reads another's tables; cross-service needs go through gRPC or Kafka.
- **Consequences:** Catalog's layout is copied into Booking on publish. Cross-database reads for the nightly audit are unresolved ([AMB-17](docs/10-planning/ambiguities.md#amb-17)).

<a id="adr-009"></a>
## ADR-009 — Transactional outbox; idempotent consumers
- **Status:** Accepted (PDF p.8, p.21–22, p.24)
- **Decision:** Every event is written to an `outbox` row in the same transaction as the state change and published by a relay. Consumers dedupe on `processed_events` and ignore stale aggregate versions. Business unique keys stop duplicates even if dedup is bypassed.
- **Consequences:** Kafka outages delay events but never lose them.

<a id="adr-010"></a>
## ADR-010 — Orchestrated sagas in Checkout
- **Status:** Accepted (PDF p.20, p.27, p.29–30)
- **Decision:** The checkout and refund sagas are orchestrated by Checkout over gRPC (calling Booking directly, not via Kafka), with durable `sagas` / `saga_steps` state and optimistic concurrency (`version`). Every step is safe to retry; a crashed worker's saga is resumed by another replica.

<a id="adr-011"></a>
## ADR-011 — Accept the single-shard hot spot in v1
- **Status:** Accepted (PDF p.9–10, p.33)
- **Context:** The `{event}` hash tag pins one event's queue keys to one shard so Lua can update them atomically.
- **Decision:** Accept it and **measure it** (Redis CPU, commands/s, memory, p99, failover time; T2). Partitioned queues with globally coordinated admission are **deliberately out of v1** unless the measured ceiling is too low. When saturated, shed joins with `429` + `Retry-After`.

<a id="adr-012"></a>
## ADR-012 — Public cached admission counter
- **Status:** Accepted (PDF p.5, p.28)
- **Decision:** `GET /queue/state` returns `{admitted_upto, rate, sold_out}`, identical for all users, `Cache-Control: max-age=1`. Clients compute position (`number − admitted_upto`) and ETA (`position ÷ rate`). No per-user server work.
- **Consequences:** ≥ 95 % of polls served from cache; status p99 < 50 ms.

<a id="adr-013"></a>
## ADR-013 — Epoch-fenced admission lease
- **Status:** Accepted (PDF p.28, p.32)
- **Decision:** Leadership: a Lua script increments `wr:{event}:epoch` and sets `wr:{event}:lease = owner:epoch` (TTL 5 s) only if free. Each tick: a Lua script checks the lease, raises `admitted` to `max(current, new)` and renews, atomically. A stale leader can never write again (T13).
- **Alternatives rejected:** etcd (see ADR-004).

<a id="adr-014"></a>
## ADR-014 — AIMD admission
- **Status:** Accepted (PDF p.9, p.28)
- **Decision:** Start at 500/s; additive increase while booking p99 and DB pool wait are healthy; halve when not; drop to 0 if the Postgres primary fails. Parameters are open ([AMB-12](docs/10-planning/ambiguities.md#amb-12)).

<a id="adr-015"></a>
## ADR-015 — 32 GA buckets
- **Status:** Accepted (PDF p.9, p.17, p.29, p.32)
- **Decision:** Each GA section is spread over 32 `ga_buckets` rows. A hold takes from several buckets (`SKIP LOCKED`, random start), with one retry using ordered `FOR UPDATE` (deadlock-free). Allocations are recorded per bucket and released once.
- **Consequences:** No single hot row at 1,000 GA holds/s. Lock wait with 32 vs 1 buckets is measured in T7.

<a id="adr-016"></a>
## ADR-016 — Best-available via `SKIP LOCKED`
- **Status:** Accepted (PDF p.5, p.28, p.31)
- **Decision:** Select up to 40 free candidates ordered by row and seat `FOR UPDATE SKIP LOCKED`, pick the first adjacent run of N in memory, then apply the same all-or-nothing conditional update. `SKIP LOCKED` only chooses; the update decides.
- **Consequences:** Target conflict rate < 5 % (T8).

<a id="adr-017"></a>
## ADR-017 — Payment durable before confirm; never refund on a timeout
- **Status:** Accepted (PDF p.6, p.29–30, p.32)
- **Decision:** On a signed webhook, in one transaction: payment `SUCCEEDED`, order `PAID_PENDING_CONFIRMATION`, saga step `CONFIRM_HOLD` pending. Then `ConfirmHold` (idempotent by `order_id`). CONFIRMED → confirm. **Definitive REJECTED only** → refund (`CONFIRM_REJECTED`). Timeout / `UNAVAILABLE` / unknown → step `UNKNOWN`, retry, reconcile via `GetBookingByOrder`.
- **Consequences:** 0 wrong refunds; no paid buyer without a ticket or refund (T12, nightly audit).

<a id="adr-018"></a>
## ADR-018 — Idempotency keys and RFC 9457
- **Status:** Accepted (PDF p.6, p.21, p.24, p.28)
- **Decision:** Every state-changing endpoint accepts `Idempotency-Key`, stored in `idempotency_keys (scope, user_id, key)`. Completed → stored response; in progress → `409`; same key with a different body → `422`. Errors are RFC 9457 problem details.

<a id="adr-019"></a>
## ADR-019 — Rotating QR and one-time check-in
- **Status:** Accepted (PDF p.6, p.31, p.34)
- **Decision:** QR = HMAC(per-ticket secret, 30-second window). Scan accepts the current and previous window, then `UPDATE tickets … WHERE status = 'VALID'`; first scan wins. Secrets are stored encrypted and never sent in plain form; transfer rotates the secret.
- **Consequences:** Screenshots go stale in ~1 minute. Live sharing is **reduced, not prevented** (explicitly acknowledged).

<a id="adr-020"></a>
## ADR-020 — Proof-of-work and limits, not ML
- **Status:** Accepted (PDF p.2, p.6, p.27)
- **Decision:** ~1 s client PoW on queue join, nonce burn, one queue entry per verified account, per-account and per-IP limits, purchase caps, transfer limits. ML bot detection is a non-goal.

<a id="adr-021"></a>
## ADR-021 — Six services + one worker
- **Status:** Accepted (PDF p.15–16)
- **Decision:** Gateway, Identity, Catalog, Waiting Room, Booking, Checkout, plus the Notifier worker; FakePay as a simulator. Each split has a stated reason (scaling, failure domain or data ownership). **Lifecycle features land inside existing services rather than adding more boxes.**

<a id="adr-022"></a>
## ADR-022 — Naive baseline first; correctness before splitting
- **Status:** Accepted (PDF p.36–37)
- **Decision:** Phase 2 builds a check-then-book baseline (gate: oversell seen). Phase 3 proves the booking core (T1, T7, T8) **before** services are split (phase 4). A phase moves on only when its gate passes; the timeline stretches rather than skipping correctness work.
- **Baseline shape:** decided 2026-11-06 as the same seven components without protections ([ADR-034](#adr-034), resolves [OQ-7](docs/10-planning/open-questions.md)).

<a id="adr-023"></a>
## ADR-023 — FakePay simulator
- **Status:** Accepted (PDF p.1–2, p.14)
- **Decision:** An in-house Go simulator with intents, signed webhooks, refunds and idempotency keys, plus latency, failures and lost webhooks on demand.
- **Alternatives rejected:** Stripe/Razorpay test mode (rate-limited, cannot fail on command). It can be swapped in later **behind the same interface**, so Checkout must depend on a provider interface.

<a id="adr-024"></a>
## ADR-024 — Postgres access layer
- **Status:** **Proposed (PDF)**. Awaiting confirmation ([OQ-3](docs/10-planning/open-questions.md), [AMB-36](docs/10-planning/ambiguities.md#amb-36))
- **Proposal:** `pgx` driver; `sqlc` for CRUD; hand-written SQL for the hold, GA and refund paths; `goose` migrations (p.14, p.37).

<a id="adr-025"></a>
## ADR-025 — Online-only gate scanning
- **Status:** Accepted (PDF p.4, p.6, p.31, p.34)
- **Decision:** Gates require venue connectivity. Offline scanning is out of scope for v1 and documented as a known limitation.

<a id="adr-026"></a>
## ADR-026 — Fail closed on Redis loss
- **Status:** Accepted (PDF p.33)
- **Decision:** If Redis is down, admissions pause. Already-admitted users continue on signed tokens; seat maps fall back to throttled DB reads; holds skip the bitmap pre-filter.

<a id="adr-027"></a>
## ADR-027 — NGINX microcache at the edge
- **Status:** Accepted (PDF p.13, p.33)
- **Decision:** NGINX `proxy_cache` with a 1 s microcache for `GET /queue/state` and static assets; a CDN such as Cloudflare if hosted. The Gateway keeps an in-process 1 s cache only as a fallback when the edge is down.
- **Alternatives rejected:** Varnish (equivalent, one more tool); a per-pod Go cache only (origin load grows with pod count).

<a id="adr-028"></a>
## ADR-028 — Hand-written Go gateway
- **Status:** Accepted (PDF p.13)
- **Decision:** Go `net/http` with pattern routing: JWT and access-token checks, rate limits, REST → gRPC.
- **Alternatives rejected:** Kong, Envoy, Traefik, named as *"the production swap"*.

<a id="adr-029"></a>
## ADR-029 — Real-dependency testing
- **Status:** Accepted (PDF p.15)
- **Decision:** `go test -race`, `testcontainers-go` with real Postgres, Redis and Kafka, table-driven and fuzz tests in every package. k6 (arrival-rate) for load; Toxiproxy, `kubectl delete pod` and `SIGSTOP` for faults.
- **Alternatives rejected:** mocks only (miss SQL and lock behaviour); JMeter, Locust, Gatling; Chaos Mesh (heavy for v1).

<a id="adr-030"></a>
## ADR-030 — Run and deploy
- **Status:** Accepted (PDF p.8, p.15)
- **Decision:** Docker Compose for the one-command local run. Kubernetes (kind or k3d) with CloudNativePG, Strimzi and Redis Sentinel for the HA demo: ≥ 2 replicas per stateless service, probes, HPA, PDBs, graceful shutdown. GitHub Actions for CI. Mailpit for email; Prometheus, Grafana, OTel Collector and Tempo (Loki optional) for observability.
- **Alternatives rejected:** Nomad/ECS (less common); managed cloud databases (cost).
- **Open:** Sentinel vs Cluster wording, kind vs k3d ([AMB-20](docs/10-planning/ambiguities.md#amb-20)).

<a id="adr-031"></a>
## ADR-031 — Canonical hold expiry
- **Status:** Accepted (PDF p.17–18, p.21)
- **Decision:** `holds.expires_at` is canonical. `seats.hold_expires_at` is a mirror written only in the same transaction (create, extend at checkout, release, expire).

<a id="adr-032"></a>
## ADR-032 — Money as integer paise
- **Status:** Accepted (PDF p.17–20)
- **Decision:** All amounts are `BIGINT` paise with `CHECK` bounds (`amount_paise > 0`, `refunded_paise BETWEEN 0 AND amount_paise`). No floating-point money anywhere.

<a id="adr-033"></a>
## ADR-033 — Single-module Go monorepo; per-service Postgres roles
- **Status:** Accepted (owner, 2026-11-06). Resolves [AMB-34](docs/10-planning/ambiguities.md#amb-34) (layout part)
- **Context:** The PDF specifies the services but no repository structure. Phase 1 must create one.
- **Decision:**
  - One Go module, `github.com/Varsh1l01/SurgeTix`, Go 1.27.
  - `cmd/<service>/main.go` for each of the 7 binaries (ADR-021).
  - `internal/platform/` holds shared infrastructure only (config, logging, health/shutdown, Postgres/Redis/Kafka clients). Business code goes in `internal/<service>/`, added as each phase needs it.
  - Each Postgres-owning service gets its own database **and its own login role**. The role owns only its database, and `PUBLIC` has no `CONNECT` on service databases. ADR-008 is therefore enforced by Postgres, not only by convention. Integration-tested in `internal/platform/postgres`.
- **Alternatives rejected:** one module per service with `go.work` (more version and CI overhead, with no isolation gain that Go's `internal/` rule and the DB roles don't already give).
- **Consequences:** One CI pipeline and one `go.mod`. Service boundaries are kept by package structure and DB roles; a later lint rule may forbid `internal/<a>` importing `internal/<b>`.

<a id="adr-034"></a>
## ADR-034 — Naive baseline: the same seven components without protections
- **Status:** Accepted (owner, 2026-11-06). Resolves [OQ-7](docs/10-planning/open-questions.md) with **Option B**. Refines [ADR-022](#adr-022).
- **Context:** Phase 2 must build a check-then-book baseline whose gate is *oversell seen* (p.36). Every SurgeTix claim is measured against it under the same k6 profile (T6, p.34). OQ-7 asked whether it should be a monolith (simplest) or the same services without the waiting room and atomic holds (fairer comparison).
- **Decision:**
  - The baseline keeps the **seven components** of Phase 1 (Gateway, Identity, Catalog, Waiting Room, Booking, Checkout, Notifier; ADR-021) and the same infrastructure (PostgreSQL, Redis, Kafka). No component is removed or rewritten.
  - **Deliberately absent** from the baseline path: waiting-room admission and queue protection, atomic seat reservation, atomic check-and-book, conditional `UPDATE … WHERE status = $from` with a rows-affected check, row or advisory locks, `REPEATABLE READ`/`SERIALIZABLE` isolation, uniqueness constraints that would block a double booking, idempotency keys, and later-phase booking optimisations.
  - Phase 2 implements the **booking and inventory path only**: client → Gateway → Booking → PostgreSQL → response. The baseline is **for testing and benchmarking only** and is never a production booking flow.
- **Scope of the invariant exemption.** CLAUDE.md invariants **1, 2, 3, 4 and 7** are intentionally not met **inside the baseline path only**. In exchange:
  - the baseline uses **its own tables**, never the real `booking_db` schema, so invariant 4's constraints are never weakened in SurgeTix itself;
  - the unsafe path is **isolated** and **off by default**. The mechanism is open ([AMB-41](docs/10-planning/ambiguities.md#amb-41));
  - invariants **8** (it publishes no Kafka events), **11** (no cross-service table reads), **12** (parameterised SQL) and **14** (integer paise) still apply.
- **Derived constraint (for credibility, R-12):** the baseline is unprotected, not sabotaged. It adds no artificial delays on the request path, and it uses the same pool sizes, Go version and infrastructure as SurgeTix. The checkpoint used by the deterministic tests is `nil` outside tests.
- **Alternatives rejected:** a single monolith (Option A). It is simpler, but the comparison would mix the effect of the service split with the effect of the protections.
- **Consequences:** The T6 comparison isolates the effect of the protections. The baseline must run through the same service topology as SurgeTix when it is measured, so it is kept frozen after Phase 2 and re-run in phase 9. Open details are tracked as [AMB-38](docs/10-planning/ambiguities.md#amb-38) … [AMB-43](docs/10-planning/ambiguities.md#amb-43). Plan: [phase-2-naive-baseline](docs/10-planning/phase-2-naive-baseline.md).

<a id="adr-035"></a>
## ADR-035 — Phase 2 baseline implementation decisions
- **Status:** Accepted (owner, 2026-11-06). Resolves [AMB-38](docs/10-planning/ambiguities.md#amb-38) … [AMB-43](docs/10-planning/ambiguities.md#amb-43). Implements [ADR-034](#adr-034).
- **Decision (owner):**
  - **Data (AMB-38):** reserved seats only. Separate baseline tables in `booking_db`, in the Postgres schema `baseline`. `goose` migrations with their own version table. A deterministic seed of 1 event and 500 seats. The real Booking schema is untouched.
  - **Contract (AMB-39):** the hold API's JSON shape on the test-only route `POST /v1/baseline/events/{eventID}/holds`. `X-Baseline-Buyer-ID` carries the identity. No idempotency. Every benchmark request is unique.
  - **Transport (AMB-40):** internal HTTP from Gateway to Booking. gRPC stays in phase 4.
  - **Isolation (AMB-41):** build tag `unsafe_baseline` **and** `SURGETIX_ENABLE_UNSAFE_BASELINE=true`, both required, default off. The guard is proven by tests.
  - **Observability (AMB-42):** a Prometheus-format `/metrics` endpoint. k6 and the SQL audit are the primary evidence. No Prometheus or Grafana until phase 8.
  - **Load (AMB-43):** the T1-shaped scenario with 50/100/250 buyers (500 if stable), 3 runs each plus a warm-up. 5,000 buyers is a future distributed run. T6 waits for its dependencies.
  - **Fairness:** unprotected but not sabotaged. No sleeps, reduced pools or resource caps. The environment is recorded with every run.
- **Implementation choices made under these decisions (derived, open to review):**
  - New dependencies: `github.com/pressly/goose/v3` and `github.com/prometheus/client_golang`.
  - Every process now serves `/metrics` (Go runtime and process CPU/memory). Booking also exports pgx pool statistics. Application routes get RED metrics.
  - **✅ Approved by the owner, 2026-11-06:** the `201` body keeps `{hold_id, seats, expires_at, amount}`. `hold_id` carries the **booking** id, `expires_at` is always `null` (a baseline booking never expires), and `amount` is integer paise. **This is a baseline-only limitation.** In SurgeTix the same shape describes a 10-minute hold. Errors use RFC 9457 with `type: about:blank` (AMB-12 still open).
  - The Gateway applies the PDF's 1 s deadline (504 on expiry, 502 if Booking is down) and keeps up to 1,024 idle connections to Booking. Go's default of 2 per host would throttle the baseline.
  - k6 uses `constant-arrival-rate` configured to saturate (an offered 20,000/s, capped at *B* VUs), so each run measures *B* concurrent buyers. Each request books 2 adjacent seats chosen deterministically.
  - **✅ Approved by the owner, 2026-11-06 (fail closed):** the flag set on a normal build is a configuration error and the process refuses to start. It is never silently ignored. Verified on the real binaries: exit 1 with the guard message.
- **Findings (measured 2026-11-06, details in the plan §8):** every run oversold (10–20 double-booked seats). Booking's 12-connection pool is the queue (≥ 99.8 % of acquires waited). At 250 buyers the Gateway's 1 s deadline produced 504s, and in one run 6 bookings committed after their client got a 504. The host→Docker-VM round trip measured 1.348 ms vs 0.154 ms inside the VM. The 500-buyer level was not run (machine stability).
- **Consequences:** Phase 2 evidence is reproducible from `loadtest/` and the committed SQL. Normal builds are unaffected and cannot serve the baseline. Plan and results: [phase-2-naive-baseline](docs/10-planning/phase-2-naive-baseline.md).
