# Progress

*Tracks status against the 9-phase build plan (PDF p.36) plus phase 0 (documentation). A phase moves on **only when its gate passes** (PDF p.37).*

**Last updated:** 2026-11-06
**Current phase:** **3 — Booking core** → status: **Planning (plan awaiting owner approval; no Phase 3 code)** ([plan](docs/10-planning/phase-3-booking-core.md)). Phase 2 gate passed on 2026-11-06. Phase 1 gate passed on 2026-11-06

## Phase status

| # | Phase | Planned weeks | Gate | Status | Gate evidence |
|---|---|---|---|---|---|
| 0 | **Documentation** (not in PDF) | — | Owner approves docs + ambiguities register | 🟢 Approved 2026-11-06 (owner started Phase 1) | [DOCUMENTATION_AUDIT.md](DOCUMENTATION_AUDIT.md) |
| 1 | Foundations | Week 1 | CI green | 🟢 Gate passed 2026-11-06. Merged to `develop` (PR #1) | [Phase 1 report](#phase-1--foundations), CI run [42148048247](https://github.com/Varsh1l01/SurgeTix/actions/runs/42148048247) |
| 2 | Naive baseline | Week 2 | Oversell seen | 🟢 Gate passed 2026-11-06. Merged to `develop` (PR #2) | [Phase 2 report](#phase-2--naive-baseline), CI run [42159920841](https://github.com/Varsh1l01/SurgeTix/actions/runs/42159920841) |
| 3 | Booking core | Weeks 3–4 in the PDF; **no fixed timeline** (owner, 2026-11-06): milestone- and gate-driven | T1, T7, T8 pass | 🟡 M1 implemented (T1–T13); **M1 gate pending** (CI `-race` run + owner review) | [Phase 3 plan](docs/10-planning/phase-3-booking-core.md) |
| 4 | Services and gRPC | Week 5 | e2e hold works | ⚪ Not started | — |
| 5 | Waiting room | Week 6 | T2, T10, T13 pass | ⚪ Not started | — |
| 6 | Checkout saga | Weeks 7–8 | T3, T5, T12 pass | ⚪ Not started | — |
| 7 | Lifecycle features | Weeks 9–10 | T9, T11 pass | ⚪ Not started | — |
| 8 | UI and dashboards | Week 11 | Full demo works | ⚪ Not started | — |
| 9 | Ship and measure | Week 12 | README report | ⚪ Not started | — |

Legend: ⚪ not started · 🔵 in progress · 🟡 awaiting approval · 🟢 gate passed · 🔴 gate failed

## Phase 0 — what was done

| Item | Result |
|---|---|
| Read source PDF | v2, 37 pages, including 4 diagrams. Diffed against v1 (33 pages): v2 adds *Data flow* (p.11–12) and an expanded *Tech stack* (p.13–15); everything else is identical |
| `docs/` structure | 11 sections, 41 Markdown files (incl. index), 4 diagram assets |
| Root files | `CLAUDE.md`, `README.md`, `PROGRESS.md`, `ARCHITECTURE_DECISIONS.md` (32 ADRs), `ARCHITECTURE_REVIEW.md`, `TASKS.md`, `DOCUMENTATION_AUDIT.md` |
| Ambiguities | 37 logged: 36 awaiting approval (2 of them narrowed by v2), 1 resolved by v2. See the [register](docs/10-planning/ambiguities.md) |
| Open questions from the PDF | 7 transcribed with blocking phase ([open-questions](docs/10-planning/open-questions.md)) |
| Validation | See [DOCUMENTATION_AUDIT.md](DOCUMENTATION_AUDIT.md). Final pre-commit review completed: 26/26 Mermaid diagrams parse, DDL matches the PDF exactly, 0 broken links |
| Review for approval | [ARCHITECTURE_REVIEW.md](ARCHITECTURE_REVIEW.md): 20 decision groups, 11 PDF inconsistencies, 14 risks |
| Application code | **None** (by instruction) |

## Phase 1 — Foundations

**Gate:** CI green (PDF p.36). **Status:** 🟢 **passed 2026-11-06**.

### Gate evidence

| Item | Result |
|---|---|
| Implementation commit | `01810e4` *feat: phase 1 foundations (service skeletons, platform, Compose, CI)* |
| Pull request | [#1](https://github.com/Varsh1l01/SurgeTix/pull/1) `feature/phase-1-foundations` → `develop`, merged 2026-11-06 02:09 UTC as `1994bc2` |
| GitHub Actions | Run [42148048247](https://github.com/Varsh1l01/SurgeTix/actions/runs/42148048247) (`pull_request`, head `01810e4`): **success** |
| ↳ `checks` | ✅ success: gofmt, `go mod tidy` clean, vet (both tags), `go test -race`, build of all services |
| ↳ `integration` | ✅ success: `go test -race -tags=integration` against real Postgres, Redis and Kafka (testcontainers, Ryuk enabled) |
| ↳ `compose` | ✅ success: `docker compose up -d --wait` became healthy from `.env.example` |
| Owner approval | Owner merged PR #1 and instructed that Phase 1 be recorded as complete (2026-11-06) |

### Implemented

| Component | Where |
|---|---|
| Single-module layout (ADR-033), 7 binaries | `go.mod`, `cmd/{gateway,identity,catalog,waitingroom,booking,checkout,notifier}` |
| Env config (fail fast, all errors at once, optional `.env`, no default credentials) | `internal/platform/config` |
| slog JSON logging (library output routed through it too) | `internal/platform/logging`, `internal/platform/redisx` |
| `/healthz`, `/readyz` (per-dependency, 503 while draining), graceful shutdown | `internal/platform/service`, `internal/platform/app` |
| Postgres (pgx), Redis (go-redis), Kafka (franz-go, `acks=all`) clients, all lazy | `internal/platform/{postgres,redisx,kafka}` |
| Compose: PostgreSQL 17.11, Redis 7.4.12, Kafka 4.4.0 KRaft, health checks, volumes, private network, ports on 127.0.0.1 | `compose.yaml` |
| 4 service databases, each with its own role and no cross-database `CONNECT` | `deploy/compose/postgres-init/01-service-databases.sh` |
| Safe env template | `.env.example` |
| CI: gofmt, tidy, vet, `-race` tests, build; integration job; compose-health job | `.github/workflows/ci.yml` |
| LF line endings for Go/shell/YAML/SQL | `.gitattributes` |

### Verification actually run (2026-11-06, Windows 11, Go 1.27.3)

| Check | Result |
|---|---|
| `gofmt -l .` | ✅ no files |
| `go vet ./...` and `go vet -tags=integration ./...` | ✅ pass |
| `go test -count=1 -cover ./...` | ✅ all pass. Coverage: config 93.1 %, service 91.8 %, app 63.6 %, kafka/logging/postgres/redisx 100 % |
| `go build ./cmd/...` | ✅ all 7 binaries |
| `go mod tidy` | ✅ no changes |
| Real binary smoke test (`booking`, deps down) | ✅ `/healthz` 200, `/readyz` 503 listing postgres/redis/kafka, all log lines JSON, password never logged |
| Missing config (`notifier` without `KAFKA_BROKERS`) | ✅ exit 1 with a clear message |
| `docker compose --env-file .env.example config` | ✅ valid. Without `.env` it refuses to start (required secrets) |
| `docker compose up -d --wait` | ✅ all 3 containers `(healthy)` on host ports 15432 / 16379 / 29092 (moved from the defaults by owner decision, because a host Postgres held 5432 and WSL held 6379) |
| End-to-end smoke test (all 7 binaries against the Compose stack) | ✅ every service: `/healthz` 200, `/readyz` 200 with its own dependency checks. Stopping Redis turned gateway and booking `/readyz` to 503 naming the failed check. `booking_svc` → `checkout_db` is refused (`no CONNECT privilege`) |
| Integration tests (`-tags=integration`) | ✅ all pass against real Postgres, Redis and Kafka (testcontainers), re-run later the same day. Needed `TESTCONTAINERS_RYUK_DISABLED=true` locally because the Ryuk reaper container fails on this Docker Desktop. CI runs with Ryuk enabled |
| `go test -race` | ⚠️ **not run locally** (the installed MinGW gcc is 32-bit). ✅ passed in CI (`checks` and `integration` jobs) |
| Workflow lint (actionlint) | ⚠️ **not run** locally. GitHub parsed and ran the workflow successfully |
| Re-run on `develop` after the merge (`1994bc2`) | ✅ gofmt clean, vet (both tags), build, and all unit + integration tests pass (`TESTCONTAINERS_RYUK_DISABLED=true`). Coverage: config 93.1 %, service 91.8 %, app 65.2 %, kafka/logging/postgres/redisx 100 % |

### Known limitations

- CI triggers only on pull requests and on pushes to `main`. Commits pushed straight to `develop` (such as documentation updates) are not run by CI.
- `-race` cannot run on the current development machine (32-bit C compiler). CI is the only place it runs.
- Locally, testcontainers needs `TESTCONTAINERS_RYUK_DISABLED=true` because the Ryuk reaper fails on this Docker Desktop. With Ryuk off, a crashed test run can leave containers behind (`docker ps -a`).
- Host ports are non-default (Postgres 15432, Redis 16379, Kafka 29092, services 18080–18086) because other software on the development machine holds 5432, 6379 and 8080.
- Services run on the host. There are no Dockerfiles yet; containerising services belongs with Kubernetes (phase 9).
- `goose`, `sqlc` and `buf` are not wired yet (no migrations, queries or protos exist). Deferred to phases 3 and 4.
- No Kafka topics exist, and auto-creation is disabled (AMB-03 was open then; decided 2026-11-06 for `booking.holds` / `booking.inventory`).
- Local Postgres has no standby, and Redis is a single node (AMB-20 remains open for Kubernetes).
- While investigating Docker, `wsl --update` upgraded WSL from 2.6.3 to 3.0.3 on this machine. That was unplanned. Docker Desktop (Engine 29.2.1) later started working.

## Phase 2 — Naive baseline

**Gate:** oversell seen (PDF p.36), plus the owner's nine acceptance criteria. **Status:** 🟢 **passed 2026-11-06** (owner).

### Gate evidence

| Item | Result |
|---|---|
| Pull request | [#2](https://github.com/Varsh1l01/SurgeTix/pull/2) `feature/phase-2-naive-baseline` → `develop`, merged 2026-11-06 04:12 UTC as `6d26dc4` (head `6c07b5e`) |
| GitHub Actions | Run [42159920841](https://github.com/Varsh1l01/SurgeTix/actions/runs/42159920841) (`pull_request`, head `6c07b5e`): **success** |
| ↳ `checks` | ✅ gofmt, `go mod tidy` clean, vet, **unit tests (race)**, **unit tests (race, unsafe_baseline build)**, build, *normal binaries do not contain the unsafe baseline*, build of the unsafe_baseline variants |
| ↳ `integration` | ✅ `go test -race -tags=integration,unsafe_baseline ./...` (real Postgres/Redis/Kafka, Ryuk enabled) |
| ↳ `compose` | ✅ Compose stack healthy from `.env.example` |
| Both race-detector steps | ✅ passed in CI (they cannot run on the development machine) |
| Evidence preserved | Deterministic oversell tests and measured results: [phase-2-naive-baseline](docs/10-planning/phase-2-naive-baseline.md) §5 and §8. Raw run output kept locally in `loadtest/results/` (git-ignored, `948ddb3`) |
| Owner approval | Owner confirmed Phase 2 complete and started Phase 3 (2026-11-06) |

| Item | State |
|---|---|
| Shape and decisions | ✅ [ADR-034](ARCHITECTURE_DECISIONS.md#adr-034) (OQ-7), [ADR-035](ARCHITECTURE_DECISIONS.md#adr-035) (AMB-38 … AMB-43) |
| Code | `internal/booking/baseline`, `internal/gateway/baseline`, `internal/platform/{unsafebaseline,metrics,problem}`, `test/e2e`, `loadtest/`. Compiled only with `-tags unsafe_baseline` and served only with `SURGETIX_ENABLE_UNSAFE_BASELINE=true` |
| Deterministic oversell | ✅ Real Postgres with a barrier: 2→1 seat = 2 bookings, 50→1 = 50, 200→10 = 200 (excess 190), overlapping multi-seat. Sequential control = 0 oversell. Isolation stays READ COMMITTED under a SERIALIZABLE server default. Also over HTTP through the Gateway ([plan §5](docs/10-planning/phase-2-naive-baseline.md#5-deterministic-concurrency-evidence)) |
| Benchmarks | ✅ 50/100/250 buyers × 3 recorded runs (+ warm-ups), k6 2.3.0. **Every run oversold: 10–20 double-booked seats**. Median req/s 679 / 484 / 560. Median p99 172.5 / 566.3 / 1,018.7 ms. Error rate 0 / 0 / 1.6 % (504 at the 1 s deadline) ([plan §8](docs/10-planning/phase-2-naive-baseline.md#8-measured-results)) |
| 500 buyers | ❌ Not run. Recommended against on this machine (p99 at the 1 s deadline at 250, 0.72 GB free RAM, degraded sampling). Owner to decide ([plan §8.4](docs/10-planning/phase-2-naive-baseline.md#84-500-buyers-not-run-recommendation)) |
| Local regression (2026-11-06) | ✅ gofmt, tidy, vet (4 tag sets), unit tests (both builds), integration tests (both tag sets, `-p 1`), builds, binary route check, Compose health, 7 services `/healthz` + `/readyz` 200, guard on real binaries. `-race` ❌ not runnable locally |
| CI on the PR | ✅ Run [42159920841](https://github.com/Varsh1l01/SurgeTix/actions/runs/42159920841): all three jobs passed, including both `-race` steps |

**Known limitations**
- **Benchmark caveat:** services run natively on Windows and Postgres in the Docker Desktop VM. The host→VM round trip measured **1.348 ms** vs **0.154 ms** inside the VM. Throughput varied up to 2.3× between repetitions. The causes of the variance are hypotheses ([plan §8.3](docs/10-planning/phase-2-naive-baseline.md#83-windowsdocker-networking-and-variance-caveat)).
- **No idempotency:** in one 250-buyer run, 6 bookings committed after their client got a 504.
- **Local test environment:** `testcontainers-go` v0.45.0 intermittently panics on Windows when test packages run in parallel ("rootless Docker is not supported on Windows"). Use `-p 1` locally. CI is Linux.
- **Sparse sampling:** the resource sampler captured 8–10 samples per 30 s run (target 1/s), and 1 sample in `b250-r1`.
- **`.gitignore`:** `loadtest/results/` is ignored since `948ddb3` (owner).

## Phase 3 — Booking core (in progress)

**Plan:** [phase-3-booking-core](docs/10-planning/phase-3-booking-core.md) (ADR-036). Milestones M1 → M5, one at a time, each closed by its gate (§14) and the owner.

### M1 — Schema, idempotency, chosen-seat holds and expiry: implemented, gate pending

| Task | Result | Owner review |
|---|---|---|
| T1 migration runner (session lock) | `internal/platform/migrate` | ✅ approved |
| T2 schema 00001–00004 | `internal/booking/migrations` (PDF DDL verbatim + approved 00004) | ✅ approved |
| T3 generated inventory | `internal/booking/inventory` | ✅ approved |
| T4 read-only audit (10 checks) | `internal/booking/inventory/audit.go` | ✅ approved |
| T5 idempotency (generation fencing) | `internal/platform/idempotency` | ✅ approved |
| T6 outbox insert helper | `internal/platform/outbox` | ✅ approved |
| T7 hold request validation | `internal/booking/holds/request.go` | ✅ approved |
| T8 `HoldSeats` | `internal/booking/holds/seats.go` (`lock_timeout` 500 ms) | ✅ approved |
| T9 inline and partial reclaim | `internal/booking/holds/reclaim.go` | ✅ approved |
| T10 hold service layer | `internal/booking/holds/service.go` | ✅ approved |
| T11 expiry worker | `internal/booking/holds/expire.go`, `internal/booking/expiry` | ✅ approved 2026-11-07 |
| T12 wiring, L6 stress, CI baseline guard | `cmd/booking/core.go`, `cmd/booking/pools.go`, `.github/workflows/ci.yml` | ✅ approved 2026-11-07 (+ 5 s idle-in-transaction timeout on the runtime pool, close-out) |
| T13 docs and this report | plan §4, §7–§9, §14; flow 4; booking service doc; operability doc | ✅ approved 2026-11-07 |

**Gate (plan §14.2):**

| # | Criterion | Status |
|---|---|---|
| 1 | Every M1 test passes locally (`-p 1`) **and in CI with `-race`** | ✅ locally, 2026-11-07: 26 integration package runs (12 normal-tag, 14 `integration,unsafe_baseline`), unit tests in both builds. ⏳ **CI not run**: it runs only on a PR or a push to `main`, which needs the owner. `-race` cannot run on this machine |
| 2 | Zero `40P01` in the L-suite and the stress test | ✅ D4 1,000 rounds, L2 300 rounds, L6 ~2,000 operations: 0 |
| 3 | Audit clean after every test (0 double-booked, 0 orphaned `HELD`) | ✅ T4 audit asserted after every hold, reclaim, service, worker and stress test |
| 4 | Schema fidelity; only deviation is the approved 00004 | ✅ T2 tests |
| 5 | Migration session-lock test proves one session | ✅ T1 tests; T12 also shows start-up waiting behind another migrator |
| 6 | Baseline byte-identical to `develop`; Phase 1 and 2 suites pass | ✅ `git diff develop -- <frozen paths>` empty; baseline and e2e integration suites pass; the CI step is added (runs with the next CI run) |
| 7 | Docs updated; owner reviews and approves | ✅ docs updated; ✅ owner approved T11–T13 (2026-11-07); ⏳ gate approval after CI |

**Evidence kept outside the repository:** verified checkpoints in `C:\Users\varsh\SurgeTix-checkpoints\` (manifest, SHA-256, restore test in a temporary clone for each).

**Open before M1 can close:** a green CI run of this branch (owner opens the PR or authorises a push), then the owner's gate approval. The idle-in-transaction timeout is decided and implemented (5 s, runtime pool only).

**Local environment notes (M1):** Docker Desktop's VM (3.7 GB) and the host (7.9 GB, heavy commit) ran short of memory several times. Once, the VM kernel's network lock got stuck (recovered with an owner-approved `wsl --shutdown --force`). Integration packages are therefore run strictly one at a time with a free-memory guard, and the Compose job is left to CI.

## Decisions needed before phase 1 (historical, see decision log)

| ID | Why it blocks |
|---|---|
| Approval of the doc set | Phase 0 gate |
| [OQ-3](docs/10-planning/open-questions.md) / [AMB-36](docs/10-planning/ambiguities.md#amb-36) | Data-access tooling chosen in Foundations |
| [AMB-34](docs/10-planning/ambiguities.md#amb-34) | Repository / module layout needed for the skeleton |
| [OQ-7](docs/10-planning/open-questions.md) | Shape of the naive baseline (phase 2). ✅ Decided 2026-11-06 ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)) |
| [AMB-20](docs/10-planning/ambiguities.md#amb-20) | Redis topology for Compose |
| [AMB-01](docs/10-planning/ambiguities.md#amb-01) | PDF file name, ideally fixed before the first commit |

High-severity ambiguities to settle before their phases: [AMB-04](docs/10-planning/ambiguities.md#amb-04), [AMB-06](docs/10-planning/ambiguities.md#amb-06), [AMB-07](docs/10-planning/ambiguities.md#amb-07), [AMB-10](docs/10-planning/ambiguities.md#amb-10).

## Decision log

| Date | ID | Decision | By |
|---|---|---|---|
| 2026-11-06 | Phase 0 | Documentation approved (owner instructed Phase 1 to start) | Owner |
| 2026-11-06 | AMB-34 | Single-module Go monorepo, `cmd/<service>` + `internal/platform` ([ADR-033](ARCHITECTURE_DECISIONS.md#adr-033)) | Owner |
| 2026-11-06 | Local ports | Non-default host ports to avoid clashes: Postgres 15432, Redis 16379, Kafka 29092, services 18080–18086 (`compose.yaml`, `.env.example`, `README.md`, `cmd/*`) | Owner |
| 2026-11-06 | Phase 1 | Gate passed (CI run 42148048247 green). PR #1 merged into `develop`. Phase 1 approved | Owner |
| 2026-11-06 | OQ-7 | Option B: the naive baseline is the same seven components without protections, for testing and benchmarking only ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)) | Owner |
| 2026-11-06 | AMB-38 … AMB-43 | Phase 2 baseline decisions ([ADR-035](ARCHITECTURE_DECISIONS.md#adr-035)). Also approved: baseline-only `hold_id` = booking id with `expires_at: null`, and fail-closed start when the flag is set on a normal build | Owner |
| 2026-11-06 | Phase 2 | Gate passed: PR #2 merged into `develop`, CI run 42159920841 green (all three jobs, both `-race` steps). Phase 2 approved | Owner |
| 2026-11-06 | AMB-03, AMB-48 schema, AMB-10 scope, AMB-09/26/49 scheduling | Topics `booking.holds` / `booking.inventory`; additive `idempotency_keys` columns approved; 1–4 limit in all three modes; tier mode, nearby alternatives and booked-only `seatbits` kept in Phase 3 (milestone M4); `SoldOut` condition to define (AMB-52); OQ-6 before cloud runs; k6 direct to Booking ([ADR-036](ARCHITECTURE_DECISIONS.md#adr-036) amendment) | Owner |
| 2026-11-06 | AMB-48 status code, AMB-47 details (M1-T5/T6) | Same idempotency key with a different body → `422` (PDF p.28); business conflicts stay `409`. T6 = outbox insert helper only (relay stays P4-5); inventory-event aggregate `section` = `event_id:section`; `occurred_at` RFC 3339 UTC, 6 fractional digits. Aggregate-version gap logged as [AMB-54](docs/10-planning/ambiguities.md#amb-54) (open) | Owner |
| 2026-11-06 | M1-T6, M1-T7 reviews; AMB-10 detail | T6 (outbox insert helper) and T7 (hold request validation) approved. Phase 3 constraints: quantity is a plain JSON integer; section ≤ 16 bytes, seat id ≤ 24 bytes (T3 generator limits), revisit with Catalog layouts in phase 4 | Owner |
| 2026-11-06 | M1 review item 5 (D1) | Hold transaction `lock_timeout` = **500 ms** (inside the 1 s `HoldSeats` deadline). `idle_in_transaction_session_timeout` stays open until T12 | Owner |
| 2026-11-06 | M1-T8, M1-T9 reviews | `HoldSeats` (T8) and inline/partial reclaim (T9) approved | Owner |
| 2026-11-07 | M1-T10 review | Hold service layer approved, with the derived behaviours: invalid bodies → unstored `422`; lost claim → retryable `503`; no automatic deadlock retry in T10 | Owner |
| 2026-11-07 | M1-T11/T12/T13 review; plan review item 5 | T11–T13 approved. `idle_in_transaction_session_timeout` = **5 s on the Booking runtime pool only** (not global, not on the role the migration runner shares); 500 ms `lock_timeout` and 30 s lease unchanged | Owner |
| 2026-11-07 | M1 execution directive | Owner authorised completing all locally executable Phase 3 work, removal of two named orphaned test containers, and no cloud spending or Git history changes | Owner |
| 2026-11-06 | Phase 2 acceptance criteria | Nine criteria in addition to the PDF gate "oversell seen" ([TASKS.md](TASKS.md), Phase 2) | Owner |
| 2026-11-06 | `.gitignore` | Added `bin/`, `*.out`, `*.prof`, `*.pprof`, `desktop.ini`, `kubeconfig*`, `.claude/settings.local.json` | Owner |

## Change log

| Date | Change |
|---|---|
| 2026-11-07 | Phase 3 M1 implemented (T1–T13): schema, idempotency, outbox, validated chosen-seat holds with inline and partial reclaim, hold service, expiry worker, start-up wiring, L6 stress, CI baseline guard. Gate pending (CI + owner) |
| 2026-11-06 | Phase 0: documentation repository initialised from PDF v2. Citations moved from v1 to v2 after the source was replaced mid-task |
| 2026-11-06 | Phase 1 Foundations implemented (service skeletons, platform packages, Compose, CI). See the Phase 1 section |
| 2026-11-06 | Phase 1 re-verification: Docker available, integration tests pass, Compose stack healthy on the new host ports, all 7 services ready end to end |
| 2026-11-06 | Phase 1 gate recorded as passed (CI green, PR #1 merged into `develop`). Current phase moved to 2, blocked by OQ-7 |
| 2026-11-06 | Phase 2 planning: OQ-7 recorded as ADR-034, plan written, AMB-38 … AMB-43 logged, P2-6 … P2-15 added. No application code |
| 2026-11-06 | Phase 2 implemented: guarded naive baseline, deterministic oversell tests, Gateway route, `/metrics`, k6 workflow. Local matrix 50/100/250 × 3 measured and documented |
| 2026-11-06 | Phase 2 gate recorded as passed (CI run 42159920841, PR #2 merged). Current phase moved to 3 (planning) |
| 2026-11-06 | Phase 3 plan finalised with owner decisions (ADR-036): AMB-09, -10, -26, -27, -28, -36, -44 … -51; no fixed timeline |
| 2026-11-06 | Phase 3 plan revised (ADR-036 amendment): AMB-03 and AMB-48 schema approved; milestones M1–M5 with M4 = PDF completeness; AMB-52 logged. No decision blocks M1 |
| 2026-11-06 | Final pre-commit review: fixed 4 Mermaid parse errors and 4 inaccurate statements, restructured CLAUDE.md into permanent and phase rules, added ARCHITECTURE_REVIEW.md, refreshed DOCUMENTATION_AUDIT.md |
