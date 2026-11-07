# Architecture Review

**Date:** 2026-11-06 · **Reviewed:** `reference/SurgeTix Product Requirements & System Architecture (2).pdf` (37 pages) as captured in `docs/` · **Status:** 🟡 **Awaiting owner approval**. **Nothing in this file is decided.**

This file is the single place to review everything that needs your judgement before or during implementation:

1. decisions requiring approval, in the order the build plan needs them;
2. inconsistencies inside the PDF;
3. engineering risks (reviewer assessment);
4. specification gaps.

The detail behind each item lives in [`docs/10-planning/ambiguities.md`](docs/10-planning/ambiguities.md) (`AMB-nn`) and [`docs/10-planning/open-questions.md`](docs/10-planning/open-questions.md) (`OQ-n`). This file summarises and prioritises them.

> **Ground rule.** Items here are **not** requirements. Risks (§4) are the reviewer's assessment and carry no design changes. Options in the linked registers are choices for you, not recommendations that have been applied.

---

## 1. Overall assessment

The PDF is an unusually complete design. It specifies full DDL, every public endpoint and internal RPC, the Kafka topics, the order state machine, seven flows, 19 failure scenarios, 13 test scenarios and a gated build plan.

Its core correctness argument is sound and is carried by four mechanisms: conditional updates, unique constraints, idempotency keys and the outbox. It is also candid about its limits: the single-shard hot spot, QR sharing, offline gates, and the statement that tests are evidence rather than proof.

The open items are concentrated in **lifecycle edges** (partial cancellation, second chance, refund failure), **cross-service data access** (audit, event context), and **unstated parameters and wire formats**. None of them undermines the double-booking guarantee for the P0 hold/checkout path, but four of them (§2, marked **H**) can produce wrong money or inventory outcomes if implemented by guesswork.

---

## 2. Decisions requiring approval (ordered by when they block)

