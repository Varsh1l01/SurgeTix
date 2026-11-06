# Progress

*Tracks status against the 9-phase build plan (PDF p.36) plus phase 0 (documentation). A phase moves on **only when its gate passes** (PDF p.37).*

**Last updated:** 2026-11-06
**Current phase:** **2 — Naive baseline** → status: **Planning. OQ-7 decided ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)). Implementation not started, waiting for owner approval of the [plan](docs/10-planning/phase-2-naive-baseline.md) and AMB-38 … AMB-43**. Phase 1 gate passed on 2026-11-06

## Phase status

| # | Phase | Planned weeks | Gate | Status | Gate evidence |
|---|---|---|---|---|---|
| 0 | **Documentation** (not in PDF) | — | Owner approves docs + ambiguities register | 🟢 Approved 2026-11-06 (owner started Phase 1) | [DOCUMENTATION_AUDIT.md](DOCUMENTATION_AUDIT.md) |
| 1 | Foundations | Week 1 | CI green | 🟢 Gate passed 2026-11-06. Merged to `develop` (PR #1) | [Phase 1 report](#phase-1--foundations), CI run [42148048247](https://github.com/Varsh1l01/SurgeTix/actions/runs/42148048247) |
| 2 | Naive baseline | Week 2 | Oversell seen | 🔵 Planning (OQ-7 decided; no code yet) | [Phase 2 plan](docs/10-planning/phase-2-naive-baseline.md) |
| 3 | Booking core | Weeks 3–4 | T1, T7, T8 pass | ⚪ Not started | — |
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
- No Kafka topics exist, and auto-creation is disabled (AMB-03 open).
- Local Postgres has no standby, and Redis is a single node (AMB-20 remains open for Kubernetes).
- While investigating Docker, `wsl --update` upgraded WSL from 2.6.3 to 3.0.3 on this machine. That was unplanned. Docker Desktop (Engine 29.2.1) later started working.

## Phase 2 — Naive baseline

**Gate:** oversell seen (PDF p.36), plus the owner's nine acceptance criteria. **Status:** 🔵 planning.

| Item | State |
|---|---|
| Shape | ✅ OQ-7 → Option B, the same seven components without protections ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)) |
| Plan | [docs/10-planning/phase-2-naive-baseline.md](docs/10-planning/phase-2-naive-baseline.md) |
| Open, blocking code | [AMB-38](docs/10-planning/ambiguities.md#amb-38) tables · [AMB-39](docs/10-planning/ambiguities.md#amb-39) contract · [AMB-40](docs/10-planning/ambiguities.md#amb-40) transport · [AMB-41](docs/10-planning/ambiguities.md#amb-41) isolation · [AMB-42](docs/10-planning/ambiguities.md#amb-42) metrics · [AMB-43](docs/10-planning/ambiguities.md#amb-43) T6 scope and scale |
| Application code | **None.** No tests, benchmarks or oversell results exist yet |

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
| 2026-11-06 | Phase 2 acceptance criteria | Nine criteria in addition to the PDF gate "oversell seen" ([TASKS.md](TASKS.md#phase-2--naive-baseline-week-2--gate-oversell-seen--current-planning)) | Owner |
| 2026-11-06 | `.gitignore` | Added `bin/`, `*.out`, `*.prof`, `*.pprof`, `desktop.ini`, `kubeconfig*`, `.claude/settings.local.json` | Owner |

## Change log

| Date | Change |
|---|---|
| 2026-11-06 | Phase 0: documentation repository initialised from PDF v2. Citations moved from v1 to v2 after the source was replaced mid-task |
| 2026-11-06 | Phase 1 Foundations implemented (service skeletons, platform packages, Compose, CI). See the Phase 1 section |
| 2026-11-06 | Phase 1 re-verification: Docker available, integration tests pass, Compose stack healthy on the new host ports, all 7 services ready end to end |
| 2026-11-06 | Phase 1 gate recorded as passed (CI green, PR #1 merged into `develop`). Current phase moved to 2, blocked by OQ-7 |
| 2026-11-06 | Phase 2 planning: OQ-7 recorded as ADR-034, plan written, AMB-38 … AMB-43 logged, P2-6 … P2-15 added. No application code |
| 2026-11-06 | Final pre-commit review: fixed 4 Mermaid parse errors and 4 inaccurate statements, restructured CLAUDE.md into permanent and phase rules, added ARCHITECTURE_REVIEW.md, refreshed DOCUMENTATION_AUDIT.md |
