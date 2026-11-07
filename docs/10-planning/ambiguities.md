# Ambiguities Register — Awaiting Owner Approval

*Source: gaps, conflicts and silences found while documenting `reference/SurgeTix Product Requirements & System Architecture (2).pdf`. Questions the PDF itself raises are in [open-questions.md](open-questions.md).*

**Nothing in this register has been decided.** The documentation set does not fill these gaps. Where a doc needs the answer, it links here. The **Options** listed are possibilities for you to choose from, not decisions; "Leaning" notes where the PDF text tilts one way.

**Status key:** 🟡 Awaiting approval · 🟢 Resolved by PDF v2 (kept for traceability) · ✅ Approved (record date + decision)

**Severity:** **H** blocks correctness or a phase gate · **M** shapes an interface or schema · **L** detail or wording.

## Summary

| ID | Topic | Sev | Blocks phase | Status |
|---|---|---|---|---|
| [AMB-01](#amb-01) | Source document revision / file name | L | — | 🟡 |
| [AMB-02](#amb-02) | No explicit documentation spec; FR → phase and test gaps | M | 0 | 🟡 |
| [AMB-03](#amb-03) | Kafka topic names: diagram vs table | M | 4 | ✅ 2026-11-06 (ADR-036) |
| [AMB-04](#amb-04) | Order and refund state-machine gaps | **H** | 6–7 | 🟡 |
| [AMB-05](#amb-05) | `EVENT_CANCELLED` refunds have no flow or API | M | 7 | 🟡 |
| [AMB-06](#amb-06) | GA partial cancellation: which bucket is restored | **H** | 7 | 🟡 |
| [AMB-07](#amb-07) | Second-chance offer mechanics | **H** | 7 | 🟡 |
| [AMB-08](#amb-08) | Who serves SSE seat-map deltas | M | 8 | 🟡 |
| [AMB-09](#amb-09) | "Price tier" has no data model | M | 3 | ✅ 2026-11-06 (ADR-036); tier data model before M4 |
| [AMB-10](#amb-10) | Purchase limits: per hold, per user, per payment method | **H** | 3 / 5 | ✅ 2026-11-06 (ADR-036) |
| [AMB-11](#amb-11) | Payment window length and the `FAILED` trigger | M | 6 | 🟡 |
| [AMB-12](#amb-12) | Unspecified numeric parameters and wire formats | M | 3–6 | 🟡 |
| [AMB-13](#amb-13) | Identity, Catalog, Waiting Room schemas not specified | M | 4 | 🟡 |
| [AMB-14](#amb-14) | Admin and staff role model | M | 4 | 🟡 |
| [AMB-15](#amb-15) | Pause / resume / kill-switch semantics; no kill endpoint | M | 8 | 🟡 |
| [AMB-16](#amb-16) | "Analytics" consumer is undefined | L | — | 🟡 |
| [AMB-17](#amb-17) | Nightly audit: "ledger", cross-DB reads, paging | M | 8 | 🟡 |
| [AMB-18](#amb-18) | Who sends verification emails | L | 4 | 🟡 (narrowed by v2) |
| [AMB-19](#amb-19) | Secret and key management | M | 4 / 6 | 🟡 |
| [AMB-20](#amb-20) | Redis Sentinel vs Cluster; HA under Compose | M | 1 / 9 | 🟡 |
| [AMB-21](#amb-21) | Edge cache / CDN product | — | — | 🟢 |
| [AMB-22](#amb-22) | Pre-sale window and ticket numbering | M | 5 | 🟡 |
| [AMB-23](#amb-23) | Event context in Checkout and Booking | M | 6–7 | 🟡 |
| [AMB-24](#amb-24) | Google sign-in and email verification | L | 4 | 🟡 |
| [AMB-25](#amb-25) | Saga states, step names, saga uniqueness | M | 6 | 🟡 |
| [AMB-26](#amb-26) | "Nearby alternatives" and the no-adjacent-run case | L | 3 | ✅ 2026-11-06 (ADR-036); algorithm before M4 |
| [AMB-27](#amb-27) | Hold transitions: release while `PAYMENT_PENDING` | M | 3 / 6 | ✅ 2026-11-06 (ADR-036) |
| [AMB-28](#amb-28) | What counts as a "Postgres round trip per seat sold" | L | 3 | ✅ 2026-11-06 (ADR-036) |
| [AMB-29](#amb-29) | "One Redis call per queue join" vs several operations | M | 5 | 🟡 |
| [AMB-30](#amb-30) | Ticket transition edge cases | L | 7 | 🟡 |
| [AMB-31](#amb-31) | FakePay API surface | M | 6 | 🟡 |
| [AMB-32](#amb-32) | Refund amount per ticket and rounding | M | 7 | 🟡 |
| [AMB-33](#amb-33) | RPC retry rule vs saga retries | L | 6 | 🟡 |
| [AMB-34](#amb-34) | Repository / code layout | M | 1 | ✅ 2026-11-06 (ADR-033) |
| [AMB-35](#amb-35) | Admin control room: Grafana vs React | L | 8 | 🟡 |
| [AMB-36](#amb-36) | OQ-3 vs Tech stack (`pgx + sqlc`) | L | 1 | ✅ 2026-11-06 (ADR-036) |
| [AMB-37](#amb-37) | "AuthPlatform" reference | L | — | 🟡 |
| [AMB-38](#amb-38) | Naive baseline: inventory model, tables, migrations, dataset | **H** | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-39](#amb-39) | Naive baseline: request contract (route, identity, idempotency, codes) | M | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-40](#amb-40) | Naive baseline: Gateway → Booking transport before gRPC | M | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-41](#amb-41) | Naive baseline: isolating the unsafe path; code, k6 and results locations | **H** | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-42](#amb-42) | Phase 2 metrics: no Prometheus/Grafana exists yet | M | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-43](#amb-43) | T6 scope, scale and environment in phase 2 | M | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-44](#amb-44) | Phase 3 entry point, caller identity and k6 target before gRPC/JWT | **H** | 3 | ✅ 2026-11-06 (ADR-036) |
| [AMB-45](#amb-45) | Inventory provisioning before Catalog; seat id format; GA bucket split | M | 3 | ✅ 2026-11-06 (ADR-036) |
| [AMB-46](#amb-46) | Phase 3 gate scale: T1 and T7 on local hardware | **H** | 3 | ✅ 2026-11-06 (ADR-036) ⚠️ |
| [AMB-47](#amb-47) | Outbox in phase 3: topic names, envelope, payload format | M | 3 | ✅ 2026-11-06 (ADR-036) |
| [AMB-48](#amb-48) | Idempotency mechanics: transaction boundary, stale claims, stored outcomes, hashing, DELETE | M | 3 | ✅ 2026-11-06 (ADR-036) |
| [AMB-49](#amb-49) | `seatbits` contents and maintenance vs inline reclaim | M | 3 | ✅ 2026-11-06 (ADR-036); implemented in Phase 3 M4 |
| [AMB-50](#amb-50) | Inline reclaim of a partly reclaimed old hold | M | 3 | ✅ 2026-11-06 (ADR-036) ⚠️ |
| [AMB-51](#amb-51) | Migration execution; creating phase 6–7 tables in phase 3 | L | 3 | ✅ 2026-11-06 (ADR-036) |
| [AMB-52](#amb-52) | `SoldOut` emission condition | M | 3 (M4) | 🟡 |
| [AMB-53](#amb-53) | GA ticket price source and hold-amount consistency | **H** | 3 (M3) | 🟡 |
| [AMB-54](#amb-54) | Aggregate version on events: no mechanism in the schema or envelope v1 | M | 4 (before P4-5) | 🟡 |

---

<a id="amb-01"></a>
## AMB-01 — Source document revision / file name · L · 🟡

**Evidence.** The task first referenced `reference/SurgeTix-Architecture.pdf`. The repo then contained `…(1).pdf` (33 pages), which was replaced by `…(2).pdf` (37 pages). Both carry the same date (Nov 4, 2026) and author, and neither has a version label. v2 adds the *Data flow* (p.11–12) and an expanded *Tech stack* (p.13–15). All other text and the three original diagrams are identical (verified by text diff and pixel comparison).
**Why it matters.** Page citations across the docs refer to v2.
**Options.** (a) Keep the file name as is. (b) Rename to a stable name, e.g. `reference/SurgeTix-Architecture-v2.pdf`, and update citations. (c) Add a version number inside future revisions.

<a id="amb-02"></a>
## AMB-02 — No explicit documentation spec; FR → phase and test gaps · M · 🟡

**Evidence.** The PDF has no section that prescribes documentation structure. Its only documentation deliverable is *"a short report in the README"* (p.36). The `docs/` layout is therefore **a proposal**. Separately: FR-1/2/3 are mapped to phase 4, FR-21 to phase 6, FR-19 to phase 8 and FR-23 to phase 3 by best fit, because the build plan does not name them. FR-1, 2, 16, 19, 20, 21, 23 and failures F9, F11, F13, F14 have no numbered test scenario.
**Options.** (a) Approve the layout and mappings as documented. (b) Adjust the mappings. (c) Add test IDs (e.g. T14+) for the uncovered failures, or accept integration/e2e coverage.

<a id="amb-03"></a>
## AMB-03 — Kafka topic names: diagram vs table · M · ✅ Approved 2026-11-06

**Evidence.** The architecture diagram (p.10) labels Kafka *"booking.events and checkout.events"*. The topic table (p.24) defines `booking.holds`, `booking.inventory`, `booking.tickets` and `checkout.orders`, each with its own key and ordering.
**Leaning.** The table, which carries the keys and ordering rules. The docs follow it.
**Options.** (a) Table is authoritative; the diagram label is shorthand. (b) Two topics as in the diagram (this would lose per-entity keying).

**✅ Decision (owner, 2026-11-06):** use the PDF's topic table names (option a). Booking's phase 3 events go to **`booking.holds`** (key `hold_id`) and **`booking.inventory`** (key `event_id:section`; `SoldOut` keyed by `event_id`). Event names are stable, and payloads are versioned JSON with event ids ([AMB-47](#amb-47)). [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)
**Why.** The table carries the per-entity keys and ordering rules (p.24). The diagram's two-topic label would lose them.
**Scope.** The decision names only the two Phase 3 topics. The table's other topics (`booking.tickets`, `checkout.orders`) follow the same table and are confirmed when their phases start (6–7).

<a id="amb-04"></a>
## AMB-04 — Order and refund state-machine gaps · H · 🟡

**Evidence.** The order transition table (p.22) and refund enum (p.20) leave these cases undefined:
1. A **second partial cancellation**: `PARTIALLY_REFUNDED → PARTIALLY_REFUNDED` is not listed.
2. The **order status while a cancellation refund is in flight** (it stays `CONFIRMED`? `REFUND_PENDING` is only defined for `CONFIRM_REJECTED`).
3. A refund that **permanently fails**: when `refunds.status = FAILED` is entered (the saga *"retries with backoff and alerts after 1 hour"*) and what the order becomes.
4. Whether `CONFIRM_REJECTED` refunds pass through `TICKETS_VOIDED` (there are no tickets).
5. A **0 % policy refund** (amount 0, `CHECK >= 0` allows it): does the order reach `REFUNDED` with `refunded_paise = 0`?
6. When `refunds.status = SENT` is set (sent to FakePay vs accepted by FakePay).
**Why it matters.** "Any other transition is rejected" (p.22), so missing edges become runtime rejections.

<a id="amb-05"></a>
## AMB-05 — `EVENT_CANCELLED` refunds have no flow or API · M · 🟡

**Evidence.** `refunds.reason` allows `'EVENT_CANCELLED'` (p.20), and capacity mentions refunds *"bursty after event changes"* (p.9). No admin endpoint, flow or test covers cancelling an event.
**Options.** (a) Out of scope for v1; keep the enum value reserved. (b) Add an admin "cancel event" flow (bulk refund saga), which needs a spec.

<a id="amb-06"></a>
## AMB-06 — GA partial cancellation: which bucket is restored · H · 🟡

**Evidence.** GA tickets have `seat_id NULL` and no bucket column (p.18). `hold_ga_allocations` records quantity per bucket with a single `released_at`, which is set once for the whole row (p.18). Cancelling **some** GA tickets of a multi-bucket hold has no defined bucket to increment, and `released_at` cannot express a partial restore.
**Options.** (a) Restore to any bucket (e.g. the first allocation row) and track the partial quantity elsewhere. (b) Add a bucket reference to GA tickets. (c) Add a released-quantity column. Each one changes the schema and needs approval.

<a id="amb-07"></a>
## AMB-07 — Second-chance offer mechanics · H · 🟡

**Evidence.** Flow 6 (p.30–31) says the Waiting Room pops the next user, issues a 5-minute section-scoped token, and *"the returned seats are reserved for that user with a hold created by the offer"*. Not specified:
- which service/RPC creates that hold (Booking's hold RPCs are Gateway-facing; `waitingroom.v1.OfferSecondChance` is listed but its caller is unspecified);
- the reserved hold's duration (5 vs 10 min);
- granularity (per seat, batch or section) and how many seats one user is offered (up to 4?);
- how the Notifier learns of the offer (there is no Waiting Room topic);
- how an unclaimed offer *"moves to the next person"* (hold expiry → `InventoryReturned`, or a Waiting Room timer).

<a id="amb-08"></a>
## AMB-08 — Who serves SSE seat-map deltas · M · 🟡

**Evidence.** The Notifier owns *"SSE seat-map deltas"* (p.16), and the data flow shows *"Notifier: SSE seat delta"* (p.12). The public route `GET …/seats/stream` sits behind the Gateway (p.26), and `booking.v1` has a server-stream `WatchSeats` (p.27).
**Leaning (v2).** The Notifier holds the SSE connections, fed from `booking.inventory`.
**Open.** Then what is `WatchSeats` for, and how does the Gateway route `/seats/stream` to a worker?

<a id="amb-09"></a>
## AMB-09 — "Price tier" has no data model · M · ✅ Approved 2026-11-06 (tier mode scheduled in Phase 3 M4)

**Evidence.** FR-2 lists *"price tiers"*, and best-available accepts *"section or tier"* (p.5, p.25). The `seats` table has only `price_paise` and no tier column (p.17).
**Options.** (a) Tier = distinct `price_paise` value. (b) Add a `tier` column copied from Catalog's layout.

**✅ Decision (owner, 2026-11-06):** section-based pricing initially. Best-available ships in **section** mode, and prices come from `seats.price_paise` exactly as in the PDF schema. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** The PDF schema has no tier column (p.17). Adding one would be a schema deviation needing its own ADR, and section mode exercises the same `SKIP LOCKED` + adjacency algorithm.
**⚠️ Conflict to keep visible.** FR-7 (P0) says *"N seats in a section **or price tier**"* (p.5, p.25). Tier mode is **deferred, not dropped**: FR-7 is not complete until tier mode exists. The owner must schedule it (candidate: phase 4, when Catalog's price tiers from FR-2 exist).

**✅ Follow-up decision (owner, 2026-11-06):** section-based pricing stays the **initial** implementation. **Tier pricing is a P0 requirement and stays in the Phase 3 completion criteria.** It is scheduled for **M4** ([phase 3 plan](phase-3-booking-core.md) §5.3, §14). [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)
**Still to decide before M4:** the tier data model. Options:
- (a) a tier is a distinct `price_paise` value (no schema change);
- (b) a `tier` column copied from the layout (a schema deviation needing an ADR).

Also whether a tier spans sections.

<a id="amb-10"></a>
## AMB-10 — Purchase limits · H · ✅ Approved 2026-11-06

**Evidence.** FR-6: holds of *"1–4 seats"*. FR-2: organizer sets a *"per-user limit"*. FR-18: *"purchase caps per account and payment method"*. Capacity: *"max 4 seats per buyer"* (p.8).
**Unclear.** Is 4 a fixed system maximum or the default of the per-event limit? Is the cap per hold or cumulative per user across orders (and which service enforces cumulative caps across Booking and Checkout)? Does FakePay have a "payment method" identity to cap on? Does the limit apply to GA quantity too?

**✅ Decision (owner, 2026-11-06):** minimum **1**, maximum **4** seats per hold, **enforced server-side** (never only in a client). Cumulative per-user and purchase limits are **deferred** to a later phase. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** FR-6 fixes *"1–4 seats"* per hold (p.5). The cumulative caps belong to features not yet built: the organiser's *"per-user limit"* comes from Catalog (FR-2, phase 4) and *"purchase caps per account and payment method"* from FR-18 (phase 5).
**Applied to every mode (derived, confirm):** `len(seat_ids)`, `best_available.quantity` and `ga.quantity` must all be integers 1–4. This follows from the owner's required tests for zero, negative and > 4 quantities. Required tests: 0, negative, 5+, non-integer, and each valid value 1–4, per mode.

**✅ Confirmed (owner, 2026-11-06):** the 1–4 limit applies to **all three modes**: reserved seats (`len(seat_ids)`), best-available (`quantity`) and GA (`quantity`). It is enforced server-side, and invalid quantities (0, negative, > 4, non-integer) are tested in every mode. Cumulative per-user caps stay deferred. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)

**✅ Confirmed (owner, 2026-11-06, M1-T7):** the quantity must be a plain JSON integer (`2.0`, `2e0`, fractions, strings, booleans and `null` are rejected). Section names are at most 16 bytes and seat ids at most 24 bytes, the limits of the T3 generator. These are **Phase 3 constraints**, to revisit when Catalog layouts are introduced in phase 4.

<a id="amb-11"></a>
## AMB-11 — Payment window length and the `FAILED` trigger · M · 🟡

**Evidence.** `StartPayment` extends the hold *"to cover the payment window, at most 10 more minutes"* (p.29). `PENDING_PAYMENT → FAILED` happens on *"payment failed or payment window expired"* (p.22).
**Unclear.** The exact window; which component detects window expiry for the **order** (the hold expiry worker is in Booking); and whether `FAILED` triggers an explicit `ReleaseHold` or waits for expiry.

<a id="amb-12"></a>
## AMB-12 — Unspecified numeric parameters and wire formats · M · 🟡

The PDF fixes many numbers (10-min hold, 15-min token, 5-s lease, 2-s expiry tick, 1,000 batch, 32 buckets, LIMIT 40, 1-s deadline, 30-s QR window, 2-min nonce TTL, 2-s + jitter snapshot TTL). **These are not given:**
- AIMD increment, decrease trigger thresholds (booking p99, DB pool wait), tick interval, rate bounds;
- queue ticket `exp`; `wr:{event}:user:{user}` TTL; JWT access/refresh TTLs; JWKS cache TTL and rotation;
- rate-limit values per scope; Gateway behaviour when Redis-based limits are unavailable;
- `idempotency_keys.expires_at` retention;
- outbox retry backoff, `DEAD` threshold; saga backoff schedule; circuit-breaker thresholds; FakePay timeouts;
- the throttle and data source for the "throttled DB reads" seat-map fallback;
- Kafka partitions, replication factor, `min.insync.replicas`, retention; event payload serialisation; trace-context propagation through the outbox;
- RFC 9457 `type` URIs; JSON schemas; SSE event format; QR payload format; scan request body; proto messages;
- cost model for "estimated cloud cost per 1,000 tickets";
- patch versions of tools and libraries.
**Proposal.** Fix these in phase-specific design notes, each submitted for approval before its phase.

<a id="amb-13"></a>
## AMB-13 — Identity, Catalog, Waiting Room schemas not specified · M · 🟡

**Evidence.** Only table names are given for Identity (`users`, `credentials`, `oauth_accounts`, `signing_keys`) and Catalog (`events`, `venues`, `seat_layouts`) (p.22). It is unknown whether the Waiting Room or Notifier have Postgres databases (they need somewhere for `processed_events` if they dedupe consumption). The mechanism of `PublishSale` (an RPC to Booking, or an event) is also unspecified. *"In every service that writes state"* does not list the services.

<a id="amb-14"></a>
## AMB-14 — Admin and staff role model · M · 🟡

**Evidence.** Endpoints require *"admin JWT"* and *"staff JWT"* (p.26). Nothing says how roles are stored, granted, or encoded as claims, or whether staff are scoped to an event/venue.

<a id="amb-15"></a>
## AMB-15 — Pause / resume / kill-switch semantics · M · 🟡

**Evidence.** `POST …/sale:pause` and `…/sale:resume` are listed (p.26). A *"kill switch per sale"* is required (p.6, p.8), but no endpoint is listed. Not stated: what pause stops (admission only? holds? checkout?), whether existing holds and payments continue, how pause differs from kill, which service owns the state (Catalog config vs Waiting Room), and the full URL prefix.

<a id="amb-16"></a>
## AMB-16 — "Analytics" consumer is undefined · L · 🟡

**Evidence.** "Analytics" is a consumer of three topics (p.24) and NFR-AVL mentions it degrading (p.8). No analytics component exists among the 6 + 1.
**Options.** (a) A placeholder consumer group, out of scope for v1. (b) Prometheus/Grafana covers it.

<a id="amb-17"></a>
## AMB-17 — Nightly audit: "ledger", cross-DB reads, paging · M · 🟡

**Evidence.** FR-22: *"ledger totals match FakePay"*, but no ledger table exists. The audit is owned by Checkout (p.16), yet it checks `BOOKED` seats and tickets in `booking_db`, while *"no service reads another's tables"* (p.17). *"Page the owner"* (p.34) has no paging tool.
**Options.** The ledger is `payments` + `refunds` sums · Booking exposes an audit RPC · the audit runs as a privileged job exempt from the ownership rule (needs an ADR).

<a id="amb-18"></a>
## AMB-18 — Who sends verification emails · L · 🟡 (narrowed by v2)

**Resolved by v2.** Email delivery uses **Mailpit** (p.15).
**Still open.** The Notifier sends *ticket and refund* emails (FR-21). Email verification (FR-1) belongs to Identity. Does Identity send directly via SMTP/Mailpit, or emit an event to the Notifier?

<a id="amb-19"></a>
## AMB-19 — Secret and key management · M · 🟡

**Evidence.** `qr_secret_enc` is encrypted, but the encryption key, algorithm and rotation are unspecified. The same applies to JWT signing-key rotation, the FakePay webhook HMAC secret, and the signing key for queue tickets and access tokens (Waiting Room). There is no secrets store for Compose or Kubernetes.

<a id="amb-20"></a>
## AMB-20 — Redis Sentinel vs Cluster; HA under Compose · M · 🟡

**Evidence.** The Redis keys section calls `{event}` *"a Redis Cluster hash tag"* (p.23), and capacity talks of a *"single shard"* (p.9). The Tech stack specifies **Redis Sentinel** on Kubernetes (p.15), which is replication with failover and no sharding. The failure table promotes a *"synchronous standby"* for Postgres (p.33). That is provided by CloudNativePG on Kubernetes, but none is described for Compose. Also *"kind or k3d"* is undecided.
**Options.** (a) v1 runs a single Redis primary (Sentinel for HA); hash tags are kept so a later move to Cluster needs no key change. (b) Run Redis Cluster.

<a id="amb-21"></a>
## AMB-21 — Edge cache / CDN product · 🟢 Resolved by v2

**Resolution (p.13).** **NGINX `proxy_cache`, 1 s microcache** locally; a CDN such as **Cloudflare** if hosted (ties to [OQ-5](open-questions.md)). Kept so links stay valid.

<a id="amb-22"></a>
## AMB-22 — Pre-sale window and ticket numbering · M · 🟡

**Evidence.** Pre-sale arrivals get random numbers at sale start, and later arrivals get `INCR` numbers (p.28). Not stated: when the waiting room opens; how shuffled numbers and `seq` relate (does `seq` start after the pre-sale count, so later joiners queue behind every pre-sale user?); and whether a pre-sale ticket's `number` is unknown until start (the join returns `201 {ticket, number}`).

<a id="amb-23"></a>
## AMB-23 — Event context in Checkout and Booking · M · 🟡

**Evidence.** `orders` has no `event_id` (p.19), yet quotes need the event's refund policy and start time. Booking's transfer cutoff (*"24 hours of the event"*) needs the event start time, which lives in Catalog. How each service obtains it (`GetHold` → event → `GetEvent`, data copied at publish, …) is unspecified.

<a id="amb-24"></a>
## AMB-24 — Google sign-in and email verification · L · 🟡

**Evidence.** FR-1 requires email verification before queueing. It is unstated whether a Google account's verified email satisfies this.

<a id="amb-25"></a>
## AMB-25 — Saga states, step names, saga uniqueness · M · 🟡

**Evidence.** `sagas.state` is free text (p.20). Only `CONFIRM_HOLD` and `SEND_REFUND` are named as steps. `UNIQUE (kind, order_id, saga_id)` contains the primary key and so constrains nothing beyond it, which means an order could have two `REFUND` sagas (needed for repeated partial cancellations?) or two `CHECKOUT` sagas.

<a id="amb-26"></a>
## AMB-26 — "Nearby alternatives" and the no-adjacent-run case · L · ✅ Approved 2026-11-06 (alternatives scheduled in Phase 3 M4)

**Evidence.** A `409` lists *"suggested alternatives"* / *"nearby alternatives"* (p.4, p.29), but the algorithm is unspecified. For best-available, the behaviour when the 40 `SKIP LOCKED` candidates contain no run of N adjacent seats (retry? non-adjacent? 409?) is also unspecified.

**✅ Decision (owner, 2026-11-06):** `409 Conflict` **lists the unavailable seats** but includes **no alternative-seat suggestions**. Best-available with no adjacent run of N among its candidates also returns `409`. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** The alternatives algorithm is unspecified, and keeping it out of the hold transaction keeps the correctness core minimal. Best-available already offers the PDF's other recovery, *"one tap for best-available"* (p.4).
**⚠️ Deviation recorded.** The PDF says *"409 with nearby alternatives"* (p.4, an unhappy path that *"must feel clean"*) and *"409 listing unavailable seats with suggested alternatives"* (p.29). This is an approved deviation in ADR-036. Alternatives are deferred, not waived, and must be revisited before the full demo (phase 8).

**✅ Follow-up decision (owner, 2026-11-06):** the **initial** conflict response stays `409` without suggestions (M1–M3). **Nearby alternatives are an explicit PDF requirement and stay in the Phase 3 completion criteria.** They are scheduled for **M4** (plan §11.4). They are computed read-only after rollback, are never held, and are stored with the deterministic `409`. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)
**Still to decide before M4:** the algorithm (e.g. up to *k* same-size runs in the same section ranked by distance, or one best-available suggestion), *k*, and the response shape.

<a id="amb-27"></a>
## AMB-27 — Hold transitions: release while `PAYMENT_PENDING` · M · ✅ Approved 2026-11-06

**Evidence.** `RELEASED` exists (p.18), and *"failures release seats"* (p.34). Not stated: whether `DELETE /holds/{id}` is allowed once checkout has started; whether a failed payment explicitly releases the hold or lets it expire; and the exact release transaction (assumed to mirror expiry).

**✅ Decision (owner, 2026-11-06):** in phase 3 only **`ACTIVE`** holds are releasable. **Derived:** that means `status = 'ACTIVE' AND expires_at > now()`; an expired hold is left to expiry. Release of `PAYMENT_PENDING` holds is decided with checkout (phase 6). [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** `PAYMENT_PENDING` exists only after `StartPayment` (phase 6). Deciding its release rules now would be speculative.

<a id="amb-28"></a>
## AMB-28 — What counts as a "Postgres round trip per seat sold" · L · ✅ Approved 2026-11-06

**Evidence.** Target ≤ 4, *"hold through confirmation"* (p.3). Unclear: per seat or per order (2.5 seats/order); whether `checkout_db` trips count; whether idempotency-claim and outbox statements count separately or as one transaction round trip.
**Added in final review.** Flow 3 (p.28–29) implies separate write transactions for the hold, `StartPayment`, the order insert, payment-saved, `ConfirmHold` and order → `CONFIRMED`. Counted naively that exceeds 4, so the counting rule decides whether the target is achievable ([ARCHITECTURE_REVIEW](../../ARCHITECTURE_REVIEW.md) I-9, R-2).

**✅ Decision (owner, 2026-11-06):** measure **Booking database (`booking_db`) round trips per seat sold**, counted by a pgx query tracer. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** Booking is the service phase 3 builds. A tracer counts actual network round trips (a pgx batch counts once), not SQL statements.
**Note.** The PDF target (≤ 4, *"hold through confirmation"*, p.3/p.8) can be fully evaluated only once confirmation exists (phase 6). Phase 3 reports the hold-path share.

<a id="amb-29"></a>
## AMB-29 — "One Redis call per queue join" vs several operations · M · 🟡

**Evidence.** NFR-EFF says *"one Redis call per queue join"*. The join burns the nonce (`pow:{nonce}`), does `SET NX` on the user key and `INCR`/`ZADD`. The diagram says *"SET NX, INCR (one per join)"* (p.12). A single Lua script could do all three, but `pow:{nonce}` has no `{event}` hash tag, so under Cluster it may live in another slot.
**Options.** (a) Rename to `pow:{event}:{nonce}`. (b) Treat the nonce burn as a separate call. (c) Read "one call" as one script for the queue keys only.

<a id="amb-30"></a>
## AMB-30 — Ticket transition edge cases · L · 🟡

Can a `CHECKED_IN` ticket be cancelled or refunded? Can a ticket with a pending refund be transferred? Does a transferred ticket's refund go to the original payer (the order belongs to the original buyer)? None of these is stated.

<a id="amb-31"></a>
## AMB-31 — FakePay API surface · M · 🟡

**Evidence.** Capabilities are listed (intents, hosted payment page, signed webhooks, refunds, idempotency keys, injected delays, failures and lost webhooks, status polling, totals; p.14, p.16, p.32). There are no endpoints, payloads, signature format, fault-configuration interface or totals endpoint.

<a id="amb-32"></a>
## AMB-32 — Refund amount per ticket and rounding · M · 🟡

**Evidence.** Refund % follows the policy (p.6), and `refund_items.amount_paise` is per ticket (p.20). The per-ticket base (seat price? order amount ÷ tickets?), the rounding rule for 50 % of odd paise, and which timestamp decides the policy band (quote time vs request time vs saga time) are unspecified.

<a id="amb-33"></a>
## AMB-33 — RPC retry rule vs saga retries · L · 🟡

**Evidence.** *"Only `UNAVAILABLE` is retried, with jittered backoff"* (p.27), yet the checkout saga retries `ConfirmHold` after **timeouts** too (p.29).
**Leaning.** The rule applies to the RPC client layer, and the saga has its own durable retry. Needs confirmation.

<a id="amb-34"></a>
## AMB-34 — Repository / code layout · M · ✅ Approved 2026-11-06

**Resolved by v2.** CI is **GitHub Actions** (p.15).
**Still open.** Monorepo vs multi-repo; Go module layout (one module or one per service); where `.proto` files live; where the naive baseline, FakePay, k6 scripts, React app, Kubernetes manifests and migrations live. The PDF gives no repository structure.
**✅ Decision (owner, 2026-11-06):** a single Go module monorepo: `cmd/<service>` per binary, `internal/platform` for shared infrastructure, `internal/<service>` added as each phase needs it. Recorded as [ADR-033](../../ARCHITECTURE_DECISIONS.md#adr-033). **Still open within it:** where `.proto` files, the naive baseline, FakePay, k6 scripts, the React app, Kubernetes manifests and migrations live. Decide these when the owning phase starts. For the naive baseline, k6 scripts and results, see [AMB-41](#amb-41).

<a id="amb-35"></a>
## AMB-35 — Admin control room: Grafana vs React · L · 🟡

**Evidence.** The Tech stack lists an *"admin control room"* page in the React app (p.13). Observability names the Grafana *"Surge Control Room"* dashboard (p.34). FR-20 combines live stats and controls.
**Options.** (a) React page for the controls plus Grafana for the charts. (b) React page embeds Grafana. (c) Grafana only, with controls via the API.

<a id="amb-36"></a>
## AMB-36 — OQ-3 vs Tech stack (`pgx + sqlc`) · L · ✅ Approved 2026-11-06

**Evidence.** The Tech stack names *"pgx + sqlc; goose migrations"* (p.14) as chosen. Open Questions (p.37) still asks the same question, with the proposal *"sqlc for CRUD, hand-written SQL for the hold, GA and refund paths"*.
**Leaning.** The proposal is the decision (sqlc for CRUD and hand-written pgx SQL on hot paths); confirm to close OQ-3.

**✅ Decision (owner, 2026-11-06):** **hand-written `pgx`** for every transactional booking path (holds, best-available, GA, release, expiry, idempotency, outbox). **`sqlc` later** for ordinary CRUD. Closes [OQ-3](open-questions.md). [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036), [ADR-024](../../ARCHITECTURE_DECISIONS.md#adr-024)
**Why.** These paths depend on exact SQL shape: CTEs with `ORDER BY … FOR UPDATE`, `SKIP LOCKED`, fenced conditional updates, rows-affected checks and pgx batching. This is also the PDF's own proposal (p.37).

<a id="amb-37"></a>
## AMB-37 — "AuthPlatform" reference · L · 🟡

**Evidence.** The Tech stack's Auth row: *"Shows the security depth from AuthPlatform without a separate project"* (p.14). AuthPlatform is not defined in the document. It appears to be a separate prior project. Confirm whether any code or design is to be reused from it, or whether the mention is context only.

<a id="amb-38"></a>
## AMB-38 — Naive baseline: inventory model, tables, migrations, dataset · H · ✅ Approved 2026-11-06

**Evidence.** Phase 2 is *"check-then-book; k6 baseline runs"* with the gate *"oversell seen"* (p.36). Nothing more is specified. T6 runs T1–T3 against the baseline, and T1 is reserved seating (*5,000 concurrent buyers on the same 500 seats*). GA contention (T7) belongs to phase 3. The real `booking_db` schema (p.17–21) has `one_live_ticket_per_seat`, `bookings.hold_id UNIQUE` and seat `CHECK`s, which would block the oversell, and invariant 4 forbids weakening them ([ADR-034](../../ARCHITECTURE_DECISIONS.md#adr-034)). CLAUDE.md said the schema starts in phase 3 with `goose`.
**Questions and options.**
1. *Inventory:* (a) reserved seats only (matches T1); (b) seats and GA.
2. *Tables:* (a) separate baseline tables (e.g. seats + bookings with no seat uniqueness) in `booking_db` under their own Postgres schema (e.g. `baseline`); (b) a separate `baseline_db` with its own role (one more database in the Compose init script); (c) another arrangement.
3. *Migrations:* (a) introduce `goose` now for the baseline tables only; (b) a plain SQL file applied by the seed/reset command; (c) created by the test harness only.
4. *Dataset:* one event and *N* seats, with *N* from AMB-43. Seat ids, sections, prices (integer paise) or none.

**✅ Decision (owner, 2026-11-06):** reserved seats only, no GA. Separate baseline tables inside `booking_db` (implemented as the Postgres schema `baseline`). The real schema and its constraints are never modified. Migrations use `goose`, with their own version table `baseline_goose_db_version`. Seed: one deterministic synthetic event with 500 seats. [ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)

<a id="amb-39"></a>
## AMB-39 — Naive baseline: request contract · M · ✅ Approved 2026-11-06

**Evidence.** The real hold endpoint is `POST /v1/events/{eventID}/holds` and needs an access token from the waiting room (p.25). Identity and JWT verification arrive in phase 4, the waiting room in phase 5. Invariant 7 requires an `Idempotency-Key` on state-changing endpoints. For T6 the k6 profile must be identical for the baseline and SurgeTix (p.34).
**Questions and options.**
1. *Route:* (a) the same path and body shape as `POST /v1/events/{eventID}/holds` with `{seat_ids}`, so the k6 profile carries over; (b) a separate path (e.g. under `/v1/baseline/…`).
2. *Semantics:* (a) a final booking (true check-then-book); (b) a hold with an expiry, as in SurgeTix.
3. *User identity:* (a) an unauthenticated user id supplied by k6 (e.g. a header); (b) no user at all; (c) wait for Identity (phase 4).
4. *Idempotency:* (a) no `Idempotency-Key`; (b) accepted and ignored.
5. *Responses:* `201` booked and `409` not available, or other codes. RFC 9457 error bodies (p.24) or not.

**✅ Decision (owner, 2026-11-06):** reuse the hold API's JSON request and response structure on a dedicated test-only route, separate from the real endpoint (implemented as `POST /v1/baseline/events/{eventID}/holds`). Identity comes from `X-Baseline-Buyer-ID`. No idempotency. Benchmark requests are unique, so retries cannot distort results. [ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)

<a id="amb-40"></a>
## AMB-40 — Naive baseline: Gateway → Booking transport before gRPC · M · ✅ Approved 2026-11-06

**Evidence.** Option B routes the request through the Gateway ([ADR-034](../../ARCHITECTURE_DECISIONS.md#adr-034)). Internal calls are gRPC + `buf` (p.13, p.27), but gRPC, `booking.v1` and the Gateway's routing belong to phase 4 (*"Services and gRPC"*, p.36), and ADR-022 proves the core before services are split.
**Options.** (a) Phase 2 Gateway is an HTTP reverse proxy to Booking's HTTP port; phase 4 replaces it with gRPC. (b) Bring gRPC, `buf` and a minimal `booking.v1` call forward to phase 2. (c) k6 calls Booking directly in phase 2, and the Gateway joins the path in phase 4.
**Note.** Whichever is chosen, the final T6 comparison must run both builds through the same topology ([ADR-034](../../ARCHITECTURE_DECISIONS.md#adr-034)).

**✅ Decision (owner, 2026-11-06):** option (a), internal HTTP between Gateway and Booking in phase 2. No early gRPC; gRPC arrives in phase 4 as planned. [ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)

<a id="amb-41"></a>
## AMB-41 — Naive baseline: isolating the unsafe path · H · ✅ Approved 2026-11-06

**Evidence.** The owner requires the unsafe path to be used only for testing and benchmarking ([ADR-034](../../ARCHITECTURE_DECISIONS.md#adr-034)). ADR-033 puts business code in `internal/<service>/`. ADR-021 forbids adding services. [AMB-34](#amb-34) left open where the baseline, k6 scripts and generated results live. CLAUDE.md P6 forbids committing generated data directories.
**Questions and options.**
1. *Guard:* (a) a Go build tag (e.g. `baseline`), so default builds do not contain the code; (b) a runtime setting (e.g. `BOOKING_MODE=naive`), off by default, logged loudly at start-up; (c) both; (d) a separate binary (e.g. `cmd/booking-baseline`), to be checked against ADR-021.
2. *Code location:* e.g. `internal/booking/naive`, or elsewhere.
3. *k6 scripts:* e.g. `loadtest/k6/`, or elsewhere.
4. *Results:* (a) commit small k6 summary JSON and audit output as evidence; (b) keep raw output out of git and transcribe results into the docs only.

**✅ Decision (owner, 2026-11-06):** both the build tag `unsafe_baseline` **and** the runtime flag `SURGETIX_ENABLE_UNSAFE_BASELINE=true` are required (default disabled). Normal builds expose no baseline endpoint. Baseline code and database operations stay isolated, baseline requests never reach the real booking tables, automated tests prove the guard, and the build and run steps are documented. No generated artifacts are committed. Locations used: `internal/booking/baseline`, `internal/gateway/baseline`, `loadtest/k6`, `loadtest/baseline`, and results in `loadtest/results/` (not committed). [ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)

<a id="amb-42"></a>
## AMB-42 — Phase 2 metrics: no Prometheus/Grafana exists yet · M · ✅ Approved 2026-11-06

**Evidence.** Phase 1 has no Prometheus, Grafana or `/metrics` endpoint ([`PROGRESS.md`](../../PROGRESS.md)). The PDF puts *"Grafana, tracing, audit"* in phase 8 (p.36) and names Prometheus and Grafana in the stack (p.15). The PDF's domain metrics include `holds_total{mode,result}`, `seats_remaining` and `double_booked_seats` (p.34). The Phase 2 plan needs request, outcome, inventory, oversell, latency and error signals. Services run on the host, so `docker stats` covers only the infrastructure containers.
**Options.** (a) Add `/metrics` with `prometheus/client_golang`, plus Prometheus and Grafana containers in Compose, now (a new Go dependency and two containers; needs an ADR). (b) `/metrics` only, no Prometheus or Grafana until phase 8. (c) No service metrics in phase 2: k6's own summary for throughput, latency and errors, the audit SQL for oversell, and OS or Go runtime counters for CPU and memory.
**Also open.** Whether baseline metrics reuse the PDF names with a label marking the build, or use separate names.

**✅ Decision (owner, 2026-11-06):** add a Prometheus-format `/metrics` endpoint and instrument the baseline booking and inventory metrics. k6 output and the SQL audit are the primary evidence. No Prometheus or Grafana infrastructure until phase 8. Naming (implementation): the PDF names with a constant label `build="unsafe_baseline"`. [ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)

<a id="amb-43"></a>
## AMB-43 — T6 scope, scale and environment in phase 2 · M · ✅ Approved 2026-11-06

**Evidence.** T6 is *"T1–T3 against the naive build with the same k6 profile"* (p.35). T2 needs the waiting room (phase 5) and T3 needs checkout and payment (phase 6), which do not exist yet. T1 is 5,000 concurrent buyers on 500 seats, run 100 times. The PDF expects k6 on 2–3 cloud VMs ([OQ-6](open-questions.md)). The development machine is an Intel i5-1235U (12 logical CPUs) with 7.7 GB RAM, on Windows 11 with Docker Desktop. k6 is not installed there.
**Questions and options.**
1. *Scope:* (a) Phase 2 runs the T1-shaped baseline only, and the T2/T3 baselines run when those phases exist; (b) another split.
2. *Scale:* (a) the full T1 scale locally, with the machine recorded; (b) scaled down, with the scale recorded; (c) cloud VMs (OQ-6).
3. *Repetitions:* 100 runs as T1 states, or fewer for the baseline.
4. *k6:* (a) the `grafana/k6` Docker image at a pinned version; (b) a local install.
5. *Matrix:* the arrival rates, inventory sizes and durations (the PDF gives none beyond T1).
6. *Co-location:* accept k6 on the same machine as the system under test and record it, or not.

**✅ Decision (owner, 2026-11-06):** the T1-shaped booking scenario only. Local matrix of 50, 100 and 250 concurrent buyers, plus 500 only if the machine stays stable, each run three times with a warm-up. The 5,000-buyer T1 scale is a future distributed run on 2–3 cloud load generators and is not forced onto the laptop. T6 runs when the waiting room and checkout exist. The baseline must be **unprotected but not sabotaged**: no sleeps, no reduced pools, no restricted CPU or memory, comparable settings, and the environment recorded. [ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)

<a id="amb-44"></a>
## AMB-44 — Phase 3 entry point, caller identity and k6 target · H · ✅ Approved 2026-11-06

**Evidence.** T1, T7 and T8 are k6 tests (p.35), so Phase 3 needs an HTTP way in. The public hold route sits behind the Gateway with an access token (p.25, p.28). gRPC, JWT/JWKS and Gateway routing are phase 4, and access tokens are phase 5 (p.36). `holds.user_id UUID NOT NULL` and the idempotency scope `(scope, user_id, key)` both need a user. The phase 2 baseline route is served on Booking's port and was benchmarked through the Gateway ([ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)).
**Questions and options.**
1. *Entry point:* (a) Booking serves `POST /v1/events/{eventID}/holds` and `DELETE /v1/holds/{holdID}` on its internal HTTP port, with phase 4 putting the Gateway (JWT → gRPC) in front; (b) a Gateway HTTP proxy as in phase 2; (c) bring gRPC forward.
2. *Identity:* (a) Booking trusts a user-id header on its internal port, phase 3 only; (b) the same, gated by a dev flag; (c) wait for JWT (phase 4).
3. *k6 target for the Phase 2 comparison:* (a) Booking directly for both builds (the baseline route also answers there); (b) through the Gateway for both.

**✅ Decision (owner, 2026-11-06):** **internal HTTP** on Booking's own port, with a **phase-3-only identity header**. **This shortcut must never be exposed publicly.** [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** It enables the k6 gate tests (T1/T7/T8) before gRPC/JWT (phase 4) and access tokens (phase 5) without pulling them forward.
**Guard (proposed, reviewed in M2):** the header is honoured only when `BOOKING_PHASE3_TRUSTED_IDENTITY=true` (default `false`) **and** Booking's listen address is a loopback address. Booking refuses to start if the flag is set with a non-loopback address. The header and flag are removed in phase 4, when identity comes from verified JWT claims.
**Still open:** the k6 target for the Phase 2 comparison. Proposed: Booking directly, for both builds.

**✅ Sub-question resolved (owner, 2026-11-06):** the k6 comparison targets **Booking directly for both** the baseline and the protected implementation. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)

<a id="amb-45"></a>
## AMB-45 — Inventory provisioning before Catalog; seat id format; GA bucket split · M · ✅ Approved 2026-11-06

**Evidence.** *"Catalog's `seat_layouts` … copied into Booking's `seats` when a sale is published"* (p.22), but Catalog arrives in phase 4. `seats.seat_id` is `TEXT`, with the only example `'A-12'` (p.17). `ga_buckets` has 32 buckets per section (p.17, p.29), but how a section's capacity is divided among them is not stated. T7 needs *"fragmented stock (no single bucket can fill a 4-ticket order)"* and a 1-bucket comparison.
**Questions and options.**
1. *Mechanism:* (a) an internal Booking operation (e.g. `LoadInventory`) that phase 4's publish step will call; (b) a dev/test seed command or migration fixture only.
2. *Seat id format:* (a) `'<section>-<row>-<number>'`; (b) the PDF's `'A-12'` style; (c) chosen by Catalog later.
3. *Bucket split:* (a) even split, remainder to the lowest buckets; (b) another rule. Test layouts (fragmented, 1 bucket) are seeding parameters.
4. *Seat ordinal* for a `seatbits` offset (see AMB-49).

**✅ Decision (owner, 2026-11-06):** **generated seats** (a deterministic generator inside Booking, used by tests and benchmarks) and **evenly distributed GA buckets**. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** Catalog's publish step, which copies `seat_layouts` (p.22), arrives in phase 4. A generator gives reproducible layouts now, including T7's fragmented and 1-bucket variants as parameters.
**Plan details (confirm in M1 review):**
- **Bucket split:** `capacity` is divided evenly over the 32 buckets, with the remainder going one each to the lowest-numbered buckets. The 1-bucket variant uses bucket 0 only.
- **Seat id format:** `'<section>-<row>-<number>'`, e.g. `'A-R01-01'`.
- **Comparison layout:** matches the Phase 2 baseline (section A, R01–R20 × 25).

**✅ Follow-up (owner, 2026-11-06, after T2):** **no database `CHECK`** on `ga_buckets.bucket`, so the PDF schema stays exact. The generator and `Load` enforce bucket numbers 0–31, with tests for valid and invalid values (implemented in M1-T3).

<a id="amb-46"></a>
## AMB-46 — Phase 3 gate scale: T1 and T7 on local hardware · H · ✅ Approved 2026-11-06 · ⚠️ depends on OQ-6

**Evidence.** T1: *"5,000 concurrent buyers on the same 500 seats, run 100 times"*. T7: *"10,000 buyers on a 2,000-ticket GA section"* (p.35). Both are phase 3 gate tests (p.36). The PDF expects k6 on 2–3 cloud VMs (p.10, OQ-6). For phase 2 the owner made 5,000 buyers a future distributed run ([ADR-035](../../ARCHITECTURE_DECISIONS.md#adr-035)). The phase 2 local runs showed pool wait dominating at 250 buyers and 0.72 GB free RAM afterwards.
**Questions and options.** (a) Run T1/T7 at full scale locally, accepting distortion. (b) Pass the gate on the deterministic suite plus scaled-down local k6 runs, with full scale on cloud VMs as a follow-up. (c) Run at full scale on cloud VMs before the gate (needs OQ-6). Also: is "run 100 times" required at the chosen scale?

**✅ Decision (owner, 2026-11-06):** **deterministic local tests**, plus **full-scale T1/T7 on cloud VMs**. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** Phase 2 measured this laptop at its limits by 250 buyers (pool wait dominated, 0.72 GB RAM free). Correctness is proven deterministically, and scale needs separate load generators (p.10).
**⚠️ Not a weakened gate.** The PDF gate stays: T1 (5,000 buyers × 100 runs) and T7 (10,000 buyers) must pass at full scale. Running on cloud VMs depends on [OQ-6](open-questions.md) (which VMs, what budget), which is still **open**. Phase 3 cannot be marked complete until OQ-6 is decided and those runs pass.

**Update (owner, 2026-11-06):** OQ-6 (cloud VMs and budget) must be resolved before the full-scale cloud validation (M5). **It does not block M1–M4.** [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)

<a id="amb-47"></a>
## AMB-47 — Outbox in phase 3: topic names, envelope, payload format · M · ✅ Approved 2026-11-06

**Evidence.** Every hold transaction ends with *"outbox events (HoldCreated on `booking.holds`, InventoryChanged per section)"* (p.28). `outbox.topic` and `outbox.payload BYTEA` are `NOT NULL` (p.21). Topic names are open ([AMB-03](#amb-03)), as are payload serialisation, the event envelope and trace context ([AMB-12](#amb-12)). The relay arrives in phase 4.
**Options.** (a) Decide AMB-03 now (the p.24 table names) and a versioned JSON envelope (event id, type, version, occurred_at, aggregate key); (b) protobuf payloads, pulling `buf` forward; (c) write no outbox rows until phase 4 (contradicts p.28 and needs retrofitting).

**✅ Decision (owner, 2026-11-06):** **versioned JSON** outbox payloads, with **stable event names** (the PDF's: `HoldCreated`, `InventoryChanged`, `HoldReleased`, `HoldExpired`, `InventoryReturned`) and **event ids** (`outbox.id`, a UUID, equal to the envelope's `event_id`). [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** It is readable in tests and audits, needs no `buf` before phase 4, and versioning lets phase 4 evolve the format without breaking consumers.
**⚠️ Dependency.** `outbox.topic` is `NOT NULL` (p.21), so the topic names must be decided. [AMB-03](#amb-03) is still open. The plan uses the p.24 table names (`booking.holds`, `booking.inventory`) **only if the owner approves AMB-03**. The outbox task in M1 is blocked until then.

**Update (owner, 2026-11-06):** the dependency is cleared. [AMB-03](#amb-03) is approved with `booking.holds` and `booking.inventory`. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)

**Update (owner, 2026-11-06, M1-T6):**
- **Scope.** T6 is the in-transaction insert helper only. The relay stays phase 4 (P4-5).
- **Aggregate for inventory events.** `{"type":"section","id":"<event_id>:<section>"}`, equal to the p.24 partition key. Hold events use `{"type":"hold","id":"<hold_id>"}`.
- **`occurred_at` format.** RFC 3339 in UTC with exactly six fractional digits (for example `2026-11-06T08:30:00.123456Z`), taken from the transaction's `now()`.
- **Open gap.** The aggregate's version that p.24 requires is not in envelope v1. It is logged as [AMB-54](#amb-54) and not resolved here.

<a id="amb-48"></a>
## AMB-48 — Idempotency mechanics · M · ✅ Approved 2026-11-06 (incl. additive schema)

**Evidence.** The claim semantics are specified (completed → stored response; in progress → 409; different body → 422, p.28). Not specified: whether the claim shares the hold's transaction (if it does, the `IN_PROGRESS` → 409 path never occurs); how an `IN_PROGRESS` row left by a crash is recovered (F8 says a retry with the same key is safe); whether non-201 outcomes (`409 unavailable`) are stored; how the request hash is computed; retention ([AMB-12](#amb-12)); and the semantics for `DELETE /v1/holds/{id}`.
**Options.** (1) Claim in its own transaction and complete inside the hold transaction, or claim and hold in one transaction. (2) Stale claims re-claimable after a lease (duration?), or blocked until `expires_at`. (3) Store `409` outcomes or not. (4) Hash the raw body or canonical JSON. (5) Retention period. (6) A repeated DELETE returns the stored `204`, or `404`.

**✅ Decision (owner, 2026-11-06):**
- **30-second stale-claim lease.**
- **Canonical JSON request hash.**
- **Deterministic `409` outcomes are stored** and replayed.
- **24-hour retention.**
- **Mandatory claim generation (fencing token):** an old claimant must never overwrite, complete or interfere with a newer claim.

[ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** The claim runs in its own short transaction so the PDF's `IN_PROGRESS → 409` path exists. Completion happens **inside** the business transaction as a fenced conditional update, so the stored response and the effect commit together (F8). A crash before commit therefore leaves only a claim that the lease releases. A crash after commit leaves a `COMPLETED` record that every retry replays.
**⚠️ Schema deviation (needs explicit confirmation).** The PDF's `idempotency_keys` has no generation or lease column (p.21). Fencing needs two **additive** columns, `claim_generation BIGINT NOT NULL DEFAULT 1` and `claimed_at TIMESTAMPTZ NOT NULL DEFAULT now()`. No existing column or constraint changes. This is recorded in ADR-036 as an approved-design / pending-schema item.

**✅ Schema confirmed (owner, 2026-11-06):** the two **additive** columns `claim_generation` and `claimed_at` may be added to `idempotency_keys` (migration 00004). Generation-based fencing ensures an old claimant can never overwrite, complete or delete a newer claimant's record. All PDF columns, the primary key and the `CHECK` are preserved. The deviation is documented in [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06).

<a id="amb-49"></a>
## AMB-49 — `seatbits` contents and maintenance vs inline reclaim · M · ✅ Approved 2026-11-06 (in Phase 3, milestone M4)

**Evidence.** `seatbits:{event}:{section}` *"rejects clearly taken seats before Postgres"* (p.23, p.28) and may only reject. The PDF does not say which states set a bit, who updates the bitmap, when (in or after the transaction), or how it is rebuilt. If HELD seats set bits, a hold that expired while the worker was stalled keeps its bits. The pre-filter then rejects the seat before inline reclaim can run, which defeats *"holds reclaim expired seats inline, so inventory is never stuck"* (p.33, F14).
**Options.** (a) Bits mark BOOKED only (sold seats); (b) bits mark HELD and BOOKED, with expiry time checked or the filter bypassed when the worker lags; (c) defer `seatbits` until measurements show it is needed. Also: update after commit vs from outbox events; rebuild at start-up.

**✅ Decision (owner, 2026-11-06):** **PostgreSQL is the only inventory truth**. **No cache or bitmap may ever prevent a valid inventory operation because of stale data.** `seatbits` is deferred in phase 3 *unless the PDF explicitly requires it*. Don't add complexity to the booking core. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** A HELD-marking bitmap can reject a seat that inline reclaim could take (breaking F14). The pre-filter is an optimisation, not a correctness mechanism, and F9 requires holds to work without it.
**⚠️ Conflict, owner confirmation needed.** The PDF does describe it explicitly: flow 2 **step 3** *"Pre-filter: seatbits in Redis rejects seats already shown as taken"* (p.28), plus the key table (p.23) and F9 (p.33). It is **not** in the phase 3 scope line (p.36) and no FR or test requires it. By the owner's own rule it may count as "explicitly required". Options:
- (a) formally defer it, as a deviation in ADR-036, to after Phase 3 measurements;
- (b) implement a **BOOKED-only** bitmap (never stale for a reclaimable hold) as a final, optional Phase 3 milestone.

Until decided, task P3-9 is ⛔.

**✅ Follow-up decision (owner, 2026-11-06), which resolves the conflict:** the PDF includes `seatbits` in the hold flow, so it **stays in the Phase 3 requirements and acceptance criteria**. Implementation is sequenced to the later milestone **M4**. When implemented:
- **booked-seats-only** bitmap semantics;
- PostgreSQL stays the authoritative inventory source;
- **stale bitmap data must never block a valid booking or reclamation.**

[ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036) (amendment 2026-11-06)
**Derived (plan §5.4):**
- *Safety invariant:* bits ⊆ seats committed as `BOOKED`. Set after commit; cleared before commit.
- *Phase 3 limitation:* nothing becomes `BOOKED` until `ConfirmHold` (phase 6), so in Phase 3 the pre-filter has no traffic effect. M4 proves the mechanism and its safety properties. Phases 6–7 must call the set and clear hooks.

<a id="amb-50"></a>
## AMB-50 — Inline reclaim of a partly reclaimed old hold · M · ✅ Approved 2026-11-06 · ⚠️ refines p.28 under contention

**Evidence.** *"A seat reclaimed from an expired hold also marks that old hold EXPIRED in the same transaction"* (p.28). If the old hold had other seats that the new request did not take, they stay `HELD` under an `EXPIRED` hold. The expiry worker only claims `ACTIVE`/`PAYMENT_PENDING` holds (p.30), so it never returns them. They are reclaimable only by a later chosen-seats request, and best-available never sees them.
**Options.** (a) In the same transaction, release the old hold's remaining seats as expiry would (and emit HoldExpired/InventoryReturned); (b) mark the old hold `EXPIRED` only when all its seats are reclaimed, leaving the rest to the worker; (c) leave as written.

**✅ Decision (owner, 2026-11-06):** when a new hold reclaims only some seats of an expired hold, release **all of the old hold's remaining seats in the same transaction**, but **only seats still owned by the old hold** (`hold_id = old AND status = 'HELD'`). Never release a seat another hold has acquired. No orphaned `HELD` seats, and inventory stays consistent. [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** Otherwise those seats stay `HELD` under an `EXPIRED` hold that the worker never scans (p.30).
**Mechanism ([phase 3 plan](phase-3-booking-core.md) §8, §10):** the old hold row and its other seats are taken with `SKIP LOCKED` (deadlock-free). **Invariant:** a hold becomes `EXPIRED` only in a transaction that leaves it with **zero** `HELD` seats and **zero** unreleased GA allocations.
**⚠️ Refinement of p.28.** The PDF says reclaim *"also marks that old hold EXPIRED in the same transaction"*. That holds unless one of the old hold's other seats is locked by a concurrent transaction at that moment. In that case the old hold stays `ACTIVE` (already past `expires_at`) and the expiry worker finishes it. A deterministic test proves it is finished.

<a id="amb-51"></a>
## AMB-51 — Migration execution; creating phase 6–7 tables in phase 3 · L · ✅ Approved 2026-11-06

**Evidence.** goose is the migration tool (p.14, ADR-035). P3-1 asks for the `booking_db` schema *"exactly as in postgres-schema"*, which includes `bookings`, `tickets` and `ticket_transfers` (used from phase 6). Kubernetes runs ≥ 2 replicas per service (p.8). The phase 2 baseline migrates at start-up with its own version table.
**Options.** (1) Run at Booking start-up with goose's Postgres session lock, or as a separate migration step/job. (2) Create the full schema in phase 3, or only the tables phase 3 uses.

**✅ Decision (owner, 2026-11-06):** **goose** migrations with a **session-level lock** (Postgres advisory lock). A test must verify that **the same database session** holds the lock for the whole migration run. The full `booking_db` schema is created in phase 3 (P3-1). [ADR-036](../../ARCHITECTURE_DECISIONS.md#adr-036)
**Why.** Kubernetes runs ≥ 2 Booking replicas (p.8). A session lock taken on one pooled connection but released or run on another would not exclude a second migrator.
**Plan details:**
- **When:** migrations run at Booking start-up, before the listener opens.
- **Lock connection:** the lock is taken on one dedicated `*sql.Conn`, and the M1 test confirms goose runs every migration on that session.
- **Version table:** goose's default, `goose_db_version`. The baseline's `baseline_goose_db_version` and `baseline` schema are untouched.


<a id="amb-52"></a>
## AMB-52 — `SoldOut` emission condition · M · 🟡

**Evidence.** The topic table lists **SoldOut** on `booking.inventory`, keyed by `event_id`, consumed by the waiting room (p.24). The queue state carries `sold_out`, which *"flips"* on sell-out (p.27–28). No flow says which transaction emits `SoldOut`, what "sold out" means, or what happens when inventory returns. The owner requires the condition to be defined before the relevant outbox flow is implemented (2026-11-06).
**Questions and options.**
1. *Meaning:* (a) no `AVAILABLE` seats and no GA `remaining` left in the whole event (everything held or booked); (b) everything `BOOKED` (only possible from phase 6); (c) per section rather than per event.
2. *Emitter:* the transaction that takes the last unit (hold, best-available, GA) emits it once, detected inside that transaction.
3. *Inventory returning* (expiry, release, cancellation): no event, so `sold_out` stays true and returned stock goes to second chance (p.31); or a reverse event, which the PDF does not name.
4. *Exactly once:* how to avoid duplicate `SoldOut` from concurrent last-unit transactions, e.g. a guarded flag row in `booking_db` (a schema addition needing an ADR) or consumer dedupe.

<a id="amb-53"></a>
## AMB-53 — GA ticket price source and hold-amount consistency · H · 🟡

**Evidence (PDF).**
- A GA hold returns `201 {hold_id, … allocation, expires_at, amount}` (p.29).
- Checkout reads the hold's amount through `booking.v1.GetHold` ("amount, status", p.26–27) and stores `orders.amount_paise` (p.19). FakePay charges the order's amount.
- Seat prices reach Booking as `seats.price_paise`, copied from Catalog's `seat_layouts` when a sale is published (p.22).
- **GA has no price anywhere in `booking_db`.** `ga_buckets` has no price column, and `holds` stores no amount (p.17–18). Catalog's columns are unspecified ([AMB-13](#amb-13)).

**Owner direction (2026-11-06):** do not add a price column to `ga_buckets`, and do not invent or hardcode a GA price. Keep T3 inventory generation independent of pricing. GA pricing is an **explicit dependency of GA holds (M3)**. Do not implement the integration now.

**How "hold amount = amount charged" already works for seats (derived):** Booking computes a seat hold's amount inside the hold transaction from its own `seats.price_paise`, which Catalog copied at publish. `GetHold` recomputes it from the same rows (`hold_seats` → `seats`). Checkout takes the amount **from `GetHold`**, never from Catalog, and charges `orders.amount_paise`. The amount is consistent as long as Booking's price rows do not change while a hold is live. The PDF does not say whether prices may change after publish.

**Options for GA (to decide before M3):**
- (a) **Copy at publish, like seats.** `PublishSale` gives Booking each GA section's price, stored in Booking's own table (a new `ga_sections(event_id, section, price_paise)` table, a schema deviation needing an ADR; not a `ga_buckets` column). The hold and `GetHold` compute `Σ allocation quantity × section price` from Booking's own data. *Leaning.* It keeps the hold path free of synchronous cross-service calls (p.12: one Postgres transaction decides) and mirrors the seat design.
- (b) **Snapshot the amount onto the hold.** An `amount_paise` column on `holds` (schema deviation), written in the hold transaction. `GetHold` returns the stored value. This also guarantees consistency if prices may change after publish. It can be combined with (a).
- (c) **Ask Catalog at hold time** over gRPC. This puts a synchronous dependency on the hot path and needs Catalog prices to be immutable during a hold. Not recommended.
- (d) **No amount for GA holds** until Catalog exists (phase 4). The hold response then deviates from p.29 temporarily.

**Also to decide:** whether a published price may ever change (if yes, (b) is required for seats too), and whether GA pricing is per section or per tier ([AMB-09](#amb-09)).

<a id="amb-54"></a>
## AMB-54 — Aggregate version on events · M · 🟡

**Evidence (PDF).**
- p.24: every event carries a unique `event_id` **and the aggregate's version**, so consumers can drop duplicates and **ignore stale versions**. ADR-009 repeats this.
- p.24: each topic is keyed by the entity whose event order matters. `booking.holds` is ordered per hold, and `booking.inventory` per section (`event_id:section`).
- p.21: `outbox` has no version or sequence column. `holds` and `seats` have no version column. No row represents a section.
- Envelope v1 ([AMB-47](#amb-47) ✅) has no version field. M1-T6 implements v1 as approved and leaves the version out.

**Implications (derived).**
- Consumers cannot tell a stale event from a newer one for the same aggregate. They can only deduplicate on `event_id`.
- The relay has nothing exact to order a key's events by. `created_at` and `occurred_at` are the transaction's **start** time. A transaction that starts earlier can wait on a lock and commit later than a newer one, so two events for one section can carry times in the opposite order to their commits. Events written in one transaction share the same time.
- **Phase 3 correctness is unaffected.** Postgres is the only inventory truth, and nothing publishes in Phase 3. The gap matters from the relay (P4-5) and the first consumer onward.

**Options (none chosen).**
- (a) **A version per aggregate.** Add a `version` column on `holds`, plus a section-level row for `booking.inventory`. Each is incremented under a lock the business transaction holds, and the envelope gains `aggregate.version`. This is a schema deviation that needs an ADR. A section row is a new lock in the §10 order, and it serialises every hold within a section.
- (b) **A sequence per key, assigned when the event is written.** A counter row per outbox key, locked and incremented in the transaction, and a `seq` column on `outbox`. It needs an ADR. Its lock-order and per-section contention effects are the same as (a).
- (c) **A global sequence** (e.g. `bigserial`) assigned at insert. Allocation order is not commit order, so the relay must handle gaps and late commits. It needs an ADR.
- (d) **No version.** Consumers deduplicate on `event_id` only and reconcile from state. This weakens p.24 and needs an ADR.
- **Also to decide:** whether adding the field changes `schema_version` (to 2) or is an additive change within v1.

**Blocks.** P4-5 (outbox relay) and any consumer that relies on per-key ordering or stale-version rejection. Linked from [phase 3 plan §9](phase-3-booking-core.md#9-transactional-outbox-pdf-p24-p28-adr-amb-47), [kafka-topics](../04-data/kafka-topics.md#event-envelope-rules-p24) and [TASKS](../../TASKS.md) P4-5.

---

## How to approve

For each item, reply with the ID and a decision (e.g. *"AMB-03: table is authoritative"*). Each decision will be:
1. marked ✅ here with the date;
2. recorded as an ADR where it is architectural;
3. propagated to every linking doc;
4. logged in [`/PROGRESS.md`](../../PROGRESS.md).
