# CLAUDE.md — Engineering Instructions for SurgeTix

Read this before doing anything in the repository. The sections marked **Permanent** apply for the life of the project. The section marked **Current phase** changes as phases advance and is updated in the same change that records a gate in `PROGRESS.md`.

## What this repo is

**SurgeTix** is a flash-sale ticketing backend (Go, PostgreSQL, Redis, Kafka, gRPC) that must **never double-sell a seat** and **never mishandle money**, while millions of buyers arrive at once.

- **Source of truth:** `reference/SurgeTix Product Requirements & System Architecture (2).pdf` (37 pages).
- **Engineering docs:** `docs/`, derived from the PDF and indexed at [`docs/README.md`](docs/README.md).
- **Status:** [`PROGRESS.md`](PROGRESS.md) · **Backlog:** [`TASKS.md`](TASKS.md) · **Decisions:** [`ARCHITECTURE_DECISIONS.md`](ARCHITECTURE_DECISIONS.md) · **Open review items:** [`ARCHITECTURE_REVIEW.md`](ARCHITECTURE_REVIEW.md)

---

## Current phase — Phase 2 (Naive baseline), blocked by OQ-7

Phase 0 was approved on 2026-11-06. Phase 1 (Foundations) passed its gate on 2026-11-06: CI run 42148048247 was green, and PR #1 was merged into `develop` (`1994bc2`). Evidence is in `PROGRESS.md`.

Phase 2 cannot start until the owner decides **OQ-7** (monolith vs the same services without the waiting room and atomic holds). Until then:
1. **No Phase 2 code**: P2-2 … P2-5 depend on P2-1 (OQ-7).
2. The Phase 1 limits still hold: **no business logic** (holds, queue, admission, payments, sagas, transfers, scanning, frontend, Kubernetes), **no tables** beyond the Compose databases and roles, **no Kafka topics** (AMB-03).

Before phase 3: **OQ-3 / AMB-36** (sqlc), **AMB-10**, **AMB-09**.

**Local environment:** host ports are non-default (Postgres 15432, Redis 16379, Kafka 29092, services 18080–18086). `-race` runs only in CI on this machine. Local integration tests need `TESTCONTAINERS_RYUK_DISABLED=true`. CI runs on pull requests and on pushes to `main`, but not on direct pushes to `develop`.

**Code layout (ADR-033):** `cmd/<service>/main.go` declares only that service's dependencies via `app.Main`. Shared infrastructure lives in `internal/platform/`. Business code goes in `internal/<service>/`, which must never import another service's package. Each service reads only its own `<SERVICE>_DATABASE_URL`.

---

## Permanent — How to work

### P1. Never invent specifications
If the PDF or an approved decision does not answer a question, **stop and ask**. Log the gap in [`docs/10-planning/ambiguities.md`](docs/10-planning/ambiguities.md) (next free `AMB-nn`), list options without choosing, and link it from every affected doc. Do not fill gaps with "reasonable defaults" in code or docs.

### P2. Source hierarchy (when things disagree)
1. Owner decisions recorded as ✅ in the ambiguities register or as dated ADRs
2. The PDF. Where the PDF contradicts itself, treat it as an ambiguity until resolved
3. `docs/`
4. Code comments, anything else

### P3. Phase discipline
- Work only on the **current** phase in `TASKS.md`. A phase is **done only when its gate passes** (PDF p.36–37): *the timeline stretches rather than skipping correctness work.*
- Record each gate result (with evidence: test output, k6 summary, audit result) in `PROGRESS.md`, then update the **Current phase** section above.
- A task blocked by an `AMB-xx` / `OQ-x` stays blocked until that item is approved.

### P4. Changing the architecture
Any deviation from the docs or the PDF (new table, column, endpoint, topic, service, dependency, or a weakened guarantee) requires an **ADR approved by the owner** *before* the change. Lifecycle features land **inside existing services**; do not add services (ADR-021).

### P5. Docs move with code
A change that alters behaviour, schema, an API, an event or a parameter updates the matching doc in the **same change**. Re-run the documentation checks in [`DOCUMENTATION_AUDIT.md`](DOCUMENTATION_AUDIT.md) §8 after doc edits.

### P6. Git
Do not commit, push, rebase or create branches unless asked. `.gitignore` is owner-maintained; propose edits, don't make them. Never commit secrets, `.env` files, keys, generated data directories or build output.

---

## Permanent — Correctness invariants

These come straight from the PDF ([correctness-guarantees](docs/07-correctness/correctness-guarantees.md)). Weakening any of them requires an approved ADR.

