# Tasks

*Backlog organised by build phase (PDF p.36). Each phase's tasks are derived **only** from that phase's scope text, its gate, and the requirements and flows it implements. Tasks that depend on an unresolved item name it.*

**Rules**
- Work only on the **current** phase. Do not start phase *n+1* until phase *n*'s gate is green in [PROGRESS.md](PROGRESS.md).
- A task blocked by an `AMB-xx` / `OQ-x` stays blocked until the item is approved.
- Task IDs (`P<phase>-<n>`) are stable.

Legend: ☐ to do · ☑ done · ⛔ blocked

---

## Phase 0 — Documentation (current)

| ID | Task | Status |
|---|---|---|
| P0-1 | Read and analyse the source PDF (v2), including diagrams | ☑ |
| P0-2 | Create the `docs/` structure and all section documents | ☑ |
| P0-3 | Create `CLAUDE.md`, `README.md`, `PROGRESS.md`, `ARCHITECTURE_DECISIONS.md`, `TASKS.md` | ☑ |
| P0-4 | Validate links, anchors, citations and coverage; write `DOCUMENTATION_AUDIT.md` | ☑ |
| P0-4a | Final pre-commit review: Mermaid parse, DDL diff, entity cross-check, secrets/.gitignore check; create `ARCHITECTURE_REVIEW.md` | ☑ |
| P0-5 | **Owner:** review the doc set | ☑ (approved 2026-11-06) |
| P0-6 | **Owner:** decide the ambiguities in the [register](docs/10-planning/ambiguities.md), at least those blocking phases 1–3 (prioritised in [ARCHITECTURE_REVIEW.md](ARCHITECTURE_REVIEW.md) §2) | ☐ |
| P0-7 | **Owner:** decide OQ-3 and OQ-7 | ☐ |
| P0-8 | Propagate approved decisions into docs and ADRs; log them in PROGRESS | ☐ (after P0-5…7) |
| P0-9 | **Owner:** decide AMB-01 (PDF file name) and approve the initial documentation commit | ☐ |

**Gate:** owner approval recorded in PROGRESS.md.

---

## Phase 1 — Foundations (Week 1) · Gate: **CI green**

