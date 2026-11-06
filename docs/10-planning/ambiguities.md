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
| [AMB-03](#amb-03) | Kafka topic names: diagram vs table | M | 4 | 🟡 |
| [AMB-04](#amb-04) | Order and refund state-machine gaps | **H** | 6–7 | 🟡 |
| [AMB-05](#amb-05) | `EVENT_CANCELLED` refunds have no flow or API | M | 7 | 🟡 |
| [AMB-06](#amb-06) | GA partial cancellation: which bucket is restored | **H** | 7 | 🟡 |
| [AMB-07](#amb-07) | Second-chance offer mechanics | **H** | 7 | 🟡 |
| [AMB-08](#amb-08) | Who serves SSE seat-map deltas | M | 8 | 🟡 |
| [AMB-09](#amb-09) | "Price tier" has no data model | M | 3 | 🟡 |
| [AMB-10](#amb-10) | Purchase limits: per hold, per user, per payment method | **H** | 3 / 5 | 🟡 |
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
| [AMB-26](#amb-26) | "Nearby alternatives" and the no-adjacent-run case | L | 3 | 🟡 |
| [AMB-27](#amb-27) | Hold transitions: release while `PAYMENT_PENDING` | M | 3 / 6 | 🟡 |
| [AMB-28](#amb-28) | What counts as a "Postgres round trip per seat sold" | L | 3 | 🟡 |
| [AMB-29](#amb-29) | "One Redis call per queue join" vs several operations | M | 5 | 🟡 |
| [AMB-30](#amb-30) | Ticket transition edge cases | L | 7 | 🟡 |
| [AMB-31](#amb-31) | FakePay API surface | M | 6 | 🟡 |
| [AMB-32](#amb-32) | Refund amount per ticket and rounding | M | 7 | 🟡 |
| [AMB-33](#amb-33) | RPC retry rule vs saga retries | L | 6 | 🟡 |
| [AMB-34](#amb-34) | Repository / code layout | M | 1 | ✅ 2026-11-06 (ADR-033) |
| [AMB-35](#amb-35) | Admin control room: Grafana vs React | L | 8 | 🟡 |
| [AMB-36](#amb-36) | OQ-3 vs Tech stack (`pgx + sqlc`) | L | 1 | 🟡 |
| [AMB-37](#amb-37) | "AuthPlatform" reference | L | — | 🟡 |
| [AMB-38](#amb-38) | Naive baseline: inventory model, tables, migrations, dataset | **H** | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-39](#amb-39) | Naive baseline: request contract (route, identity, idempotency, codes) | M | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-40](#amb-40) | Naive baseline: Gateway → Booking transport before gRPC | M | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-41](#amb-41) | Naive baseline: isolating the unsafe path; code, k6 and results locations | **H** | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-42](#amb-42) | Phase 2 metrics: no Prometheus/Grafana exists yet | M | 2 | ✅ 2026-11-06 (ADR-035) |
| [AMB-43](#amb-43) | T6 scope, scale and environment in phase 2 | M | 2 | ✅ 2026-11-06 (ADR-035) |

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
## AMB-03 — Kafka topic names: diagram vs table · M · 🟡

**Evidence.** The architecture diagram (p.10) labels Kafka *"booking.events and checkout.events"*. The topic table (p.24) defines `booking.holds`, `booking.inventory`, `booking.tickets` and `checkout.orders`, each with its own key and ordering.
**Leaning.** The table, which carries the keys and ordering rules. The docs follow it.
**Options.** (a) Table is authoritative; the diagram label is shorthand. (b) Two topics as in the diagram (this would lose per-entity keying).

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
## AMB-09 — "Price tier" has no data model · M · 🟡

**Evidence.** FR-2 lists *"price tiers"*, and best-available accepts *"section or tier"* (p.5, p.25). The `seats` table has only `price_paise` and no tier column (p.17).
**Options.** (a) Tier = distinct `price_paise` value. (b) Add a `tier` column copied from Catalog's layout.

<a id="amb-10"></a>
## AMB-10 — Purchase limits · H · 🟡

**Evidence.** FR-6: holds of *"1–4 seats"*. FR-2: organizer sets a *"per-user limit"*. FR-18: *"purchase caps per account and payment method"*. Capacity: *"max 4 seats per buyer"* (p.8).
**Unclear.** Is 4 a fixed system maximum or the default of the per-event limit? Is the cap per hold or cumulative per user across orders (and which service enforces cumulative caps across Booking and Checkout)? Does FakePay have a "payment method" identity to cap on? Does the limit apply to GA quantity too?

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
## AMB-26 — "Nearby alternatives" and the no-adjacent-run case · L · 🟡

**Evidence.** A `409` lists *"suggested alternatives"* / *"nearby alternatives"* (p.4, p.29), but the algorithm is unspecified. For best-available, the behaviour when the 40 `SKIP LOCKED` candidates contain no run of N adjacent seats (retry? non-adjacent? 409?) is also unspecified.

<a id="amb-27"></a>
## AMB-27 — Hold transitions: release while `PAYMENT_PENDING` · M · 🟡

**Evidence.** `RELEASED` exists (p.18), and *"failures release seats"* (p.34). Not stated: whether `DELETE /holds/{id}` is allowed once checkout has started; whether a failed payment explicitly releases the hold or lets it expire; and the exact release transaction (assumed to mirror expiry).

<a id="amb-28"></a>
## AMB-28 — What counts as a "Postgres round trip per seat sold" · L · 🟡

**Evidence.** Target ≤ 4, *"hold through confirmation"* (p.3). Unclear: per seat or per order (2.5 seats/order); whether `checkout_db` trips count; whether idempotency-claim and outbox statements count separately or as one transaction round trip.
**Added in final review.** Flow 3 (p.28–29) implies separate write transactions for the hold, `StartPayment`, the order insert, payment-saved, `ConfirmHold` and order → `CONFIRMED`. Counted naively that exceeds 4, so the counting rule decides whether the target is achievable ([ARCHITECTURE_REVIEW](../../ARCHITECTURE_REVIEW.md) I-9, R-2).

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
## AMB-36 — OQ-3 vs Tech stack (`pgx + sqlc`) · L · 🟡

**Evidence.** The Tech stack names *"pgx + sqlc; goose migrations"* (p.14) as chosen. Open Questions (p.37) still asks the same question, with the proposal *"sqlc for CRUD, hand-written SQL for the hold, GA and refund paths"*.
**Leaning.** The proposal is the decision (sqlc for CRUD and hand-written pgx SQL on hot paths); confirm to close OQ-3.

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

---

## How to approve

For each item, reply with the ID and a decision (e.g. *"AMB-03: table is authoritative"*). Each decision will be:
1. marked ✅ here with the date;
2. recorded as an ADR where it is architectural;
3. propagated to every linking doc;
4. logged in [`/PROGRESS.md`](../../PROGRESS.md).
