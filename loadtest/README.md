# Load tests

*Source: PDF p.15 (k6, arrival-rate executors), p.34–36 (baseline comparison, T1, T6), owner decisions [ADR-034](../ARCHITECTURE_DECISIONS.md#adr-034) and [ADR-035](../ARCHITECTURE_DECISIONS.md#adr-035).*

## Phase 2: the unsafe naive baseline

> ⚠️ **The baseline deliberately oversells.** It exists only to measure what SurgeTix's protections buy. It is never a booking flow.

### Two keys, both required

| Key | How | Default |
|---|---|---|
| Build tag | `go build -tags unsafe_baseline ./cmd/booking ./cmd/gateway` | Absent. A normal build does not even link the baseline packages (`go list -deps` test, CI binary check) |
| Runtime flag | `SURGETIX_ENABLE_UNSAFE_BASELINE=true` in the **process environment** of those two binaries | `false`. On a normal build, `true` is a configuration error and the service refuses to start |

Keep the flag `false` in `.env`, because the other five services read `.env` too and would refuse to start. Set it only for the tagged Booking and Gateway processes.

### Run it

From the repo root, in Git Bash or any POSIX shell:

```sh
cp .env.example .env                 # once; set real local passwords
docker compose up -d --wait          # Postgres, Redis, Kafka

go build -o bin/ ./cmd/...                                            # normal build: all 7
go build -tags unsafe_baseline -o bin/unsafe/ ./cmd/booking ./cmd/gateway

for s in identity catalog waitingroom checkout notifier; do bin/$s & done
SURGETIX_ENABLE_UNSAFE_BASELINE=true bin/unsafe/booking &   # migrates schema "baseline", seeds 500 seats
SURGETIX_ENABLE_UNSAFE_BASELINE=true bin/unsafe/gateway &   # forwards the route to GATEWAY_BOOKING_URL
```

Both tagged services log a `WARN` line, `UNSAFE NAIVE BASELINE ENABLED`, at start-up. Then:

```sh
loadtest/baseline/run.sh 50 try 30s               # one run: reset -> k6 -> audit
LEVELS="50 100 250" loadtest/baseline/matrix.sh   # the approved matrix: warm-up + 3 runs per level
go run ./loadtest/baseline/summarize loadtest/results
```

`run.sh` checks first that the route answers through the Gateway (`422` for a malformed event id). If it gets anything else, it stops.

### What one run does

1. **Reset**: `internal/booking/baseline/sql/reset.sql`. All 500 seats become `AVAILABLE` and the bookings are deleted.
2. **Audit before**: `sql/audit.sql`, the same file the Go tests embed.
3. **Snapshots** of Booking and Gateway `/metrics`, then **sampling** once per second: process CPU seconds and RSS for both, and `docker stats` for Postgres.
4. **k6** (`grafana/k6:2.3.0` in Docker), `loadtest/k6/baseline-t1.js`:
   - `constant-arrival-rate`, offering 20,000 arrivals/s with at most *B* VUs, so at most *B* requests are in flight. The system is measured at *B* concurrent buyers. Arrivals that could not start are counted as `dropped_iterations`.
   - Each request asks for 2 adjacent seats picked deterministically from `(SEED, iteration)`, with a unique `X-Baseline-Buyer-ID`. There are no retries.
   - `201` and `409` are the expected answers. Anything else counts toward the error rate.
5. **Audit after** and an **environment record** (`env.txt`): CPU, RAM, OS, Docker VM limits, Go, k6 and Postgres versions, Postgres settings, Booking pool size, Gateway proxy settings, and the git commit.

Results go to `loadtest/results/<utc>-b<B>-<label>/`. They are generated artifacts and are not committed. The measured figures are transcribed into [the Phase 2 plan §8](../docs/10-planning/phase-2-naive-baseline.md#8-measured-results).

### Fairness (ADR-034)

The baseline is unprotected, not sabotaged:

- **No artificial waits:** no sleeps or delays on the request path. The test checkpoint is `nil` outside tests.
- **Default pool size:** Booking uses pgx's default pool, `max(4, NumCPU)` connections (12 on the reference machine), and records it.
- **No connection throttling:** the Gateway keeps up to 1,024 idle connections to Booking and applies the PDF's 1 s deadline.
- **No resource limits** on any container or process beyond Docker Desktop's VM size, which is recorded.
