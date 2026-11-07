# Tasks

*Backlog organised by build phase (PDF p.36). Each phase's tasks are derived **only** from that phase's scope text, its gate, and the requirements and flows it implements. Tasks that depend on an unresolved item name it.*

**Rules**
- Work only on the **current** phase. Do not start phase *n+1* until phase *n*'s gate is green in [PROGRESS.md](PROGRESS.md).
- A task blocked by an `AMB-xx` / `OQ-x` stays blocked until the item is approved.
- Task IDs (`P<phase>-<n>`) are stable.

Legend: ☐ to do · ☑ done · ⛔ blocked

---

## Phase 0 — Documentation (gate passed 2026-11-06)

| ID | Task | Status |
|---|---|---|
| P0-1 | Read and analyse the source PDF (v2), including diagrams | ☑ |
| P0-2 | Create the `docs/` structure and all section documents | ☑ |
| P0-3 | Create `CLAUDE.md`, `README.md`, `PROGRESS.md`, `ARCHITECTURE_DECISIONS.md`, `TASKS.md` | ☑ |
| P0-4 | Validate links, anchors, citations and coverage; write `DOCUMENTATION_AUDIT.md` | ☑ |
| P0-4a | Final pre-commit review: Mermaid parse, DDL diff, entity cross-check, secrets/.gitignore check; create `ARCHITECTURE_REVIEW.md` | ☑ |
| P0-5 | **Owner:** review the doc set | ☑ (approved 2026-11-06) |
| P0-6 | **Owner:** decide the ambiguities in the [register](docs/10-planning/ambiguities.md), at least those blocking phases 1–3 (prioritised in [ARCHITECTURE_REVIEW.md](ARCHITECTURE_REVIEW.md) §2) | ☐ |
| P0-7 | **Owner:** decide OQ-3 and OQ-7 | ◐ OQ-7 decided 2026-11-06 ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)); OQ-3 open |
| P0-8 | Propagate approved decisions into docs and ADRs; log them in PROGRESS | ☐ (after P0-5…7) |
| P0-9 | **Owner:** decide AMB-01 (PDF file name) and approve the initial documentation commit | ☐ |

**Gate:** owner approval recorded in PROGRESS.md.

---

## Phase 1 — Foundations (Week 1) · Gate: **CI green** · 🟢 passed 2026-11-06

