# Documentation Audit

**Date:** 2026-11-06 (final pre-commit review) · **Scope:** every file that would be committed (7 root Markdown files, 41 Markdown files under `docs/`, 4 diagram PNGs, the source PDF, `.gitignore`) · **Audited against:** `reference/SurgeTix Product Requirements & System Architecture (2).pdf` (37 pages)

**Verdict:** ✅ **Ready for the initial documentation commit.** The docs are complete against the PDF and internally consistent: 0 broken links, anchors or diagrams. They add nothing without flagging it, and contain no secrets or build artifacts. **36 ambiguities, 7 PDF open questions and 1 proposed ADR remain open for owner approval** ([ARCHITECTURE_REVIEW.md](ARCHITECTURE_REVIEW.md)). They block later phases, not this commit.

---

## 1. Method

| Step | How |
|---|---|
| Source integrity | Re-extracted the PDF in `reference/`. The text is identical to the v2 text the docs were written from (37 pages) |
| Diagram extraction | All 4 embedded diagrams extracted and read (architecture p.10, data flow p.12, seat lifecycle p.27, build plan p.36). The 3 diagrams shared with v1 are pixel-identical |
| Mermaid validation | Every ` ```mermaid ` block was parsed with **mermaid 11's own parser** (Node + jsdom) |
| DDL fidelity | All SQL in `04-data/postgres-schema.md` compared with the PDF's DDL (p.17–21) after stripping whitespace |
| Entity cross-check | Every RPC, event, topic, Redis key, metric, index, table, endpoint and status value named in the PDF was searched for in the docs |
| Fact check | 48 numeric/technical facts string-matched against the PDF text |
| Links/anchors | Script over all `.md` files: relative links, `#anchors` (explicit `<a id>` and heading slugs), page-citation range, ID definitions vs references |
| Cross-file consistency | Phase gates, counts (ADRs, ambiguities, OQs) and statuses compared across `CLAUDE.md`, `README.md`, `PROGRESS.md`, `TASKS.md`, `ARCHITECTURE_DECISIONS.md`, `ARCHITECTURE_REVIEW.md`, build-phases and test plan |
| Commit safety | `git add --dry-run .`; secret-pattern scan (private keys, AWS/GitHub/Slack/OpenAI-style tokens, `password=`/`secret=`/`api_key=` literals, email addresses); OS-junk scan; `.gitignore` review |
| Manual semantic review | Claims marked as interpretation re-read against the PDF; inaccurate ones corrected (§6) |

## 2. Automated check results

| Check | Result |
|---|---|
| Markdown files scanned | **48** |
| Broken relative links | **0** |
| Broken anchors | **0** |
| Page citations outside p.1–37 | **0** |
| Mermaid diagrams | **26 / 26 parse** (4 failures found and fixed, §6) |
| DDL vs PDF | **Identical**: 5,883 / 5,883 non-whitespace characters |
| PDF entities missing from docs | **0** across 28 RPCs, 19 events, 4 topics, 11 Redis keys, 9 metrics, 6 indexes, 24 tables, 21 endpoints, 31 status values |
| Failure rows | **19 / 19** (F1–F19) |
| FR-1…FR-23 · T1…T13 | **23 / 23 · 13 / 13** |
| Key facts not found in the PDF | **0 / 48** |
| `AMB` / `ADR` references undefined | **0 / 0** (37 AMB, 32 ADR defined) |
| Docs without a `Source:` line | **0** (the index excepted) |
| Empty or template-only files | **0**. Smallest is `fakepay-simulator.md` (≈ 2.4 KB) |
| Application code / scaffolding | **None** |
| Secrets, credentials, emails, keys | **None found** |
| Ignored or OS-junk files present | **None** |

### Re-run 2026-11-06 (Phase 2 planning change: OQ-7 → ADR-034, AMB-38 … AMB-43, phase 2 plan)

| Check | Result |
|---|---|
| Markdown files scanned | **49** (adds `docs/10-planning/phase-2-naive-baseline.md`) |
| Broken relative links / anchors | **0 / 0** |
| `AMB` / `ADR` references undefined | **0** (43 AMB, 34 ADR defined) |
| Mermaid diagrams | **27 / 28 parse** with mermaid 11.4.1 + jsdom. Both new diagrams parse. The one failure is the state diagram in `03-services/waiting-room.md`, **unchanged since phase 0**, which passed the earlier run. It may be a parser-version difference (labels such as `Lua#1`) and needs a follow-up |
| Sequence diagrams containing `;` | **0** |
| DDL in `postgres-schema.md` | **Unchanged** (only a note added outside the SQL) |
| New doc has a `Source:` line, citations within p.1–37 | ✅ |
| Files changed | Markdown only. No code, secrets or generated files |

