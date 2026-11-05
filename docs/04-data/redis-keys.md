# Redis Keys

*Source: PDF p.22–24 (Redis keys table, hash-tag note), p.14 (Redis 7, go-redis, Lua), p.15 (Redis Sentinel on Kubernetes), p.27–28 (flow 1), p.33 (failures).*

Redis 7 via `go-redis`. Multi-key atomic logic is written as **Lua scripts** (p.14).

> **Redis is never the source of truth for inventory or money.** Postgres is. Every key below is either a queue structure, a cache, a fencing primitive or a limiter. Losing Redis may pause admissions and slow reads. **It must never corrupt seat or money state** (p.31).

## Key catalogue

| Key | Type | Owner | Purpose | TTL |
|---|---|---|---|---|
| `wr:{event}:presale` | Sorted set (random score) | Waiting Room | Pre-sale arrivals; shuffled into ticket numbers at sale start | — |
| `wr:{event}:seq` | Counter | Waiting Room | Next queue ticket number after sale start | — |
| `wr:{event}:admitted` | Counter (**public**) | Waiting Room | Highest admitted ticket number; **only increases**; cached at the edge for 1 s | — |
| `wr:{event}:user:{user}` | String | Waiting Room | One active queue entry per user (`SET NX`) | TTL (value unspecified) |
| `wr:{event}:epoch` | Counter | Waiting Room | Leadership epoch; incremented every time the admission lease is acquired (**fencing token**) | — |
| `wr:{event}:lease` | String `owner:epoch` | Waiting Room | Admission-controller lease; renewed only by the owner of the current epoch | **5 s** |
| `wr:{event}:second_chance` | Sorted set (ticket number) | Waiting Room | Users waiting after sell-out, offered returned inventory in order | — |
| `pow:{nonce}` | String | Waiting Room | Spent proof-of-work nonces (no replay) | **2 min** |
| `seatbits:{event}:{section}` | Bitmap | Booking | Fast pre-filter: rejects clearly taken seats before Postgres (**Postgres stays the truth**) | — |
| `seatmap:{event}:{section}` | String | Catalog | Cached availability snapshot for page loads | **2 s + jitter** |
| `rl:{scope}:{key}` | Hash | Gateway | Token-bucket rate-limit state | TTL (value unspecified) |

"—" means the PDF gives no TTL. TTLs marked "unspecified" are listed in [AMB-12](../10-planning/ambiguities.md#amb-12).

## Hash tag rule

*"The `{event}` part is a Redis Cluster hash tag: every `wr:` key for one event lands in the same slot, so the admission Lua script can check the lease and update the counter atomically. The cost is the single-shard hot spot noted under Capacity estimates."* (p.23–24)

**Rule:** any Lua script that touches more than one key must only touch keys with the **same** `{event}` tag. Note that the per-user key `wr:{event}:user:{user}` has its tag on `{event}`, **not** on the user. This is deliberate, so join logic can stay in the event's slot.

> The Tech stack specifies **Redis Sentinel** on Kubernetes (p.15), which is primary/replica failover and not Cluster sharding. Under Sentinel, hash tags have no effect: all keys live on one primary. Which topology v1 uses (and therefore whether the "single shard" is literally one Sentinel-managed primary) needs confirming: [AMB-20](../10-planning/ambiguities.md#amb-20).

## Lua scripts named by the PDF

| Script | Keys | Atomic steps | Source |
|---|---|---|---|
| **Acquire leadership** | `wr:{event}:epoch`, `wr:{event}:lease` | If the lease is free: `INCR epoch`; `SET lease = owner:epoch` (TTL 5 s) | p.28 |
| **Admission tick** | `wr:{event}:lease`, `wr:{event}:admitted` | (a) lease == own `owner:epoch`? (b) `admitted = max(current, new)` (c) renew the lease | p.28 |

**Derived:** the join path (`pow:{nonce}` burn + `SET NX wr:{event}:user:{user}` + `INCR seq` / `ZADD presale`) would need to be a single script to meet *"one Redis call per queue join"* (NFR-EFF; the diagram says *"SET NX, INCR (one per join)"*). However, `pow:{nonce}` does not carry the `{event}` tag, so it could land in a different Cluster slot. See [AMB-29](../10-planning/ambiguities.md#amb-29).

## Sizing

Queue of 1M entries ≈ **100 MB** (p.9). Everything fits in memory.

## Failure behaviour (p.33)

| Failure | Effect |
|---|---|
| Redis down | **Admissions pause (fail closed)**; admitted users continue with signed tokens; seat map falls back to throttled DB reads; holds skip the bitmap pre-filter |
| Hot event's shard saturated | Join/status latency rises; joins shed with `429` + `Retry-After`; the edge cache keeps status reads off Redis |

## Benchmark obligations (T2, p.10)

Step one event's join and poll rate up until Redis saturates. Record **Redis CPU, commands/s, memory, p99 latency and failover time**.