| # | Item | Sev | Blocks | Question in one line |
|---|---|---|---|---|
| 1 | **Documentation approval** | — | Phase 0 gate | Approve the `docs/` set as the engineering baseline |
| 2 | [AMB-02](docs/10-planning/ambiguities.md#amb-02) | M | 0 | Approve the doc layout, FR → phase mapping, and how untested FRs/failures are covered |
| 3 | [AMB-01](docs/10-planning/ambiguities.md#amb-01) | L | 0 (commit) | Keep the PDF file name `…(2).pdf` or rename it to a stable name before the first commit |
| 4 | [AMB-34](docs/10-planning/ambiguities.md#amb-34) | M | **1** | ✅ Decided 2026-11-06: single-module monorepo ([ADR-033](ARCHITECTURE_DECISIONS.md#adr-033)) |
| 5 | [OQ-3](docs/10-planning/open-questions.md) / [AMB-36](docs/10-planning/ambiguities.md#amb-36) | L | **1** | ✅ Decided 2026-11-06: hand-written `pgx` on transactional paths, `sqlc` later for CRUD ([ADR-036](ARCHITECTURE_DECISIONS.md#adr-036)) |
| 6 | [AMB-20](docs/10-planning/ambiguities.md#amb-20) | M | **1** | Redis Sentinel (single primary) vs Cluster; HA expectations under Compose; kind vs k3d |
| 7 | [OQ-7](docs/10-planning/open-questions.md) | — | **2** | ✅ Decided 2026-11-06: the same seven components without protections ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)) |
| 7a | [AMB-38](docs/10-planning/ambiguities.md#amb-38), [AMB-41](docs/10-planning/ambiguities.md#amb-41) | **H** | **2** | ✅ Decided 2026-11-06 ([ADR-035](ARCHITECTURE_DECISIONS.md#adr-035)) |
| 7b | [AMB-39](docs/10-planning/ambiguities.md#amb-39), [AMB-40](docs/10-planning/ambiguities.md#amb-40), [AMB-42](docs/10-planning/ambiguities.md#amb-42), [AMB-43](docs/10-planning/ambiguities.md#amb-43) | M | **2** | ✅ Decided 2026-11-06 ([ADR-035](ARCHITECTURE_DECISIONS.md#adr-035)) |
| 8 | [AMB-10](docs/10-planning/ambiguities.md#amb-10) | **H** | **3** | ✅ Decided 2026-11-06: 1–4 per hold, server-side; cumulative caps later ([ADR-036](ARCHITECTURE_DECISIONS.md#adr-036)) |
| 9 | [AMB-09](docs/10-planning/ambiguities.md#amb-09) | M | 3 | ✅ Decided 2026-11-06: section mode first. ⚠️ Tier mode (FR-7, P0) deferred, phase to be scheduled |
| 10 | [AMB-26](docs/10-planning/ambiguities.md#amb-26), [AMB-27](docs/10-planning/ambiguities.md#amb-27), [AMB-28](docs/10-planning/ambiguities.md#amb-28) | L–M | 3 | ✅ Decided 2026-11-06. ⚠️ AMB-26 deviates from p.4/p.29 (no alternatives yet) |
| 10a | [AMB-44](docs/10-planning/ambiguities.md#amb-44) … [AMB-51](docs/10-planning/ambiguities.md#amb-51), [AMB-03](docs/10-planning/ambiguities.md#amb-03) | L–H | **3** | ✅ Decided 2026-11-06 ([ADR-036](ARCHITECTURE_DECISIONS.md#adr-036) + amendment). Tier mode, nearby alternatives and booked-only `seatbits` are in Phase 3 (M4). Open before M4: AMB-09 tier model, AMB-26 algorithm, [AMB-52](docs/10-planning/ambiguities.md#amb-52) `SoldOut`. OQ-6 before M5 cloud runs |
| 11 | [AMB-03](docs/10-planning/ambiguities.md#amb-03), [AMB-13](docs/10-planning/ambiguities.md#amb-13), [AMB-14](docs/10-planning/ambiguities.md#amb-14), [AMB-18](docs/10-planning/ambiguities.md#amb-18), [AMB-19](docs/10-planning/ambiguities.md#amb-19), [AMB-24](docs/10-planning/ambiguities.md#amb-24) | M | 4 | Topic names; Identity/Catalog schemas; roles; verification email; secrets; Google verification |
| 12 | [AMB-22](docs/10-planning/ambiguities.md#amb-22), [AMB-29](docs/10-planning/ambiguities.md#amb-29), [OQ-2](docs/10-planning/open-questions.md), [OQ-6](docs/10-planning/open-questions.md) | M | 5 | Pre-sale numbering; one-Redis-call join; PoW difficulty; load-gen VMs/budget |
| 13 | [AMB-04](docs/10-planning/ambiguities.md#amb-04) | **H** | **6–7** | Missing order/refund state transitions |
| 14 | [AMB-11](docs/10-planning/ambiguities.md#amb-11), [AMB-25](docs/10-planning/ambiguities.md#amb-25), [AMB-31](docs/10-planning/ambiguities.md#amb-31), [AMB-33](docs/10-planning/ambiguities.md#amb-33) | M | 6 | Payment window; saga states; FakePay API; retry layers |
| 15 | [AMB-06](docs/10-planning/ambiguities.md#amb-06) | **H** | **7** | GA partial cancellation: which bucket is restored (schema change likely) |
| 16 | [AMB-07](docs/10-planning/ambiguities.md#amb-07) | **H** | **7** | Second-chance offer mechanics end to end |
| 17 | [AMB-05](docs/10-planning/ambiguities.md#amb-05), [AMB-23](docs/10-planning/ambiguities.md#amb-23), [AMB-30](docs/10-planning/ambiguities.md#amb-30), [AMB-32](docs/10-planning/ambiguities.md#amb-32), [OQ-1](docs/10-planning/open-questions.md), [OQ-4](docs/10-planning/open-questions.md) | M | 7 | Event cancellation; event context; ticket edges; refund rounding; policy defaults; second-chance transferability |
| 18 | [AMB-08](docs/10-planning/ambiguities.md#amb-08), [AMB-15](docs/10-planning/ambiguities.md#amb-15), [AMB-17](docs/10-planning/ambiguities.md#amb-17), [AMB-35](docs/10-planning/ambiguities.md#amb-35), [AMB-16](docs/10-planning/ambiguities.md#amb-16) | M | 8 | SSE owner; pause/kill semantics; audit data access; control-room UI; analytics |
| 19 | [OQ-5](docs/10-planning/open-questions.md), [AMB-37](docs/10-planning/ambiguities.md#amb-37) | L | 9 | Hosting; AuthPlatform reference |
| 20 | [AMB-12](docs/10-planning/ambiguities.md#amb-12) | M | 3–6 (per item) | Numeric parameters and wire formats, approved per phase |

**Totals:** 30 ambiguities open (4 high: AMB-04, AMB-06, AMB-07, AMB-53; incl. the new AMB-52, AMB-53), 22 approved (AMB-03, -09, -10, -26, -27, -28, -34, -36, -38 … -51), 1 resolved by PDF v2 (AMB-21). 7 PDF open questions, 2 decided (OQ-3, OQ-7). 0 proposed ADRs (ADR-024 accepted).

---

## 3. Inconsistencies inside the PDF

These are statements in the PDF that cannot all be true as written. The docs follow neither side silently; each is linked to its register entry.

| # | Statement A | Statement B | Ref |
|---|---|---|---|
| I-1 | Diagram: Kafka carries *"booking.events and checkout.events"* (p.10) | Topic table: `booking.holds`, `booking.inventory`, `booking.tickets`, `checkout.orders` (p.24) | AMB-03 |
| I-2 | `{event}` is *"a Redis Cluster hash tag"*; *"single shard"* (p.9, p.23) | Kubernetes uses **Redis Sentinel** (p.15) | AMB-20 |
| I-3 | Tech stack: *"pgx + sqlc"* chosen (p.14) | Open question: pgx **or** sqlc? (p.37) | AMB-36 |
| I-4 | *"Only UNAVAILABLE is retried"* (p.27) | Checkout saga retries `ConfirmHold` after **timeouts** (p.29) | AMB-33 |
| I-5 | *"One Redis call per queue join"* (p.7) | Join burns a nonce, does `SET NX` and `INCR`/`ZADD`; `pow:{nonce}` lacks the `{event}` tag (p.23, p.27–28) | AMB-29 |
| I-6 | Audit owned by Checkout, checks `BOOKED` seats and tickets (p.7, p.16) | *"No service reads another's tables"* (p.17) | AMB-17 |
| I-7 | Notifier owns *"SSE seat-map deltas"* (p.16) | Booking exposes `WatchSeats` server-stream; `/seats/stream` is a Gateway route (p.26–27) | AMB-08 |
| I-8 | Kill switch required per sale (p.6, p.8) | No kill-switch endpoint in the API table (p.26) | AMB-15 |
| I-9 | *"≤ 4 Postgres round trips per seat sold, hold through confirmation"* (p.3, p.7) | Flow 3 performs at least five separate write transactions across two databases: hold, `StartPayment`, order insert, payment-saved, `ConfirmHold`, plus order → `CONFIRMED` (p.28–29) | AMB-28, risk R-2 |
| I-10 | `sagas UNIQUE (kind, order_id, saga_id)` suggests a uniqueness rule (p.20) | It contains the primary key, so it constrains nothing extra | AMB-25 |
| I-11 | Admin *"control room"* in the React app (p.13) | Grafana *"Surge Control Room"* dashboard (p.34) | AMB-35 |

---

## 4. Risks (reviewer assessment, not requirements)

Likelihood/impact are the reviewer's estimate. "Mitigation in PDF" cites existing design. "Decision needed" points to what you would approve. **No mitigation beyond the PDF has been applied to the docs.**

| ID | Risk | L / I | Mitigation in PDF | Decision needed |
|---|---|---|---|---|
| R-1 | **Per-event Redis ceiling.** All queue traffic for one event hits one Redis primary, so the 1M (and especially 10M) targets may exceed it | M / H | Accepted for v1; measured in T2; `429` shedding; edge cache absorbs polls | AMB-20; revisit after T2 |
| R-2 | **Round-trip target may be unattainable as literally worded** (I-9), which would put a headline efficiency claim at risk | H / M | Single transaction per hold mode | AMB-28 (define the counting rule) |
| R-3 | **Order state machine gaps** cause legitimate transitions (second partial cancel, permanent refund failure) to be *rejected* at runtime, leaving money stuck | M / H | Conditional transitions; reconciler; audit | AMB-04 |
| R-4 | **GA partial cancellation** cannot be expressed by `hold_ga_allocations.released_at`, which threatens the "restore exactly what was taken, once" guarantee | H / H for GA cancels | Per-bucket allocations | AMB-06 (likely schema change) |
| R-5 | **Second-chance flow underspecified**. The anti-bot reservation and strict ordering depend on details not in the PDF | M / M | Reserved hold, ordered sorted set | AMB-07, OQ-4 |
| R-6 | **Audit vs data ownership**: the nightly audit, the only cross-database money/seat check, has no sanctioned data path | M / M | — | AMB-17 |
| R-7 | **PoW fairness on low-end devices**: ~1 s on a mid-range phone may be several seconds on low-end ones | M / L | Tune after measuring | OQ-2 |
| R-8 | **SSE fan-out in a Kafka-lag-scaled worker**: long-lived connections don't fit a worker that scales on lag | M / L | Graceful degradation if SSE fails | AMB-08 |
| R-9 | **Local HA fidelity**: Compose runs every database on one Postgres instance with no standby, so failover behaviour (F13) is only observable on Kubernetes | H / L | CloudNativePG on K8s | AMB-20 |
| R-10 | **Laptop resource load**: kind/k3d plus CloudNativePG, Strimzi (3 brokers), Sentinel and the observability stack can exhaust a laptop, which distorts chaos (T4) and latency results | M / M | Report states exact setup | OQ-5, OQ-6 |
| R-11 | **Secrets unspecified**: the QR-secret encryption key, webhook HMAC secret, token signing keys and their rotation | M / M | "Never sent in plain form" | AMB-19 |
| R-12 | **Baseline credibility**: the "≥ 10× lower error rate" claim depends on a baseline that reviewers accept as fair | M / M | Identical k6 profile (T6); same seven-component topology ([ADR-034](ARCHITECTURE_DECISIONS.md#adr-034)); unprotected but not sabotaged | AMB-40, AMB-43 |
| R-13 | **Schedule**: 9 gates in ~12 part-time weeks; the PDF itself says the timeline stretches rather than skipping gates | H / L | Gate rule | — (acknowledge) |
| R-14 | **Untested failure modes**: F9 Redis down, F11 edge down, F13 Postgres failover and F14 expiry stopped have no scenario | M / M | — | AMB-02 |

---

## 5. Specification gaps (summary)

- **Schemas not given:** Identity, Catalog, and any Waiting Room/Notifier persistence (AMB-13).
- **Wire formats not given:** JSON bodies, proto messages, RFC 9457 types, SSE events, QR payload, scan body, Kafka payloads (AMB-12).
- **Parameters not given:** AIMD tuning, token/key TTLs, rate limits, backoffs, breaker thresholds, idempotency retention, Kafka partitions/RF (AMB-12).
- **Flows not given:** event cancellation (AMB-05); kill switch (AMB-15); verification email (AMB-18).
- **Process not given:** repository layout (AMB-34), paging/alert routing (AMB-17).

---

## 6. Corrections made during the final documentation review

These fixed documentation errors only. No architecture was changed.

| File | Correction |
|---|---|
| `docs/04-data/kafka-topics.md`, `docs/06-flows/01-…`, `03-…`, `05-…` | 4 Mermaid sequence diagrams failed to parse (`;` is a statement separator in Mermaid). Replaced with `,` in message text; all 26 diagrams now parse |
| `docs/02-architecture/data-flow.md` | Removed the claim that Postgres is written "exactly" at the diagram's three marks; noted the unmarked `StartPayment` and order-insert writes (feeds I-9) |
| `docs/05-api/public-rest-api.md` | Removed an unsupported claim that the 24 h transfer cutoff returns `409`; marked the code unspecified |
| `docs/03-services/identity.md` | `verify-email` response aligned with the PDF row ("200 tokens, 401") |
| `docs/05-api/api-conventions.md` | `422` meaning limited to what the PDF states |
| `CLAUDE.md` | Split into permanent rules vs phase-0 rules; added the PDF's implementation standards |

---

## 7. Approval

Reply per item (e.g. *"AMB-06: add a bucket column to GA tickets"*, *"OQ-7: same services without protections"*). Each approval will be recorded in the register with the date, reflected in an ADR where architectural, propagated to all linking docs, and logged in [`PROGRESS.md`](PROGRESS.md).

- [ ] §2 items 1–6 (needed before phase 1)
- [ ] §2 item 7 (needed before phase 2)
- [ ] §3 inconsistencies acknowledged, each with a chosen side
- [ ] §4 risks acknowledged (accept / mitigate / defer)
