# Deployment and Operability

*Source: PDF p.8 (operability, availability, durability NFRs), p.9 (Kafka brokers), p.10 (load-test VMs), p.13–15 (tech stack: edge cache, run and deploy), p.33 (Postgres failover), p.36 (phases 1 and 9), p.37 (OQ-5 hosting).*

## Environments

| Environment | Purpose | Composition (from the PDF) |
|---|---|---|
| **Local (Docker Compose)** | Development; **one-command local run** | All services + FakePay; **PostgreSQL 17** (separate databases on one instance); **Redis 7**; **Kafka (KRaft), 1 broker**; **NGINX** edge cache (1 s `proxy_cache`); **Mailpit**; Prometheus, Grafana, OTel Collector, Tempo (Loki optional) |
| **Kubernetes (kind or k3d)** | High-availability demo, chaos tests | **CloudNativePG** (Postgres with a synchronous standby), **Strimzi** (Kafka, **3 brokers**), **Redis Sentinel**; ≥ 2 replicas per stateless service |
| **CI (GitHub Actions)** | Build, test, lint | `go test -race`, `testcontainers-go` integration tests, `buf` lint/breaking checks. The exact pipeline is unspecified |
| **Load generators** | Big k6 runs | **2–3 cloud VMs** for the 50,000-VU run. Which VMs and what budget: [OQ-6](../10-planning/open-questions.md) |
| **Hosted (optional)** | Application season | Open: demo video and local only, or a small cloud VM ([OQ-5](../10-planning/open-questions.md)). If hosted, a CDN such as **Cloudflare** replaces NGINX at the edge |

## Kubernetes requirements (NFR-OPS, NFR-AVL)

| Requirement | Notes |
|---|---|
| **≥ 2 replicas per stateless service** | No single point of failure in the request path |
| **Liveness/readiness probes** | Readiness should reflect dependency health (derived). Booking (phase 3 M1-T12): migrations run at start-up before the HTTP server listens, and `/readyz` also checks that the schema is at the binary's embedded migration version. Its runtime pool sets `idle_in_transaction_session_timeout` = 5 s (abandoned transactions release their locks); migrations and the plain pool do not |
| **HPA** | Scale signals per service from p.15–16: request rate (Gateway), queue joins (Waiting Room), admitted rate (Booking), orders (Checkout), seat-map reads (Catalog), Kafka lag (Notifier). Metric wiring is unspecified |
| **PodDisruptionBudgets** | Keep ≥ 1 replica during voluntary disruptions |
| **Graceful shutdown** | Drain in-flight requests; release leases; stop claiming outbox/expiry/saga batches |
| **Postgres HA** | Synchronous standby promoted on primary failure; admission rate drops to zero until healthy |
| **Kafka durability** | `acks=all`; 3 brokers |

## Build requirements

- **Go 1.27**, all services built with **PGO** (profiles from representative load), hot paths allocation-checked (p.7, p.13).
- Small container images (one of the reasons for choosing Go).

## Operational controls

| Control | Scope | Source |
|---|---|---|
| **Pause sale** | `POST …/sale:pause` | p.26 |
| **Resume sale** | `POST …/sale:resume` | p.26 |
| **Kill switch** | Per sale | p.6 (FR-20), p.8 |
| **Admission rate** | Automatic (AIMD); drops to 0 on DB failure; pauses on Redis failure | p.28, p.33 |

What each control stops or allows is not defined ([AMB-15](../10-planning/ambiguities.md#amb-15)).

## Background processes that must run (and be replica-safe)

| Process | Owner | Cadence | Concurrency safety |
|---|---|---|---|
| Admission controller | Waiting Room | per tick | Lease + epoch fencing |
| Hold expiry worker | Booking | every 2 s, batches of 1,000 | `FOR UPDATE SKIP LOCKED` |
| Outbox relay | every writing service | continuous | `locked_by` / `locked_until` |
| Saga executor | Checkout | `next_attempt_at` | `sagas.version` optimistic concurrency |
| Confirmation reconciler | Checkout | orders > 1 min in `PAID_PENDING_CONFIRMATION` | Idempotent `ConfirmHold` / `GetBookingByOrder` |
| Payment/refund reconciler | Checkout | pending > 2 min, polls FakePay | `provider_ref` unique |
| Nightly audit | Checkout | nightly | Read-only + report |
| Kafka consumers | Catalog, Waiting Room, Notifier | continuous | `processed_events` + version check |

## Secrets

JWT signing keys (Identity), the FakePay webhook HMAC secret, and the key for `qr_secret_enc`. Storage and rotation are unspecified ([AMB-19](../10-planning/ambiguities.md#amb-19)).

## Gaps

[AMB-15](../10-planning/ambiguities.md#amb-15) · [AMB-19](../10-planning/ambiguities.md#amb-19) · [AMB-20](../10-planning/ambiguities.md#amb-20) (Sentinel vs Cluster; no standby under Compose) · [AMB-34](../10-planning/ambiguities.md#amb-34) (repository layout and CI pipeline detail)