1. **Every** seat / GA bucket / hold / order / refund / ticket / saga transition is a **conditional `UPDATE … WHERE status = $from`** with a rows-affected check. Any other transition is rejected.
2. **A seat hold is all or nothing**: rows affected ≠ seats requested → rollback.
3. **Postgres decides; Redis, caches and Kafka never grant.** `seatbits` may only reject early. `SKIP LOCKED` only chooses candidates.
4. **Never weaken** `CHECK (remaining >= 0)`, `one_live_ticket_per_seat`, `bookings.order_id UNIQUE`, `payments.order_id UNIQUE`, `provider_ref UNIQUE`, `refund_items.ticket_id` PK, `refunded_paise ≤ amount_paise`, `transfer_count ≤ 1`, or `one_full_refund_per_rejected_order`.
5. **Persist payment success before calling `ConfirmHold`** (one transaction: payment `SUCCEEDED`, order `PAID_PENDING_CONFIRMATION`, saga step pending).
6. **Never refund on a timeout, `UNAVAILABLE` or unknown error.** Refund only on a definitive `REJECTED` or a cancellation.
7. **Every state-changing endpoint takes an `Idempotency-Key`** (completed → stored response; in progress → 409; different body → 422). Consumers dedupe via `processed_events` and ignore stale versions.
8. **Never publish to Kafka outside the transactional outbox.** Producers use `acks=all`.
9. **`holds.expires_at` is canonical**; `seats.hold_expires_at` is only written in the same transaction.
10. **Admission-counter writes check the lease epoch inside the same Lua script.** `admitted` only increases.
11. **No service reads another service's tables.** Cross-service needs go through gRPC or Kafka.
12. **Parameterized SQL only. QR secrets are never sent in plain form.**
13. **Admissions fail closed** if Redis is unavailable.
14. **Money is integer paise**, never floating point.

---

## Permanent — Implementation standards (from the PDF)

| Area | Standard | PDF |
|---|---|---|
| Language/build | Go 1.27; all services built with **PGO**; hot paths allocation-checked | p.7, p.13 |
| Data access | PostgreSQL 17, `pgx`; `sqlc` vs hand-written per ADR-024 once approved; `goose` migrations; no ORM | p.14 |
| Locking | `FOR UPDATE SKIP LOCKED` for candidate selection, expiry batches and outbox claims; ordered `FOR UPDATE` where waiting is required (GA retry) | p.28–30 |
| Internal calls | gRPC + Protobuf, one package per service, `buf` lint and breaking-change checks; propagate the 1 s gateway deadline; retry only `UNAVAILABLE` with jittered backoff | p.13, p.27 |
| Public API | REST under `/v1/`, SSE for live updates, RFC 9457 errors | p.24 |
| Sagas | Durable steps in `sagas` / `saga_steps`; every step safe to retry; optimistic concurrency on `version` | p.20, p.27 |
| Observability | RED metrics per service; the domain and efficiency metrics in [observability](docs/08-operations/observability.md); OpenTelemetry traces across gateway → service → DB/Redis → Kafka → notifier; `slog` JSON with `trace_id`, `user_id`, `event_id`, `order_id` | p.8, p.34 |
| Testing | `go test -race`; `testcontainers-go` with **real** Postgres, Redis and Kafka (no mock-only tests for SQL or locking); table-driven and fuzz tests | p.15 |
| Load tests | k6 arrival-rate executors; the **same profile** against the naive baseline and SurgeTix; run the audit (`double_booked_seats`, `oversold_ga` = 0) **after every run** | p.3, p.15, p.34 |
| Fault injection | Toxiproxy, `kubectl delete pod`, `SIGSTOP` | p.15 |
| Run/deploy | One-command Docker Compose; Kubernetes with probes, HPA, PDBs, graceful shutdown, ≥ 2 replicas per stateless service | p.8, p.15 |

---

## Permanent — Documentation conventions

- Every doc starts with `*Source: PDF p.X–Y …*` (pages of the current PDF revision).
- Mark readings of the PDF that are not quotes as **Derived:**.
- IDs are never renumbered: `FR-n`, `NFR-*`, `T-n`, `F-n`, `G-n`, `ADR-nnn`, `OQ-n`, `AMB-nn`, `R-n`, `I-n`. Strike through retired items instead of deleting them.
- Mermaid for diagrams. **Do not use `;` inside sequence-diagram text** (Mermaid treats it as a statement separator). The original PDF images in `docs/assets/` win over transcriptions.
- When a decision is approved: mark it ✅ (with date) in the register, add or update the ADR, update every linking doc, log it in `PROGRESS.md`.

## Build order

1 Foundations (CI green) → 2 Naive baseline (oversell seen) → 3 Booking core (T1, T7, T8) → 4 Services & gRPC (e2e hold) → 5 Waiting room (T2, T10, T13) → 6 Checkout saga (T3, T5, T12) → 7 Lifecycle (T9, T11) → 8 UI & dashboards (full demo) → 9 Ship & measure (README report). Details: [build-phases](docs/10-planning/build-phases.md).

## Where to look

| Need | Doc |
|---|---|
| Requirements + traceability | `docs/01-requirements/functional-requirements.md` |
| Architecture, data flow, tech stack | `docs/02-architecture/` |
| Service boundaries | `docs/03-services/*.md` |
| DDL, state machines, Redis, Kafka | `docs/04-data/` |
| APIs | `docs/05-api/` |
| Flows | `docs/06-flows/*.md` |
| Guarantees and failure handling | `docs/07-correctness/` |
| Tests and gates | `docs/09-testing/test-plan.md` |
| Unresolved items | `ARCHITECTURE_REVIEW.md`, `docs/10-planning/ambiguities.md`, `docs/10-planning/open-questions.md` |
