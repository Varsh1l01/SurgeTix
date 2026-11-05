# Technology Choices (Tech Stack)

*Source: PDF p.13–15 (Tech stack table), p.7 (efficiency NFR), p.34 (observability), p.37 (open questions).*

> *Every tool is free, runs on a laptop, and was picked for one job; each row names the realistic alternative and why it lost.* (PDF p.13)

Each major choice is recorded as an ADR in [`/ARCHITECTURE_DECISIONS.md`](../../ARCHITECTURE_DECISIONS.md).

## Tech stack

| Layer | Choice | Where it's used | Why | Why not the alternatives | ADR |
|---|---|---|---|---|---|
| **Language** | **Go 1.27** | All services, workers, FakePay | Cheap goroutine per request, small memory and images, fast startup, strong standard library (`net/http`, `crypto`, `context`) | **Java/Spring:** heavier, slower startup. **Node:** one thread for CPU work such as JWT signing and proof-of-work checks. **Rust:** slow to build with while learning | ADR-002 |
| **Frontend** | **React + TypeScript (Vite)** | Buyer app, admin control room, gate scanner page | Already known; it only needs to demo the flows | **Next.js:** server rendering not needed. **Native mobile app:** out of scope | — |
| **Edge cache** | **NGINX `proxy_cache`, 1 s microcache** (a CDN such as Cloudflare if hosted) | `GET /queue/state`, static assets | One config block absorbs nearly every queue poll | **Varnish:** equivalent, one more tool. **Per-pod cache in Go only:** origin load grows with pod count | ADR-027 |
| **Gateway** | **Go `net/http`** (pattern routing) | Single public entry: JWT and access-token checks, rate limits, REST to gRPC | Full control, and it is the logic interviewers ask about | **Kong, Envoy, Traefik:** production-grade but hide the interesting code; name them as the production swap | ADR-028 |
| **Service calls** | **gRPC + Protobuf** (**`buf`** for lint and breaking-change checks) | Gateway to services, Checkout to Booking | Typed contracts, deadlines that propagate, HTTP/2 multiplexing | **REST/JSON:** no contracts, no deadline propagation. **GraphQL:** built for clients, not service calls. **Connect-RPC:** a fine alternative, less recognised | ADR-006 |
| **Source of truth** | **PostgreSQL 17**, one database per service; **`pgx` + `sqlc`**; **`goose`** migrations | Seats, GA buckets, holds, bookings, tickets, orders, payments, refunds, sagas, idempotency, outbox | Row locks, conditional updates, `CHECK` and partial unique constraints, `SKIP LOCKED` | **MySQL:** no partial indexes. **MongoDB:** document transactions are heavier for multi-seat updates. **Cassandra:** no multi-row transactions. **CockroachDB/Spanner:** distributed SQL adds latency the 50,000-seat inventory does not need. **GORM:** hides the SQL you must control for locking | ADR-003, ADR-024 |
| **In-memory state** | **Redis 7** (**`go-redis`**, Lua scripts) | Queue tickets and counters, admission lease and epoch, proof-of-work nonces, seat bitmaps, seat-map snapshots, rate limits | Sub-millisecond atomic operations, TTLs, sorted sets, Lua for check-and-set | **Memcached:** no scripts or sorted sets. **Postgres for every poll:** tens of thousands of queries per second on the database. **etcd for the lease:** correct but a whole cluster for one key. **Valkey:** a drop-in open-source fork; either works | ADR-004 |
| **Event log** | **Apache Kafka (KRaft mode) + `franz-go`** | Outbox relays publish; Notifier, Catalog and Waiting Room consume | Durable, replayable, many consumer groups, per-key ordering | **RabbitMQ:** strong for job queues, no replay. **NATS JetStream:** lighter and reasonable, less recognised. **Redis Streams:** would add load to the Redis already busy with the queue. **SQS/SNS:** cloud-only | ADR-005 |
| **Payments** | **FakePay** (own Go simulator: intents, signed webhooks, refunds, idempotency keys) | Checkout | Latency, failures and lost webhooks on demand, at load | **Stripe or Razorpay test mode:** realistic but rate-limited and cannot fail on command; can be swapped in behind the same interface later | ADR-023 |
| **Auth** | Identity service: **`bcrypt`**, **`golang-jwt`** (RS256), **`x/oauth2`** (Google, PKCE), JWKS | Sign-up, sign-in, verification at the gateway | Shows the security depth from *AuthPlatform* without a separate project ([AMB-37](../10-planning/ambiguities.md#amb-37)) | **Keycloak:** heavy to run locally. **Auth0/Firebase:** hosted, hides the work | — |
| **Live updates** | **Server-Sent Events** | Seat-map deltas, second-chance offers | One-way push over plain HTTP with built-in reconnect | **WebSockets:** two-way not needed, more connection state. **Polling:** wasteful at this scale | ADR-007 |
| **Email** | **Mailpit** (local SMTP inbox) | Ticket and refund emails | Free, inspectable during demos | **SendGrid/SES:** real delivery not needed; swappable | — |
| **Observability** | **Prometheus + Grafana**; **OpenTelemetry SDK, Collector and Tempo**; **`slog` JSON logs** (**Loki optional**) | Every service; the Surge Control Room dashboard | Open-source standard, runs locally, one trace across services | **ELK:** heavy on memory. **Datadog/New Relic:** paid. **Jaeger:** fine instead of Tempo | — |
| **Load testing** | **k6** (arrival-rate executors), spread across VMs for big runs | Tests T1 to T13 | JavaScript scripts, efficient Go engine, fixed request rates | **JMeter:** heavy and GUI-oriented. **Locust:** lower throughput per machine. **Gatling:** JVM tooling | — |
| **Fault injection** | **Toxiproxy**, **`kubectl delete pod`**, **`SIGSTOP`** | T4, T5, T12, T13 | Precise faults such as "commit, then drop the response" | **Chaos Mesh:** powerful, heavy for v1 | — |
| **Testing** | **`go test -race`**, **`testcontainers-go`**, table-driven and fuzz tests | Every package | Real Postgres, Redis and Kafka catch locking bugs that mocks cannot | **Mocks only:** miss SQL and lock behaviour | ADR-029 |
| **Run and deploy** | **Docker Compose** locally; **Kubernetes (kind or k3d)** with **CloudNativePG**, **Strimzi** and **Redis Sentinel**; **GitHub Actions** CI | Local development, high-availability demo, CI | Standard, free, transfers to any company | **Nomad/ECS:** less common. **Managed cloud databases:** cost | ADR-030 |

## Pinned versions and libraries (quick reference)

| Component | Version / library named in the PDF |
|---|---|
| Go | 1.27 |
| PostgreSQL | 17 |
| Redis | 7 |
| Kafka | KRaft mode (no ZooKeeper); version not pinned |
| Postgres driver / codegen / migrations | `pgx`, `sqlc`, `goose` |
| Redis client | `go-redis` |
| Kafka client | `franz-go` |
| JWT | `golang-jwt` (RS256) |
| OAuth | `golang.org/x/oauth2` |
| Protobuf tooling | `buf` |
| Frontend | React + TypeScript, Vite |

Patch versions and the exact library versions are not pinned in the PDF. They are fixed at implementation time and recorded in `go.mod` / lockfiles ([AMB-12](../10-planning/ambiguities.md#amb-12)).

## Also required by other sections of the PDF

| Concern | Requirement | Source |
|---|---|---|
| Build optimisation | **PGO** for all services; allocation-checked hot paths | p.7 |
| Profiling | **pprof**: CPU-seconds and allocations per booking | p.34 |
| Error format | **RFC 9457** problem details | p.24 |
| Cluster features | Probes, HPA, PodDisruptionBudgets, graceful shutdown | p.8 |
| Kafka durability | `acks=all`; 1 broker locally, 3 on Kubernetes | p.8–9 |

## Remaining undecided items

| Question | Status | Reference |
|---|---|---|
| Postgres access: the Tech stack lists `pgx` + `sqlc` as chosen, but Open Questions still asks "pgx with hand-written SQL, or sqlc?" with the proposal "sqlc for CRUD, hand-written SQL for the hold, GA and refund paths" | Needs confirmation that OQ-3 is closed on the proposal | [AMB-36](../10-planning/ambiguities.md#amb-36), [OQ-3](../10-planning/open-questions.md) |
| Redis **Sentinel** (Tech stack) vs. Redis **Cluster** hash-tag wording (Redis keys section) | Needs confirmation | [AMB-20](../10-planning/ambiguities.md#amb-20) |
| kind **or** k3d | Either allowed | [AMB-20](../10-planning/ambiguities.md#amb-20) |
| Hosting: local + demo video only, or a small cloud VM (which would bring in a CDN such as Cloudflare) | Open | [OQ-5](../10-planning/open-questions.md) |

## Usage rules that follow from these choices

These restate PDF rules so implementers can apply them per technology.

**PostgreSQL**
- Every inventory, order, ticket and refund state change is a **conditional `UPDATE … WHERE status = $from`**. Check `rows affected` (p.22, p.31).
- Use `FOR UPDATE SKIP LOCKED` to **choose candidates** (best-available, GA buckets, expiry batches, outbox claim). The conditional update still **decides** (p.31).
- `seats` uses `fillfactor = 80` for HOT updates (p.17).
- **Parameterized SQL only** (p.8). No ORM: GORM was rejected because it hides the SQL needed for locking (p.14).

**Redis**
- All `wr:{event}:*` keys share the `{event}` hash tag. Multi-key atomic logic goes in **Lua scripts** (p.23–24).
- Redis is **never** the source of truth for inventory. `seatbits` may only *reject* early (p.23, p.28).
- If Redis is unavailable, **admissions fail closed** (p.33).

**Kafka**
- Producers use **`acks=all`** (p.8).
- Events are written to the **outbox** in the business transaction and published by a relay, never directly (p.8, p.21).
- Each topic is keyed by the entity whose order matters. Consumers dedupe via `processed_events` and ignore stale aggregate versions (p.23–24).

**gRPC**
- Propagate the incoming deadline. Retry only on `UNAVAILABLE` with jittered backoff (p.27).
- Contracts are linted and checked for breaking changes with `buf` (p.13).

**Testing**
- Integration tests run against **real** Postgres, Redis and Kafka via `testcontainers-go`. Mocks alone are rejected for locking-sensitive code (p.15).
- Race detector (`-race`) is on for all Go tests (p.15).
