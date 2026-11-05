# Failure Scenarios

*Source: PDF p.32–34 (failure scenarios table, transcribed in full). Test references from p.35–36.*

Each row is **Failure → Effect → Handling** as specified. The "Test" column links to the scenario that exercises it, where one exists.

## Client and application failures

| # | Failure | Effect | Handling | Test |
|---|---|---|---|---|
| F1 | Client retries a hold, checkout, cancel or transfer | Possible duplicate | Idempotency record returns the stored response; different body with the same key → `422` | T1, T12 |
| F2 | `ConfirmHold` committed but the response was lost (timeout) | Checkout doesn't know the outcome | Step marked `UNKNOWN`; the retry is idempotent and returns the existing booking; reconciler checks by `order_id`. **No refund on a timeout** | **T12** |
| F3 | Payment succeeds after the hold expired and the seats moved on | Paid, no seats | `ConfirmHold` returns REJECTED → refund saga with reason `CONFIRM_REJECTED` | T3, T5 |
| F4 | Payment or refund webhook lost or duplicated | Order or refund stuck, or processed twice | Reconciler polls FakePay for anything pending > 2 min; `provider_ref` unique dedupes | T5 |
| F5 | Refund provider unavailable | Buyer waits for money | Tickets already void; saga retries with backoff; completes within 15 min of recovery; alert after 1 hour | **T5**, T9 |
| F6 | Checkout worker crashes mid-saga | Saga half done | Steps are durable; another replica resumes from the last completed step | **T4** |
| F7 | Admission controller pauses past its lease | Two controllers believe they lead | Epoch fencing: the stale controller's next write fails the Lua check and it steps down | **T13** |
| F8 | Booking service crashes mid-request | Unknown outcome for the client | The transaction either committed or not; a retry with the same key is safe | **T4** |

## Infrastructure failures

| # | Failure | Effect | Handling | Test |
|---|---|---|---|---|
| F9 | **Redis down** | Queue counter and caches unavailable | **Admissions pause (fail closed)**; admitted users continue with signed tokens; seat map falls back to throttled DB reads; holds skip the bitmap pre-filter | — |
| F10 | Redis shard for a hot event saturated | Join and status latency rise | Measured ceiling documented; joins shed with `429` + `Retry-After`; edge cache keeps status reads off Redis | **T2** |
| F11 | CDN or edge cache down | Status polls hit origin | Gateway serves `queue/state` from an in-process 1 s cache; clients back off on `429` | — |
| F12 | **Kafka down** | Events not published | Outbox rows stay `PENDING` with retry backoff; the relay publishes on recovery; second-chance offers delay but are never lost | **T4** |
| F13 | **Postgres primary fails** | Holds, checkouts and refunds fail briefly | Synchronous standby promoted; admission rate drops to zero until healthy | — |
| F14 | Expiry worker stops | Expired holds linger | Holds reclaim expired seats inline, so inventory is never stuck | — |

## External and adversarial failures

| # | Failure | Effect | Handling | Test |
|---|---|---|---|---|
| F15 | FakePay slow or failing | Checkout latency rises | Timeout + circuit breaker; hold extended for the payment window; failures release seats | **T5** |
| F16 | Bot floods queue joins | Queue polluted | Proof-of-work, one entry per verified account, per-IP limits | **T10** |
| F17 | QR code shared live or replayed | Someone else tries to enter | Code expires within about a minute; one-time check-in lets only the first scan in | **T11** |
| F18 | Gate loses connectivity | Scans fail | **Out of scope for v1**: gates require connectivity; documented as a known limitation | — |
| F19 | Nightly audit finds a mismatch | Possible bug | Page the owner; the report lists the exact orders, seats and payments involved | audit |

## Fault-injection tooling (p.15)

| Tool | Used for |
|---|---|
| **Toxiproxy** | Precise network faults such as *"commit, then drop the response"* (T12), FakePay latency/failure (T5) |
| **`kubectl delete pod`** | Kill booking / checkout pods and the Kafka broker (T4) |
| **`SIGSTOP`** | Pause the admission leader past its lease (T13) |

## Gaps

- F9: the throttle for the DB fallback, and *which* DB serves seat maps (Catalog holds no inventory), are unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)).
- F13: the "synchronous standby" exists on Kubernetes via **CloudNativePG**. Under Docker Compose there is no standby described ([AMB-20](../10-planning/ambiguities.md#amb-20)).
- F15: circuit-breaker thresholds are unspecified ([AMB-12](../10-planning/ambiguities.md#amb-12)).
- F19: on-call / paging mechanism is unspecified ([AMB-17](../10-planning/ambiguities.md#amb-17)).
- No scenario has a test for F9, F11, F13 and F14. See [test-plan](../09-testing/test-plan.md#coverage-gaps).