## 3. Coverage matrix: review areas

| Review area | Where it is covered | Status |
|---|---|---|
| **All seven services** (6 services + Notifier worker; plus the FakePay simulator) | `03-services/` (8 files), `02-architecture/system-architecture.md` | ✅ |
| **Database ownership boundaries** | `system-architecture.md` → *Data ownership*; ADR-008; each service doc → *Owned data*; CLAUDE invariant 11 | ✅. Identity/Catalog columns unspecified (AMB-13) |
| **Public APIs** (21 endpoint rows) | `05-api/public-rest-api.md`, `api-conventions.md` | ✅ |
| **gRPC communication** (5 packages, 28 RPCs, deadlines, retries) | `05-api/internal-grpc-api.md`, ADR-006 | ✅. Retry-layer conflict flagged (AMB-33) |
| **Kafka events** (4 topics, 19 events, keys, ordering, outbox) | `04-data/kafka-topics.md`, ADR-005, ADR-009 | ✅. Naming conflict flagged (AMB-03) |
| **Booking workflows** (7 flows) | `06-flows/01…07`, `02-architecture/data-flow.md` | ✅ |
| **Consistency requirements** | `01-requirements/non-functional-requirements.md` (boundaries + degradation matrix) | ✅ |
| **Constraints, indexes** | `04-data/postgres-schema.md` (verbatim DDL + constraint notes) | ✅ (DDL identical) |
| **Transactions and concurrency** | Flows 2–5 and 7 (one-transaction boundaries, `SKIP LOCKED`, ordered locks, conditional updates); `07-correctness/correctness-guarantees.md` G-1…G-7; `04-data/state-machines.md` | ✅ |
| **Failure handling** | `07-correctness/failure-scenarios.md` (F1–F19) | ✅. 4 rows have no test (AMB-02) |
| **Testing scenarios** | `09-testing/test-plan.md` (T1–T13 + gates + coverage gaps) | ✅ |
| **Permanent engineering instructions** | `CLAUDE.md`: permanent work rules P1–P6, 14 invariants, implementation standards, doc conventions; temporary phase-0 rules separated | ✅ |
| **Unresolved decisions / risks** | `ARCHITECTURE_REVIEW.md` (20 decision groups, 11 inconsistencies, 14 risks), `ambiguities.md`, `open-questions.md` | ✅ |

## 4. Coverage matrix: PDF sections

| PDF section | Pages | Documented in |
|---|---|---|
| Overview | 1 | `00-overview/product-overview.md`, `README.md` |
| Goals, non-goals, success metrics | 1–3 | `00-overview/goals-and-success-metrics.md` |
| Users and key journeys | 3–4 | `00-overview/users-and-journeys.md` |
| Functional requirements FR-1…23 | 5–7 | `01-requirements/functional-requirements.md` |
| Non-functional requirements | 7–8 | `01-requirements/non-functional-requirements.md` |
| Capacity, Redis hot spot, load-test target | 8–10 | `01-requirements/capacity-estimates.md` |
| Architecture overview + diagram | 10–11 | `02-architecture/system-architecture.md`, `assets/architecture-overview.png` |
| Data flow + diagram | 11–12 | `02-architecture/data-flow.md`, `assets/data-flow.png` |
| Tech stack | 13–15 | `02-architecture/technology-choices.md`, ADRs |
| Services and responsibilities | 15–16 | `system-architecture.md`, `03-services/*` |
| Data ownership | 17 | `system-architecture.md`, ADR-008 |
| Data model (DDL) | 17–21 | `04-data/postgres-schema.md` |
| Invariants | 21–22 | `postgres-schema.md`, `correctness-guarantees.md` |
| Order state machine | 22 | `04-data/state-machines.md` |
| Redis keys | 22–24 | `04-data/redis-keys.md` |
| Kafka topics | 24 | `04-data/kafka-topics.md` |
| API design, public REST | 24–26 | `05-api/api-conventions.md`, `public-rest-api.md` |
| Internal gRPC, deadlines | 26–27 | `05-api/internal-grpc-api.md` |
| Seat lifecycle diagram | 27 | `state-machines.md`, `assets/seat-lifecycle.png` |
| Key flows 1–7 | 27–31 | `06-flows/01…07` |
| Correctness guarantees | 31–32 | `07-correctness/correctness-guarantees.md` |
| Failure scenarios | 32–34 | `07-correctness/failure-scenarios.md` |
| Observability and load testing | 34 | `08-operations/observability.md` |
| Test scenarios, deliverable | 35–36 | `09-testing/test-plan.md` |
| Build phases + diagram | 36–37 | `10-planning/build-phases.md`, `assets/build-plan.png`, `TASKS.md`, `PROGRESS.md` |
| Open questions | 37 | `10-planning/open-questions.md`, `ARCHITECTURE_REVIEW.md` |

