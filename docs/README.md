# SurgeTix Engineering Documentation

This directory is the engineering documentation set for **SurgeTix**, a flash-sale ticketing backend that must stay correct and responsive when demand exceeds supply by 100× or more.

The **single source of truth** for everything here is the source specification:

> `reference/SurgeTix Product Requirements & System Architecture (2).pdf` (37 pages, dated Nov 4, 2026, author @Varshil). This set was written against this **second revision** of the PDF; see [AMB-01](10-planning/ambiguities.md#amb-01).

Every document cites the PDF pages it was derived from (e.g. *Source: PDF p.17–21*). Where the PDF is silent, contradictory or open, the gap is **not filled in here**. It is logged in [`10-planning/ambiguities.md`](10-planning/ambiguities.md) with an ID (`AMB-xx`) and awaits owner approval.

## How to read this set

| If you want to… | Start with |
|---|---|
| Understand what SurgeTix is and why it exists | [00-overview/product-overview.md](00-overview/product-overview.md) |
| Know what "done" and "success" mean | [00-overview/goals-and-success-metrics.md](00-overview/goals-and-success-metrics.md) |
| See every requirement and where it is implemented/tested | [01-requirements/functional-requirements.md](01-requirements/functional-requirements.md) |
| Understand the shape of the system | [02-architecture/system-architecture.md](02-architecture/system-architecture.md) |
| Work on a specific service | [03-services/](03-services/) |
| Write SQL, Redis or Kafka code | [04-data/](04-data/) |
| Add or change an endpoint / RPC | [05-api/](05-api/) |
| Implement an end-to-end behaviour | [06-flows/](06-flows/) |
| Reason about correctness and failure | [07-correctness/](07-correctness/) |
| Run, deploy or observe the system | [08-operations/](08-operations/) |
| Write or run tests and load tests | [09-testing/test-plan.md](09-testing/test-plan.md) |
| Know what to build next and what is undecided | [10-planning/](10-planning/) |

## Directory map

```
docs/
├── README.md                         ← this index
├── assets/                           ← diagrams extracted verbatim from the PDF
│   ├── architecture-overview.png     (PDF p.10)
│   ├── data-flow.png                 (PDF p.12)
│   ├── seat-lifecycle.png            (PDF p.27)
│   └── build-plan.png                (PDF p.36)
├── 00-overview/
│   ├── product-overview.md           problem, approach, scope, differentiators
│   ├── goals-and-success-metrics.md  goals, non-goals, metric targets
│   ├── users-and-journeys.md         actors, happy path, unhappy paths
│   └── glossary.md                   domain and technical vocabulary
├── 01-requirements/
│   ├── functional-requirements.md    FR-1…FR-23 + traceability matrix
│   ├── non-functional-requirements.md consistency, latency, efficiency, security…
│   └── capacity-estimates.md         reference sale, load figures, Redis hot spot
├── 02-architecture/
│   ├── system-architecture.md        funnel, services, communication, data ownership
│   ├── data-flow.md                  one purchase end to end: 4 phases, 27 steps
│   └── technology-choices.md         full tech stack, versions, alternatives rejected
├── 03-services/
│   ├── gateway.md
│   ├── identity.md
│   ├── catalog.md
│   ├── waiting-room.md
│   ├── booking.md
│   ├── checkout.md
│   ├── notifier.md
│   └── fakepay-simulator.md
├── 04-data/
│   ├── postgres-schema.md            all DDL from the PDF, annotated
│   ├── state-machines.md             seat, hold, order, refund, ticket, saga step
│   ├── redis-keys.md                 key catalogue and hash-tag rules
│   └── kafka-topics.md               topics, keys, events, consumers
├── 05-api/
│   ├── api-conventions.md            idempotency, errors, deadlines, retries, auth
│   ├── public-rest-api.md            gateway REST + SSE endpoints
│   └── internal-grpc-api.md          per-service gRPC packages
├── 06-flows/
│   ├── 01-queue-join-and-admission.md
│   ├── 02-hold-seats.md
│   ├── 03-checkout-and-payment-saga.md
│   ├── 04-hold-expiry.md
│   ├── 05-cancel-and-refund-saga.md
│   ├── 06-second-chance-offers.md
│   └── 07-transfer-and-gate-scan.md
├── 07-correctness/
│   ├── correctness-guarantees.md     the seven guarantees and their mechanisms
│   └── failure-scenarios.md          failure → effect → handling
├── 08-operations/
│   ├── observability.md              metrics, traces, logs, efficiency metrics
│   └── deployment-and-operability.md Compose, Kubernetes, kill switch
├── 09-testing/
│   └── test-plan.md                  T1…T13, baseline comparison, report deliverable
└── 10-planning/
    ├── build-phases.md               9 phases, ~12 weeks, test gates
    ├── open-questions.md             the 7 open questions listed in the PDF
    ├── ambiguities.md                gaps/conflicts found while documenting (needs approval)
    └── phase-2-naive-baseline.md     phase 2 plan: baseline shape, tests, k6, open items
```

## Documentation conventions

- **Cite the source.** Each document opens with a `Source:` line naming PDF pages.
- **Spec vs. interpretation.** Statements taken from the PDF are written plainly. Anything that is a reading of the PDF rather than a quote is marked **Derived:**. Anything undecided links to an `AMB-xx` or `OQ-x` entry.
- **Stable IDs.** Requirements (`FR-n`), tests (`T-n`), decisions (`ADR-nnn` in [`/ARCHITECTURE_DECISIONS.md`](../ARCHITECTURE_DECISIONS.md)), open questions (`OQ-n`) and ambiguities (`AMB-nn`) are never renumbered. Retired items get struck through, not deleted.
- **Money** is always written in paise (`*_paise BIGINT`), matching the schema.
- **Diagrams** are drawn in Mermaid so they can be diffed. The original PDF images in `assets/` remain the reference whenever a Mermaid transcription and an image disagree.

## Root-level companion files

| File | Purpose |
|---|---|
| [`/CLAUDE.md`](../CLAUDE.md) | Operating rules for AI-assisted work in this repo |
| [`/README.md`](../README.md) | Project front page (later also holds the load-test report the PDF requires) |
| [`/PROGRESS.md`](../PROGRESS.md) | Phase-by-phase status against the build plan |
| [`/ARCHITECTURE_DECISIONS.md`](../ARCHITECTURE_DECISIONS.md) | ADR log derived from the PDF |
| [`/ARCHITECTURE_REVIEW.md`](../ARCHITECTURE_REVIEW.md) | Decisions requiring approval, PDF inconsistencies, risks, spec gaps |
| [`/TASKS.md`](../TASKS.md) | Actionable backlog organised by build phase and test gate |
| [`/DOCUMENTATION_AUDIT.md`](../DOCUMENTATION_AUDIT.md) | Coverage and consistency audit of this documentation set |
