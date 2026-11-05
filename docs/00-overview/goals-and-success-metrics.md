# Goals, Non-Goals and Success Metrics

*Source: PDF p.1–3.*

> **Definition of success (PDF p.1):** zero double-bookings under any load, a flat booking-path p99 during a spike, and a measured improvement over a naive baseline under identical load.

## Goals

| # | Goal | Primary mechanism | Verified by |
|---|---|---|---|
| G1 | Make double-selling a seat or GA ticket **impossible within the data model** (conditional updates, check and unique constraints), and verify it with repeated concurrency tests. | Conditional `UPDATE`, `CHECK`, partial unique index | T1, T7 |
| G2 | Keep the booking path responsive during a spike by **admitting users at a rate the system can serve, adjusted to live health**. | AIMD admission controller | T2, T3 |
| G3 | Give every buyer a **fair, transparent experience**: queue position, ETA, a clear sold-out signal, and a second chance when seats come back. | Cached queue state, second-chance queue | T2, T9 |
| G4 | **Never double-charge, never refund a confirmed booking by mistake, never leave a paid buyer without a ticket or a refund.** | Durable payment-before-confirm, idempotent `ConfirmHold`, refund only on definitive rejection | T5, T12, nightly audit |
| G5 | Support the **full ticket lifecycle**: purchase, cancellation with refund, transfer to a friend, and online entry scan. | Checkout + Booking sagas | T9, T11 |
| G6 | **Maximize efficiency**: the fewest DB operations, network calls and servers per ticket sold, measured and reported. | Round-trip budget, caching, PGO | Efficiency metrics |
| G7 | **Measure the impact of each layer** against a naive baseline under an identical load profile. | Naive baseline build + k6 | T6 |

## Non-goals

These are explicitly excluded by the PDF (p.2). Do not design for them in v1.

- **Real payments, real money or PCI compliance.** FakePay stands in, including for refunds.
- **Multi-region deployment.**
- **Dynamic pricing.**
- **A paid resale marketplace.** A *free* transfer to a friend **is** in scope (FR-16).
- **A polished consumer frontend.** A thin React app exists only to demo the flows.
- **Machine-learning bot detection.** Proof-of-work, limits and heuristics only.

Other v1 exclusions stated elsewhere in the PDF:

- **Offline gate scanning.** Gates need venue connectivity (p.4, p.6, p.31, p.34).
- **Partitioned per-event queues with globally coordinated admission.** These are deliberately left out of v1 and considered only if the measured single-shard ceiling is too low (p.10).

## Success metrics

The PDF calls these **targets**. Real values come from load tests (p.2–3).

| Metric | Definition | Target |
|---|---|---|
| Double-booked seats or oversold GA tickets | Audit query after every test run | **0 in every run** |
| Seat-hold API latency | `POST /holds` request → response, at the admitted rate | **p99 < 300 ms** |
| Checkout API latency | `POST /checkout` request → response (order + payment intent created; excludes the buyer paying) | **p99 < 300 ms** |
| Payment completion latency | Buyer submits payment on FakePay → signed webhook received (provider-controlled; healthy FakePay) | **p99 < 2 s** |
| Booking confirmation latency | Webhook received → order `CONFIRMED` with tickets issued | **p99 < 500 ms** |
| Refund completion latency | Cancellation accepted → refund `COMPLETED` | Healthy provider: **p99 < 60 s**. Provider down: tickets voided immediately, refund completes **within 15 min of provider recovery** |
| Queue status latency and cache share | `GET /queue/state` | **p99 < 50 ms; ≥ 95 % answered by cache** |
| Hold conflict rate | 409 responses ÷ hold attempts | **< 5 % with best-available**; baseline measured |
| Postgres round trips per seat sold | Counted from query metrics, hold through confirmation | **≤ 4** |
| Buyer-facing error rate | (5xx + timeouts) ÷ buyer requests, same k6 profile as the naive baseline (T6) | **≥ 10× lower than baseline** |
| Wrong refunds | Refunds issued for orders that end `CONFIRMED` | **0** (nightly audit) |
| Paid orders without a ticket or refund | Nightly audit after recovery | **0** |

Additional latency targets from the non-functional requirements (p.7):

| Metric | Target |
|---|---|
| Seat-map read | p99 < 100 ms |
| Online gate scan | p99 < 150 ms |
| Seat map / queue position staleness | ≤ 2 s |

### How metrics are measured

- Correctness metrics (double-booked, oversold, wrong refunds, paid-without-ticket) come from **audit queries**. They are exported as Prometheus gauges (`double_booked_seats`, `oversold_ga`) that must stay at 0. See [observability.md](../08-operations/observability.md).
- Latency and error rates come from **k6** runs and **RED metrics**.
- Round trips per seat sold come from **Postgres query metrics** across hold → checkout → confirmation.
- Provider-dependent steps have an **SLO for a healthy provider** and a **recovery objective for an unavailable provider** (p.7).

### Caveat on guarantees

The PDF is explicit (p.32): the correctness properties are **guarantees of the data model and code paths**. Tests (T1, T7, T12, T13) check them repeatedly, but *"tests are evidence, not a proof of every possible execution."*
