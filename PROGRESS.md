# Progress

*Tracks status against the 9-phase build plan (PDF p.36) plus phase 0 (documentation). A phase moves on **only when its gate passes** (PDF p.37).*

**Last updated:** 2026-11-06
**Current phase:** **0 — Documentation** → status: **Complete, awaiting owner approval**

## Phase status

| # | Phase | Planned weeks | Gate | Status | Gate evidence |
|---|---|---|---|---|---|
| 0 | **Documentation** (not in PDF) | — | Owner approves docs + ambiguities register | 🟡 **Awaiting approval** | [DOCUMENTATION_AUDIT.md](DOCUMENTATION_AUDIT.md) |
| 1 | Foundations | Week 1 | CI green | ⚪ Not started (blocked by phase 0, OQ-3, AMB-34) | — |
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

## Decisions needed before phase 1

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
| — | — | *(none yet)* | — |

## Change log

| Date | Change |
|---|---|
| 2026-11-06 | Phase 0: documentation repository initialised from PDF v2. Citations moved from v1 to v2 after the source was replaced mid-task |
| 2026-11-06 | Final pre-commit review: fixed 4 Mermaid parse errors and 4 inaccurate statements, restructured CLAUDE.md into permanent and phase rules, added ARCHITECTURE_REVIEW.md, refreshed DOCUMENTATION_AUDIT.md |
