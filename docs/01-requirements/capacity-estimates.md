# Capacity Estimates

*Source: PDF p.8–10.*

> **Key insight (PDF p.8):** *The crowd is huge but the inventory is tiny, so the hard load is queue polling, not booking. The booking core only ever sees the admitted rate.*

## Reference sale (design target)

| Parameter | Value |
|---|---|
| Venue | One **50,000-seat** stadium |
| Buyers | **1,000,000** arriving within **5 minutes** |
| Max seats per buyer | **4** |
| Average seats per order | ~**2.5** → ≈ **20,000 orders** |
| Design/test target | **1M** buyers |
| Stretch goal | **10M** buyers. Each event's queue counters live on one Redis shard, so adding app replicas does not spread that load (see below) |

## Load figures

| Quantity | Estimate | What it drives |
|---|---|---|
| **Queue joins** | 1M ÷ 300 s ≈ **3,300/s** (33,000/s at 10M: stretch, single Redis shard, *measure before claiming*) | One Redis `INCR` / `ZADD` per join, after a proof-of-work check |
| **Queue status polls** | Adaptive interval (**5 s near the front, 30 s far back**) ≈ **40,000/s** from clients | Answered by one global counter **cached at the edge for 1 s**. Origin sees ~1 request per edge location per second |
| **Admitted users** | Adaptive, **start at 500/s** | Bounds everything below it |
| **Hold attempts** | 500/s × ~1.1 attempts with best-available ≈ **550/s**. Target capacity **1,000/s** | Postgres primary sizing, connection pool |
| **GA ticket holds** | Up to **1,000/s on one section** | Spread over **32 counter buckets** so no single row is hot |
| **Seat-map reads** | Every admitted user, several refreshes ≈ **2,000/s** | Redis snapshot per section. SSE deltas for live viewers |
| **Checkout + payment** | ≈ 20,000 orders over the sale, **peak ~200/s** | Saga throughput, FakePay limits |
| **Cancellations and refunds** | Assume **3 % of orders ≈ 600**, bursty after event changes | Refund saga, second-chance offers |
| **Gate scans** | 50,000 entries over 2 hours, **peak ~30/s** | Scan endpoint, check-in uniqueness |
| **Kafka events** | Holds, expiries, confirmations, refunds ≈ **2,000/s peak** | **1 broker locally, 3 on Kubernetes** |
| **Data size** | Seats 50K rows ≈ **10 MB**. Queue 1M entries ≈ **100 MB** in Redis | **Everything fits in memory** |

## The per-event Redis hot spot

The queue sequence, the admission counter and the other per-event keys share **one Redis Cluster hash slot** via the `{event}` hash tag, so Lua scripts can update them **atomically**. As a consequence, **one event's queue traffic is pinned to a single Redis shard**, no matter how many Waiting Room replicas run.

**v1 position (PDF p.10):** accept this and **measure it**.

- Benchmark one event's queue at increasing join and poll rates.
- Record **Redis CPU, commands/s, memory, p99 latency and failover time**.
- Only if the measured ceiling is too low, design **partitioned queues with a globally coordinated admission policy**. This is *a much harder problem deliberately left out of v1*.

When the shard saturates in production-like conditions, joins are **shed with `429` + `Retry-After`** and the edge cache keeps status reads off Redis (PDF p.33).

This is recorded as [ADR-011](../../ARCHITECTURE_DECISIONS.md#adr-011) in the decision log.

## Load-test target

What a single machine cannot generate, a k6 run spread across **2–3 cloud VMs** can (PDF p.10):

- **50,000 virtual users** polling the queue (T2).
- A separate **contention test: 5,000 concurrent buyers on the same 500 seats** (T1).

The VM choice and budget are open ([OQ-6](../10-planning/open-questions.md)).

## Derived sizing notes

*These follow arithmetically from the figures above. They are not additional requirements.*

- **Poll-to-origin reduction.** With a 1 s edge TTL, origin load for `queue/state` is bounded by the number of edge locations (≈ N req/s), independent of crowd size. This is what makes the ≥ 95 % cache-share target reachable.
- **Admitted rate vs. hold capacity.** Start rate 500/s × 1.1 attempts = 550 holds/s, which leaves ~1.8× headroom to the 1,000 holds/s target before AIMD should back off.
- **Round-trip budget.** At ≤ 4 Postgres round trips per seat sold and ~50,000 seats, a full sell-out costs ≤ ~200,000 round trips on the booking/checkout path (excluding reads and expiry). See [AMB-28](../10-planning/ambiguities.md#amb-28) on which databases count.