**Uncovered PDF content:** none.

## 5. Fidelity and interpretation

| Class | Treatment |
|---|---|
| **Spec** | Written plainly with a page citation |
| **Derived** | Labelled **Derived:**: e.g. hold `SET` list, unpaired Gateway routes (†), hold/payment/saga-step/outbox state diagrams, FR → phase map, degradation matrix |
| **Gap / conflict** | Not filled. Logged as `AMB-nn`; summarised in `ARCHITECTURE_REVIEW.md` |

**Flagged additions beyond the PDF:** Phase 0 (documentation); one marked design *recommendation* in `internal-grpc-api.md`; the `docs/` layout itself (AMB-02); reviewer-assessed risks in `ARCHITECTURE_REVIEW.md` §4 (explicitly not requirements).

## 6. Corrections made in the final review

| File | Issue | Fix |
|---|---|---|
| `04-data/kafka-topics.md`, `06-flows/01-…`, `03-…`, `05-…` | 4 sequence diagrams **failed to parse** (`;` is a Mermaid statement separator) | `;` → `,` in message/note text (15 occurrences); now 26/26 parse. Rule added to CLAUDE.md |
| `02-architecture/data-flow.md` | Claimed Postgres is written "exactly" at the diagram's three marks | Noted the unmarked `StartPayment` and order-insert writes. Raised as inconsistency I-9 / AMB-28 |
| `05-api/public-rest-api.md` | Unsupported claim that the 24 h transfer cutoff returns `409` | Marked the response code as unspecified |
| `03-services/identity.md` | `verify-email` response differed from the PDF | Aligned to "200 tokens, 401" |
| `05-api/api-conventions.md` | `422` meaning broader than the PDF | Limited to what the PDF states |
| `CLAUDE.md` | Phase-0 restrictions mixed with permanent rules; implementation standards missing | Split into permanent / current-phase sections; added the PDF's implementation standards |
| Root files | No architecture review file | Created `ARCHITECTURE_REVIEW.md`; linked it from README, docs index, PROGRESS, TASKS, CLAUDE |

## 7. `.gitignore` review

**Current file is adequate for this commit.** Nothing secret, generated or ignored is present, and `git add --dry-run .` stages only docs, assets, the PDF and `.gitignore`.

Covered today: `.env` / `.env.*` (with `!.env.example`), `*.pem`, `*.key`, `secrets/`, `*.exe`, `*.test`, `coverage.out`, `node_modules/`, `dist/`, `build/`, `.vscode/`, `.idea/`, `*.log`, `.DS_Store`, `Thumbs.db`, `postgres-data/`, `redis-data/`, `kafka-data/`.

**Suggested additions for later phases (not applied, since `.gitignore` is owner-maintained):**

| Pattern | Why |
|---|---|
| `bin/` | Go binaries on Linux/macOS have no `.exe` extension |
| `*.out`, `*.prof`, `*.pprof` | Coverage and pprof output beyond `coverage.out`. **Do not ignore `*.pgo`**: PGO needs the `default.pgo` profiles committed |
| `desktop.ini` | Windows/OneDrive folder metadata (the repo lives in OneDrive) |
| `.claude/settings.local.json` | Personal assistant settings |
| `*.kubeconfig`, `kubeconfig*` | Local cluster credentials (kind/k3d) |
| k6 result folders | Raw load-test output, once a location is chosen (AMB-34) |

## 8. Re-running this audit

After any doc change:
1. links and anchors resolve;
2. page citations lie within the current PDF revision;
3. every `AMB` / `ADR` / `OQ` / `FR` / `T` / `F` reference is defined;
4. all Mermaid blocks parse (no `;` in sequence text);
5. `postgres-schema.md` SQL still matches the PDF unless an approved ADR changed it;
6. `git add --dry-run .` stages nothing secret or generated;
7. update the date and results above.