| ID | Task | Depends on |
|---|---|---|
| P1-1 | Decide and create the repository/module layout | ☑ ADR-033 |
| P1-2 | Service skeletons (empty binaries with health endpoints only) | ☑ 7 binaries, `/healthz` + `/readyz`, graceful shutdown |
| P1-3 | Docker Compose with PostgreSQL 17, Redis 7, Kafka (KRaft, 1 broker) | ☑ `up --wait` healthy locally (host ports 15432 / 16379 / 29092) and in the CI `compose` job. Single Redis node locally; the Kubernetes topology question ([AMB-20](docs/10-planning/ambiguities.md#amb-20)) stays open |
| P1-4 | Migration tooling (`goose`) and data-access tooling (`pgx`/`sqlc`) wired | ◐ `pgx` pool wired. `goose` and `sqlc` **deferred to phase 3**: there is no schema or query in phase 1, so wiring them now would be empty. `sqlc` still needs [OQ-3](docs/10-planning/open-questions.md) / [AMB-36](docs/10-planning/ambiguities.md#amb-36) |
| P1-5 | GitHub Actions CI: build, `go test -race`, lint, `buf` lint/breaking | ☑ gofmt, `go mod tidy` check, vet (both tags), `-race` tests, build, integration and compose jobs. First run green (run 42148048247, PR #1). `buf` deferred to phase 4 (no `.proto` files yet) |
| P1-6 | `testcontainers-go` harness for Postgres/Redis/Kafka integration tests | ☑ passes locally (Ryuk disabled) and in the CI `integration` job with `-race` |
| P1-7 | **Owner:** push, confirm all three CI jobs green, review Phase 1 | ☑ CI green, PR #1 merged into `develop` 2026-11-06 |

---

## Phase 2 — Naive baseline (Week 2) · Gate: **oversell seen** · 🟢 passed 2026-11-06

Plan and results: [phase-2-naive-baseline](docs/10-planning/phase-2-naive-baseline.md) · Decisions: [ADR-034](ARCHITECTURE_DECISIONS.md#adr-034), [ADR-035](ARCHITECTURE_DECISIONS.md#adr-035) · Run: [`loadtest/README.md`](loadtest/README.md).

| ID | Task | Depends on |
|---|---|---|
| P2-1 | Decide the baseline shape (monolith vs services without waiting room/atomic holds) | ☑ Option B, 2026-11-06 ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)) |
| P2-2 | Implement check-then-book (deliberately non-atomic): naive repository with CHECK and BOOK SQL, explicit `READ COMMITTED`, nil-by-default checkpoint between them | ☑ `internal/booking/baseline/store.go` |
| P2-3 | k6 baseline scenarios (arrival-rate), kept identical for T6 later, plus the reset → run → audit wrapper | ☑ `loadtest/k6/baseline-t1.js`, `loadtest/baseline/` |
| P2-4 | Audit queries over the baseline tables: double-booked seats, excess bookings (and oversold GA if in scope) | ☑ `sql/audit.sql` (GA out of scope) |
| P2-5 | Run, record and archive the baseline oversell evidence | ☑ Plan §5 and §8. Raw output kept locally in `loadtest/results/`, not committed |
| P2-6 | **Owner:** approve the phase 2 plan and decide AMB-38 … AMB-43 | ☑ 2026-11-06 ([ADR-035](ARCHITECTURE_DECISIONS.md#adr-035)) |
| P2-7 | Baseline tables, seed and reset command (separate from the real schema) | ☑ goose migrations, schema `baseline`, `sql/reset.sql` |
| P2-8 | Deterministic concurrency tests on real Postgres (barrier): 2 on 1 seat, *k* on 1 seat, demand ≫ inventory, overlapping multi-seat, sequential control, isolation-level assertion; pass `-race` | ☑ pass locally and with `-race` in CI run 42159920841 |
| P2-9 | Booking HTTP endpoint for the baseline, behind the isolation guard (off by default) | ☑ incl. guard tests in both builds and the `go list -deps` check |
| P2-10 | Gateway routing to Booking, plus an end-to-end test against Compose | ☑ `internal/gateway/baseline`, `test/e2e`, and the benchmark runs through Compose |
| P2-11 | Baseline metrics: requests, outcomes, inventory, oversell, latency, errors | ☑ `/metrics` on all services (plan §7) |
| P2-12 | Benchmark matrix: inventory sizes × arrival rates, repeated runs, environment captured with each result | ☑ 50/100/250 buyers × 3 runs + warm-ups. 500 **not run** (plan §8.4, owner to decide) |
| P2-13 | Document architecture, method and **measured** results (plan §8); no estimated numbers | ☑ |
| P2-14 | Regression: Phase 1 checks, Compose health, `/healthz` + `/readyz` for all 7 services, CI green on the PR | ☑ local regression and CI run 42159920841 (all three jobs) |
| P2-15 | **Owner:** gate review: oversell seen + the 9 acceptance criteria | ☑ approved 2026-11-06 |

**Acceptance criteria (owner, 2026-11-06).** Phase 2 is complete only when: (1) the naive flow works end to end · (2) the seven-service architecture is intact · (3) a reproducible concurrency test exists · (4) the oversell is demonstrated and documented with test evidence · (5) the k6 load tests run · (6) latency, throughput and error-rate metrics are recorded · (7) the results are reproducible · (8) the docs contain the architecture, method and measured results · (9) no Phase 1 function is broken. The PDF gate is **oversell seen**.

---

## Phase 3 — Booking core · Gate: **T1, T7, T8 pass** · current (planning)

Plan: [phase-3-booking-core](docs/10-planning/phase-3-booking-core.md) · Decisions: [ADR-036](ARCHITECTURE_DECISIONS.md#adr-036). **No fixed timeline** (owner, 2026-11-06): one milestone at a time (M1 → M5), each implemented, tested, reviewed and **approved** before the next. **M4 (PDF completeness: tier mode, nearby alternatives, `seatbits`, `SoldOut`) is part of the Phase 3 completion criteria.** **No code until the plan is approved.**

| ID | Task | Milestone | Depends on |
|---|---|---|---|
| P3-1 | `booking_db` schema exactly as in [postgres-schema](docs/04-data/postgres-schema.md#booking_db), plus `idempotency_keys`, `outbox`, `processed_events`, and the approved additive 00004 (`claim_generation`, `claimed_at`); goose with a session lock ([AMB-51](docs/10-planning/ambiguities.md#amb-51)) | M1 (T1, T2) | [AMB-48](docs/10-planning/ambiguities.md#amb-48) ✅ |
| P3-2 | Idempotency claim (completed / in-progress 409 / different-body 422), 30 s lease, canonical hash, stored deterministic 409, 24 h retention, **generation fencing** | M1 (T5, T10) | [AMB-48](docs/10-planning/ambiguities.md#amb-48); T5 package approved 2026-11-06; T10 service layer approved 2026-11-06 (I1–I8); same key + different body → **422** confirmed by owner (the package returns `Mismatch`; M2 maps it) |
| P3-3 | `HoldSeats`: all-or-nothing conditional update + inline reclaim + **partial reclaim** ([AMB-50](docs/10-planning/ambiguities.md#amb-50)) | M1 (T8, T9) | P3-1, P3-2; T8 + T9 approved 2026-11-06 (`lock_timeout` 500 ms) |
| P3-4 | `HoldBestAvailable`: `SKIP LOCKED LIMIT 40` + adjacency, **section mode**; no run → `409` | M2 | [AMB-09](docs/10-planning/ambiguities.md#amb-09) ✅, [AMB-26](docs/10-planning/ambiguities.md#amb-26) ✅ |
| P3-5 | `HoldGA`: 32 buckets, multi-bucket allocation, ordered-lock retry | M3 | P3-1; ⛔ [AMB-53](docs/10-planning/ambiguities.md#amb-53) GA price source (hold `amount`) |
| P3-6 | Hold limits: **1–4 per hold, server-side, all three modes** (seats, best-available, GA); invalid quantities tested; cumulative caps deferred | M1 (T7) | [AMB-10](docs/10-planning/ambiguities.md#amb-10) ✅ (confirmed); T7 validation + canonical form approved 2026-11-06 (wired in T10); id limits 16/24 bytes and plain-integer quantity are Phase 3 constraints, revisit with Catalog layouts (phase 4) |
| P3-7 | Expiry worker (2 s, batches of 1,000, `SKIP LOCKED`, `released_at`), drained-only `EXPIRED` rule | M1 (T11) seats; M3 GA | P3-3; T11 seats worker and T12 wiring implemented, awaiting review |
| P3-8 | `ReleaseHold` (only `ACTIVE`, unexpired) | M2 | [AMB-27](docs/10-planning/ambiguities.md#amb-27) ✅ |
| P3-9 | `seatbits` pre-filter: **booked-seats-only**, never blocks a valid hold or reclaim, skipped when Redis is down; set-after-commit / clear-before-commit hooks for phases 6–7 | **M4** | [AMB-49](docs/10-planning/ambiguities.md#amb-49) ✅ |
| P3-10 | Outbox writes (HoldCreated / InventoryChanged / HoldReleased / HoldExpired / InventoryReturned) to `booking.holds` / `booking.inventory`, versioned JSON envelope | M1 (T6) | [AMB-47](docs/10-planning/ambiguities.md#amb-47) ✅, [AMB-03](docs/10-planning/ambiguities.md#amb-03) ✅; T6 insert helper approved 2026-11-06 (callers wired in T8+); aggregate version open ([AMB-54](docs/10-planning/ambiguities.md#amb-54)) |
| P3-11 | T1, T7, T8 k6 scenarios + audit after every run; full-scale T1/T7 on cloud VMs | **M5** | [AMB-46](docs/10-planning/ambiguities.md#amb-46) ✅; [OQ-6](docs/10-planning/open-questions.md) before the cloud runs (does not block M1–M4) |
| P3-12 | Step 0: record the Phase 2 gate | — | ☑ 2026-11-06 |
| P3-13 | Generated inventory (seats, even GA split, comparison / fragmented / 1-bucket layouts); bucket range 0–31 enforced in code; production `Load` rejects the benchmark-only one-bucket layout | M1 (T3) | [AMB-45](docs/10-planning/ambiguities.md#amb-45) ✅ |
| P3-14 | Internal HTTP API, phase-3 identity guard (never public), RFC 9457 errors, metrics, round-trip tracer | M2 | [AMB-44](docs/10-planning/ambiguities.md#amb-44) ✅, [AMB-28](docs/10-planning/ambiguities.md#amb-28) ✅ |
| P3-15 | Audit SQL for the real schema: 10 read-only checks (double-booked, orphans, mirror, `oversold_ga`, conservation/restore-once, FR-22 seat↔ticket); plan §13.3 | M1 (T4) | P3-1; implemented, awaiting review |
| P3-16 | Lock-order validation suite (L1–L6) and the baseline-frozen CI check | M1 (T9, T11, T12) | P3-3, P3-7; L1, L2 (and L3 = D4) done in T8/T9; L1 against the real worker in T11; L6 stress and the CI baseline-frozen step in T12 (CI run pending) |
| P3-17 | Comparison with the frozen Phase 2 baseline (same profile and machine), **k6 direct to Booking for both** | **M5** | P3-11 |
| P3-18 | Milestone reports, docs, regression, owner gate reviews (M1 … M5) | each | — |
| P3-19 | **Tier mode** for best-available (FR-7, **P0**) | **M4** | [AMB-09](docs/10-planning/ambiguities.md#amb-09): tier data model before M4 |
| P3-20 | **Nearby alternatives** in `409` (read-only, never held, stored with the `409`) | **M4** | [AMB-26](docs/10-planning/ambiguities.md#amb-26): algorithm before M4 |
| P3-21 | **`SoldOut`** on `booking.inventory` (key `event_id`) | **M4** | ⛔ [AMB-52](docs/10-planning/ambiguities.md#amb-52): emission condition |

## Phase 4 — Services and gRPC (Week 5) · Gate: **e2e hold works**

| ID | Task | Ref |
|---|---|---|
| P4-1 | Proto packages `identity.v1`, `catalog.v1`, `waitingroom.v1`, `booking.v1`, `checkout.v1` | [internal-grpc-api](docs/05-api/internal-grpc-api.md) |
| P4-2 | Identity: sign-up/login/refresh/verify (bcrypt), Google OAuth PKCE, RS256, JWKS | FR-1; [AMB-13](docs/10-planning/ambiguities.md#amb-13), [AMB-14](docs/10-planning/ambiguities.md#amb-14), [AMB-18](docs/10-planning/ambiguities.md#amb-18), [AMB-24](docs/10-planning/ambiguities.md#amb-24) |
| P4-3 | Catalog: events, venues, layouts, `PublishSale`, seat-map snapshots | FR-2, FR-3; [AMB-13](docs/10-planning/ambiguities.md#amb-13) |
| P4-4 | Gateway: routing REST → gRPC, JWT (cached JWKS), rate limits, request IDs, 1 s deadline | ADR-028 |
| P4-5 | Outbox relay → Kafka (`acks=all`) | ADR-009; [AMB-03](docs/10-planning/ambiguities.md#amb-03); ⛔ [AMB-54](docs/10-planning/ambiguities.md#amb-54) aggregate version |
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