| ID | Task | Depends on |
|---|---|---|
| P1-1 | Decide and create the repository/module layout | ☑ ADR-033 |
| P1-2 | Service skeletons (empty binaries with health endpoints only) | ☑ 7 binaries, `/healthz` + `/readyz`, graceful shutdown |
| P1-3 | Docker Compose with PostgreSQL 17, Redis 7, Kafka (KRaft, 1 broker) | ☑ written; `config` validated; **`up` not yet run** (local WSL failure, CI job pending). Single Redis node locally; the Kubernetes topology question ([AMB-20](docs/10-planning/ambiguities.md#amb-20)) stays open |
| P1-4 | Migration tooling (`goose`) and data-access tooling (`pgx`/`sqlc`) wired | ◐ `pgx` pool wired. `goose` and `sqlc` **deferred to phase 3**: there is no schema or query in phase 1, so wiring them now would be empty. `sqlc` still needs [OQ-3](docs/10-planning/open-questions.md) / [AMB-36](docs/10-planning/ambiguities.md#amb-36) |
| P1-5 | GitHub Actions CI: build, `go test -race`, lint, `buf` lint/breaking | ☑ gofmt, `go mod tidy` check, vet (both tags), `-race` tests, build, integration and compose jobs. **First run pending push.** `buf` deferred to phase 4 (no `.proto` files yet) |
| P1-6 | `testcontainers-go` harness for Postgres/Redis/Kafka integration tests | ☑ written and compiled (`-tags=integration`); **not yet run** (needs Docker; runs in the CI `integration` job) |
| P1-7 | **Owner:** push, confirm all three CI jobs green, review Phase 1 | ☐ |

---

## Phase 2 — Naive baseline (Week 2) · Gate: **oversell seen**

| ID | Task | Depends on |
|---|---|---|
| P2-1 | Decide the baseline shape (monolith vs services without waiting room/atomic holds) | ⛔ [OQ-7](docs/10-planning/open-questions.md) |
| P2-2 | Implement check-then-book (deliberately non-atomic) | P2-1 |
| P2-3 | k6 baseline scenarios, kept identical for T6 later | P2-2 |
| P2-4 | Audit queries `double_booked_seats` / `oversold_ga` | — |
| P2-5 | Run, record and archive the baseline oversell evidence | P2-3, P2-4 |

---

## Phase 3 — Booking core (Weeks 3–4) · Gate: **T1, T7, T8 pass**

| ID | Task | Ref |
|---|---|---|
| P3-1 | `booking_db` schema exactly as in [postgres-schema](docs/04-data/postgres-schema.md#booking_db), plus `idempotency_keys`, `outbox`, `processed_events` | ADR-003 |
| P3-2 | Idempotency claim (completed / in-progress 409 / different-body 422) | FR-12 |
| P3-3 | `HoldSeats`: all-or-nothing conditional update + inline reclaim of expired holds | FR-6, flow 2a |
| P3-4 | `HoldBestAvailable`: `SKIP LOCKED LIMIT 40` + adjacency | FR-7; [AMB-09](docs/10-planning/ambiguities.md#amb-09), [AMB-26](docs/10-planning/ambiguities.md#amb-26) |
| P3-5 | `HoldGA`: 32 buckets, multi-bucket allocation, ordered-lock retry | FR-8 |
| P3-6 | Hold limits | ⛔ [AMB-10](docs/10-planning/ambiguities.md#amb-10) |
| P3-7 | Expiry worker (2 s, batches of 1,000, `SKIP LOCKED`, `released_at`) | FR-9, flow 4 |
| P3-8 | `ReleaseHold` | FR-23; [AMB-27](docs/10-planning/ambiguities.md#amb-27) |
| P3-9 | `seatbits` bitmap pre-filter (reject-only) | flow 2 step 3 |
| P3-10 | Outbox writes for HoldCreated / InventoryChanged / HoldExpired / InventoryReturned | ADR-009 |
| P3-11 | T1, T7, T8 k6 scenarios + audit after every run | test plan |

---

## Phase 4 — Services and gRPC (Week 5) · Gate: **e2e hold works**

| ID | Task | Ref |
|---|---|---|
| P4-1 | Proto packages `identity.v1`, `catalog.v1`, `waitingroom.v1`, `booking.v1`, `checkout.v1` | [internal-grpc-api](docs/05-api/internal-grpc-api.md) |
| P4-2 | Identity: sign-up/login/refresh/verify (bcrypt), Google OAuth PKCE, RS256, JWKS | FR-1; [AMB-13](docs/10-planning/ambiguities.md#amb-13), [AMB-14](docs/10-planning/ambiguities.md#amb-14), [AMB-18](docs/10-planning/ambiguities.md#amb-18), [AMB-24](docs/10-planning/ambiguities.md#amb-24) |
| P4-3 | Catalog: events, venues, layouts, `PublishSale`, seat-map snapshots | FR-2, FR-3; [AMB-13](docs/10-planning/ambiguities.md#amb-13) |
| P4-4 | Gateway: routing REST → gRPC, JWT (cached JWKS), rate limits, request IDs, 1 s deadline | ADR-028 |
| P4-5 | Outbox relay → Kafka (`acks=all`) | ADR-009; [AMB-03](docs/10-planning/ambiguities.md#amb-03) |
| P4-6 | E2E test: sign in → (temporary admission bypass?) → hold | gate. How a hold is reached before the waiting room exists needs a decision |

---

## Phase 5 — Waiting room (Week 6) · Gate: **T2, T10, T13 pass**

| ID | Task | Ref |
|---|---|---|
| P5-1 | PoW challenge + verify + nonce burn | FR-18; [OQ-2](docs/10-planning/open-questions.md), [AMB-29](docs/10-planning/ambiguities.md#amb-29) |
| P5-2 | Join: `SET NX`, pre-sale shuffle, `INCR`; signed queue ticket | FR-4; [AMB-22](docs/10-planning/ambiguities.md#amb-22) |
| P5-3 | Public cached `queue/state` + NGINX 1 s microcache + Gateway fallback | ADR-012, ADR-027 |
| P5-4 | Admission leader: epoch/lease Lua scripts | ADR-013 |
| P5-5 | AIMD rate from booking p99 and DB pool wait | ADR-014; [AMB-12](docs/10-planning/ambiguities.md#amb-12) |
| P5-6 | Admit → 15-min access token; Gateway verifies on booking routes | FR-5 |
| P5-7 | Per-IP / per-account limits; `429` + `Retry-After` shedding | FR-18 |
| P5-8 | T2 (incl. Redis saturation benchmark), T10, T13 | test plan |

---

## Phase 6 — Checkout saga (Weeks 7–8) · Gate: **T3, T5, T12 pass**

| ID | Task | Ref |
|---|---|---|
| P6-1 | FakePay simulator (intents, page, signed webhooks, refunds, fault injection) | ADR-023; ⛔ [AMB-31](docs/10-planning/ambiguities.md#amb-31) |
| P6-2 | `checkout_db` schema | [postgres-schema](docs/04-data/postgres-schema.md#checkout_db) |
| P6-3 | `StartCheckout` → `StartPayment` → payment intent | flow 3; [AMB-11](docs/10-planning/ambiguities.md#amb-11) |
| P6-4 | Webhook: HMAC verify; one tx payment + order + saga step | ADR-017; [AMB-19](docs/10-planning/ambiguities.md#amb-19) |
| P6-5 | `ConfirmHold` (idempotent), ticket issuance with encrypted QR secret | FR-10, FR-11 |
| P6-6 | Saga executor: durable steps, `UNKNOWN` retry, optimistic concurrency | ADR-010; [AMB-25](docs/10-planning/ambiguities.md#amb-25), [AMB-33](docs/10-planning/ambiguities.md#amb-33) |
| P6-7 | Reconcilers (> 1 min confirm; > 2 min FakePay poll); alerts | flow 3 |
| P6-8 | `CONFIRM_REJECTED` refund path | flow 3; [AMB-04](docs/10-planning/ambiguities.md#amb-04) |
| P6-9 | Circuit breaker on FakePay | F15 |
| P6-10 | Notifier: confirmation email via Mailpit | FR-21 |
| P6-11 | T3, T5, T12 | test plan |

---

## Phase 7 — Lifecycle features (Weeks 9–10) · Gate: **T9, T11 pass**

| ID | Task | Ref |
|---|---|---|
| P7-1 | Cancellation quote + refund policy | FR-13; ⛔ [OQ-1](docs/10-planning/open-questions.md), [AMB-32](docs/10-planning/ambiguities.md#amb-32), [AMB-23](docs/10-planning/ambiguities.md#amb-23) |
| P7-2 | Refund saga + `CancelTickets` | FR-14; ⛔ [AMB-04](docs/10-planning/ambiguities.md#amb-04), [AMB-06](docs/10-planning/ambiguities.md#amb-06) |
| P7-3 | Second-chance queue and offers | FR-15; ⛔ [AMB-07](docs/10-planning/ambiguities.md#amb-07), [OQ-4](docs/10-planning/open-questions.md) |
| P7-4 | Rotating QR endpoint | FR-11 |
| P7-5 | Transfer | FR-16; [AMB-30](docs/10-planning/ambiguities.md#amb-30) |
| P7-6 | Gate scan / check-in | FR-17; [AMB-14](docs/10-planning/ambiguities.md#amb-14) |
| P7-7 | Refund receipt email | FR-21 |
| P7-8 | T9, T11 | test plan |

---

## Phase 8 — UI and dashboards (Week 11) · Gate: **full demo works**

| ID | Task | Ref |
|---|---|---|
| P8-1 | React + TS buyer flow (queue, seat map, hold, pay, tickets, cancel, transfer) | — |
| P8-2 | Gate scanner page | FR-17 |
| P8-3 | Admin control room + pause/resume/kill | FR-20; ⛔ [AMB-15](docs/10-planning/ambiguities.md#amb-15), [AMB-35](docs/10-planning/ambiguities.md#amb-35) |
| P8-4 | SSE seat-map deltas | FR-19; ⛔ [AMB-08](docs/10-planning/ambiguities.md#amb-08) |
| P8-5 | Grafana *Surge Control Room*; Prometheus metrics; OTel traces → Tempo; slog | [observability](docs/08-operations/observability.md) |
| P8-6 | Nightly audit | FR-22; ⛔ [AMB-17](docs/10-planning/ambiguities.md#amb-17) |

---

## Phase 9 — Ship and measure (Week 12) · Gate: **README report**

| ID | Task | Ref |
|---|---|---|
| P9-1 | Kubernetes manifests (kind/k3d): CloudNativePG, Strimzi, Redis Sentinel, probes, HPA, PDBs, graceful shutdown | ADR-030; [AMB-20](docs/10-planning/ambiguities.md#amb-20) |
| P9-2 | T4 chaos | test plan |
| P9-3 | PGO builds; allocation checks; efficiency metrics | NFR-EFF |
| P9-4 | T6: re-run T1–T3 on the baseline with the identical k6 profile | test plan; [OQ-6](docs/10-planning/open-questions.md) |
| P9-5 | README load-test report (charts + exact setup) and demo video | PDF p.36; [OQ-5](docs/10-planning/open-questions.md) |
