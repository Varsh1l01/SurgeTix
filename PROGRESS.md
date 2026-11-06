# Progress

*Tracks status against the 9-phase build plan (PDF p.36) plus phase 0 (documentation). A phase moves on **only when its gate passes** (PDF p.37).*

**Last updated:** 2026-11-06
**Current phase:** **1 — Foundations** → status: **Implemented, awaiting first CI run and owner review**

## Phase status

| # | Phase | Planned weeks | Gate | Status | Gate evidence |
|---|---|---|---|---|---|
| 0 | **Documentation** (not in PDF) | — | Owner approves docs + ambiguities register | 🟢 Approved 2026-11-06 (owner started Phase 1) | [DOCUMENTATION_AUDIT.md](DOCUMENTATION_AUDIT.md) |
| 1 | Foundations | Week 1 | CI green | 🟡 Implemented; gate pending the first CI run on GitHub | [Phase 1 report](#phase-1--foundations) |
| 2 | Naive baseline | Week 2 | Oversell seen | ⚪ Not started (blocked by OQ-7) | — |
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

**Gate:** CI green (PDF p.36). **Status:** 🟡 implemented; gate pending the first CI run.

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
| `go test -race` | ❌ **not run locally**: the installed MinGW gcc is 32-bit. Runs in CI |
| Workflow lint (actionlint) | ❌ **not run**: blocked by tool permissions. GitHub validates the workflow on push |

### Known limitations

- The Phase 1 gate cannot be confirmed until the CI jobs `checks`, `integration` and `compose` pass on GitHub.
- Services run on the host. There are no Dockerfiles yet; containerising services belongs with Kubernetes (phase 9).
- `goose`, `sqlc` and `buf` are not wired yet (no migrations, queries or protos exist). Deferred to phases 3 and 4.
- No Kafka topics exist, and auto-creation is disabled (AMB-03 open).
- Local Postgres has no standby, and Redis is a single node (AMB-20 remains open for Kubernetes).
- While investigating Docker, `wsl --update` upgraded WSL from 2.6.3 to 3.0.3 on this machine. That was unplanned and did not fix the WSL error; a reboot or a *Virtual Machine Platform* repair may be needed.

## Decisions needed before phase 1 (historical, see decision log)

| ID | Why it blocks |
|---|---|
| Approval of the doc set | Phase 0 gate |
| [OQ-3](docs/10-planning/open-questions.md) / [AMB-36](docs/10-planning/ambiguities.md#amb-36) | Data-access tooling chosen in Foundations |
| [AMB-34](docs/10-planning/ambiguities.md#amb-34) | Repository / module layout needed for the skeleton |
| [OQ-7](docs/10-planning/open-questions.md) | Shape of the naive baseline (phase 2) |
| [AMB-20](docs/10-planning/ambiguities.md#amb-20) | Redis topology for Compose |
| [AMB-01](docs/10-planning/ambiguities.md#amb-01) | PDF file name, ideally fixed before the first commit |

High-severity ambiguities to settle before their phases: [AMB-04](docs/10-planning/ambiguities.md#amb-04), [AMB-06](docs/10-planning/ambiguities.md#amb-06), [AMB-07](docs/10-planning/ambiguities.md#amb-07), [AMB-10](docs/10-planning/ambiguities.md#amb-10).

## Decision log

| Date | ID | Decision | By |
|---|---|---|---|
| 2026-11-06 | Phase 0 | Documentation approved (owner instructed Phase 1 to start) | Owner |
| 2026-11-06 | AMB-34 | Single-module Go monorepo, `cmd/<service>` + `internal/platform` ([ADR-033](ARCHITECTURE_DECISIONS.md#adr-033)) | Owner |
| 2026-11-06 | Local ports | Non-default host ports to avoid clashes: Postgres 15432, Redis 16379, Kafka 29092, services 18080–18086 (`compose.yaml`, `.env.example`, `README.md`, `cmd/*`) | Owner |
| 2026-11-06 | `.gitignore` | Added `bin/`, `*.out`, `*.prof`, `*.pprof`, `desktop.ini`, `kubeconfig*`, `.claude/settings.local.json` | Owner |

## Change log

| Date | Change |
|---|---|
| 2026-11-06 | Phase 0: documentation repository initialised from PDF v2. Citations moved from v1 to v2 after the source was replaced mid-task |
| 2026-11-06 | Phase 1 Foundations implemented (service skeletons, platform packages, Compose, CI). See the Phase 1 section |
| 2026-11-06 | Phase 1 re-verification: Docker available, integration tests pass, Compose stack healthy on the new host ports, all 7 services ready end to end |
| 2026-11-06 | Final pre-commit review: fixed 4 Mermaid parse errors and 4 inaccurate statements, restructured CLAUDE.md into permanent and phase rules, added ARCHITECTURE_REVIEW.md, refreshed DOCUMENTATION_AUDIT.md |
