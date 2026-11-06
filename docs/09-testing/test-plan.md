# Test Plan

*Source: PDF p.35–36 (Test scenarios T1–T13, deliverable), p.34 (baseline comparison), p.10 (load-test target), p.15 (testing, load testing, fault-injection tooling), p.32 (tests are evidence, not proof), p.36 (phase gates).*

## Strategy

1. **Correctness under concurrency first.** Prove the booking core before splitting services (phase 3 gate: T1, T7, T8).
2. **Real dependencies.** `go test -race` plus **`testcontainers-go`** with real Postgres, Redis and Kafka. *Mocks miss SQL and lock behaviour* (p.15). Table-driven and **fuzz** tests in every package.
3. **One comparison for every claim.** The same **k6** scenarios (arrival-rate executors) run against the **naive baseline** and against SurgeTix, and the Grafana *Surge Control Room* records both (p.34).
4. **Audit after every run.** `double_booked_seats` and `oversold_ga` must be **0** (success metrics).
5. **Precise faults.** Toxiproxy, `kubectl delete pod`, `SIGSTOP`.

## Scenarios (verbatim from p.35–36)

| ID | Scenario | Pass criteria | Tooling | Phase gate |
|---|---|---|---|---|
| **T1** | **Contention:** 5,000 concurrent buyers on the same 500 seats, run 100 times | 0 double-booked seats; holds granted ≤ 500 | k6 | 3 Booking core |
| **T2** | **Spike:** 50,000 virtual users join in 60 s and poll the queue; also step one event's join and poll rate up until Redis saturates | Status p99 < 50 ms; ≥ 95 % of polls served from cache; Redis ceiling, CPU, memory and failover time recorded | k6 across 2–3 VMs | 5 Waiting room |
| **T3** | **Full sale:** T2 plus admitted users hold, pay and confirm until sold out | Per-step latency targets met; > 99 % of admitted users complete or see sold-out | k6 | 6 Checkout saga |
| **T4** | **Chaos:** kill a booking pod, a checkout pod mid-saga and the Kafka broker | No lost bookings or refunds; sagas resume; recovery without manual steps | `kubectl delete pod` | 9 Ship and measure |
| **T5** | **Slow provider:** FakePay adds 3 s latency and 20 % failures, then goes down for 5 minutes | Breaker opens; seats released on failure; no double charges; refunds complete within 15 min of recovery | Toxiproxy / FakePay faults | 6 Checkout saga |
| **T6** | **Baseline:** T1–T3 against the naive build with the same k6 profile | Recorded for comparison (errors, p99, oversells) | k6 | 2 Naive baseline ("k6 baseline runs") |
| **T7** | **GA contention:** 10,000 buyers on a 2,000-ticket GA section, including fragmented stock (no single bucket can fill a 4-ticket order) | 0 oversold; multi-bucket orders succeed; released allocations restore exact quantities; lock wait with 32 buckets vs 1 measured | k6 | 3 Booking core |
| **T8** | **Best-available vs manual picking** under the same load | Conflict rate and holds per second compared and reported | k6 | 3 Booking core |
| **T9** | **Refund storm:** 10 % of orders cancel within 2 minutes while sales continue | Every refund completes exactly once; returned seats reach second-chance users within 5 s | k6 | 7 Lifecycle |
| **T10** | **Bot run:** 1,000 scripted joins per second from few IPs | Join rate capped by proof-of-work and limits; human join latency unaffected | k6 | 5 Waiting room |
| **T11** | **Gate:** replay a 2-minute-old QR; scan the same ticket twice concurrently | Old code rejected; exactly one check-in succeeds | k6 / integration | 7 Lifecycle |
| **T12** | **Lost confirmation:** Booking commits `ConfirmHold`, then the response is dropped (fault-injection proxy) | Order ends `CONFIRMED`; 0 refunds issued; reconciler or retry resolves within 2 min | Toxiproxy | 6 Checkout saga |
| **T13** | **Stale leader:** pause the admission leader (`SIGSTOP`) longer than its lease, then resume | New leader takes over; 0 counter writes from the old epoch | `SIGSTOP` | 5 Waiting room |

## Metric targets checked across runs

From [goals-and-success-metrics](../00-overview/goals-and-success-metrics.md#success-metrics): hold p99 < 300 ms · checkout p99 < 300 ms · payment completion p99 < 2 s · confirmation p99 < 500 ms · refund p99 < 60 s / ≤ 15 min after recovery · queue status p99 < 50 ms, ≥ 95 % cached · conflict rate < 5 % · ≤ 4 Postgres round trips per seat sold · error rate ≥ 10× lower than baseline · 0 wrong refunds · 0 paid-without-ticket-or-refund · seat-map p99 < 100 ms · scan p99 < 150 ms.

## The naive baseline

- Built in **phase 2** as **"check-then-book"**; its gate is **"oversell seen"** (p.36). The baseline must visibly fail.
- **Shape decided (2026-11-06):** the same seven components **without protections** ([ADR-034](../../ARCHITECTURE_DECISIONS.md#adr-034), resolves [OQ-7](../10-planning/open-questions.md)).
- T6 runs T1–T3 against it with the identical k6 profile. Which parts of T6 run in phase 2, and at what scale, is open ([AMB-43](../10-planning/ambiguities.md#amb-43)).
- **Method** ([phase 2 plan](../10-planning/phase-2-naive-baseline.md)): the oversell is first proven by **deterministic** integration tests on real Postgres. A barrier between CHECK and BOOK forces every buyer past the check, so the counts are exact. k6 runs then measure it under load, and the baseline audit runs after every run. No results exist yet.

## Deliverable (p.36)

*A short report in the README with side-by-side charts (error rate, p99, oversold seats, DB queries per second) and the exact setup (machines, replica counts, versions).* This is the phase 9 gate ("README report").

## Load-test scale (p.10)

50,000 VUs polling the queue (T2), plus a separate contention test of **5,000 concurrent buyers on the same 500 seats** (T1). k6 is spread across **2–3 cloud VMs** ([OQ-6](../10-planning/open-questions.md)).

## Coverage gaps

*Derived. No numbered scenario covers these; they need integration/e2e tests or approval to leave them untested.*

| Area | Gap |
|---|---|
| Redis down (F9) | Fail-closed admissions and the throttled seat-map fallback have no scenario |
| CDN/edge down (F11) | In-process cache fallback untested |
| Postgres failover (F13) | Synchronous standby promotion untested |
| Expiry worker stopped (F14) | Inline reclaim exercised implicitly by T1 only if holds expire during the run |
| FR-1, FR-2, FR-16, FR-19, FR-20, FR-21, FR-23 | No numbered scenario |
| Nightly audit itself | Correctness of the audit queries |

Listed for approval in [AMB-02](../10-planning/ambiguities.md#amb-02).
