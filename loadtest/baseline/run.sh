#!/usr/bin/env bash
# One reproducible naive-baseline run: reset -> run k6 -> audit (ADR-034, AMB-43).
#
#   loadtest/baseline/run.sh BUYERS [LABEL] [DURATION]
#
# Prerequisites (see loadtest/README.md):
#   * docker compose up -d --wait            (Postgres, Redis, Kafka)
#   * Booking and Gateway built with -tags unsafe_baseline and running with
#     SURGETIX_ENABLE_UNSAFE_BASELINE=true; the other services running normally
#   * .env in the repo root (passwords are read from it, never printed)
#
# Writes everything to $RESULTS_DIR/<utc-stamp>-b<BUYERS>-<LABEL>/:
#   k6-key.json, k6-summary.json   k6 results (key figures + full summary)
#   audit-before.csv, audit-after.csv
#   metrics-{booking,gateway}-{before,after}.txt   Prometheus snapshots
#   process-samples.csv            booking/gateway CPU seconds + RSS, 1/s
#   postgres-stats.csv             docker stats for Postgres, ~1/s
#   env.txt                        machine, versions, Postgres and pool settings
set -euo pipefail

BUYERS=${1:?usage: run.sh BUYERS [LABEL] [DURATION]}
LABEL=${2:-run}
DURATION=${3:-30s}

cd "$(dirname "$0")/../.."
K6_IMAGE=${K6_IMAGE:-grafana/k6:2.3.0}
GATEWAY_URL=${GATEWAY_URL:-http://localhost:18080}
BOOKING_URL=${BOOKING_URL:-http://localhost:18084}
K6_BASE_URL=${K6_BASE_URL:-http://host.docker.internal:18080}
OFFERED_RATE=${OFFERED_RATE:-20000}
SEED=${SEED:-20261106}
RESULTS_DIR=${RESULTS_DIR:-loadtest/results}

STAMP=$(date -u +%Y%m%dT%H%M%SZ)
RUN_ID="${STAMP}-b${BUYERS}-${LABEL}"
OUT="$RESULTS_DIR/$RUN_ID"
mkdir -p "$OUT"

[ -f .env ] || { echo "run.sh: .env not found in the repo root" >&2; exit 1; }
BOOKING_DB_PASSWORD=$(grep -E '^BOOKING_DB_PASSWORD=' .env | head -1 | cut -d= -f2-)

psql_booking() {
  docker compose exec -T -e PGPASSWORD="$BOOKING_DB_PASSWORD" postgres \
    psql -h localhost -U booking_svc -d booking_db -v ON_ERROR_STOP=1 "$@"
}
hostpath() { # docker -v needs a Windows path under Git Bash
  local p; p=$(cd "$1" && pwd)
  if command -v cygpath >/dev/null 2>&1; then cygpath -w "$p"; else echo "$p"; fi
}

# Preflight: the baseline route must answer through the Gateway. An invalid
# event id proves the path Gateway -> Booking -> handler (422) without booking.
code=$(curl -s -o /dev/null -w '%{http_code}' -X POST -H 'X-Baseline-Buyer-ID: preflight' \
  -d '{"seat_ids":[]}' "$GATEWAY_URL/v1/baseline/events/not-a-uuid/holds" || true)
if [ "$code" != "422" ]; then
  echo "run.sh: baseline route not reachable through the Gateway (got HTTP $code, want 422)." >&2
  echo "        Are Booking and Gateway built with -tags unsafe_baseline and SURGETIX_ENABLE_UNSAFE_BASELINE=true?" >&2
  exit 1
fi

echo "[$RUN_ID] reset"
psql_booking -q < internal/booking/baseline/sql/reset.sql
psql_booking --csv < internal/booking/baseline/sql/audit.sql > "$OUT/audit-before.csv"
curl -s "$BOOKING_URL/metrics" > "$OUT/metrics-booking-before.txt"
curl -s "$GATEWAY_URL/metrics" > "$OUT/metrics-gateway-before.txt"

PG_CTR=$(docker compose ps -q postgres)
echo "unix_ts,service,process_cpu_seconds_total,process_resident_memory_bytes" > "$OUT/process-samples.csv"
echo "unix_ts,container,cpu_percent,mem_usage" > "$OUT/postgres-stats.csv"
sample() {
  while :; do
    ts=$(date +%s)
    for svc in booking gateway; do
      url=$BOOKING_URL; [ "$svc" = gateway ] && url=$GATEWAY_URL
      vals=$(curl -s --max-time 1 "$url/metrics" | awk '/^process_cpu_seconds_total /{c=$2} /^process_resident_memory_bytes /{m=$2} END{print c","m}')
      echo "$ts,$svc,$vals" >> "$OUT/process-samples.csv"
    done
    docker stats --no-stream --format '{{.Name}},{{.CPUPerc}},{{.MemUsage}}' "$PG_CTR" | sed "s/^/$ts,/" >> "$OUT/postgres-stats.csv" || true
    sleep 1
  done
}
sample &
SAMPLER=$!
trap 'kill $SAMPLER 2>/dev/null || true' EXIT

echo "[$RUN_ID] k6: $BUYERS buyers, $DURATION, offered $OFFERED_RATE/s, seed $SEED"
MSYS_NO_PATHCONV=1 docker run --rm --add-host=host.docker.internal:host-gateway \
  -v "$(hostpath "$OUT"):/results" -v "$(hostpath loadtest/k6):/scripts:ro" \
  -e BASE_URL="$K6_BASE_URL" -e BUYERS="$BUYERS" -e DURATION="$DURATION" \
  -e OFFERED_RATE="$OFFERED_RATE" -e SEED="$SEED" -e RUN_ID="$RUN_ID" -e SUMMARY_DIR=/results \
  "$K6_IMAGE" run --quiet /scripts/baseline-t1.js > "$OUT/k6-stdout.txt" 2> "$OUT/k6-stderr.txt"

kill $SAMPLER 2>/dev/null || true
wait $SAMPLER 2>/dev/null || true
curl -s "$BOOKING_URL/metrics" > "$OUT/metrics-booking-after.txt"
curl -s "$GATEWAY_URL/metrics" > "$OUT/metrics-gateway-after.txt"

echo "[$RUN_ID] audit"
psql_booking --csv < internal/booking/baseline/sql/audit.sql > "$OUT/audit-after.csv"

{
  echo "run_id: $RUN_ID"
  echo "date_utc: $(date -u +%FT%TZ)"
  echo "git_commit: $(git rev-parse HEAD)"
  echo "git_uncommitted_files: $(git status --porcelain | wc -l | tr -d ' ')"
  echo "buyers: $BUYERS"
  echo "duration: $DURATION"
  echo "offered_rate_per_s: $OFFERED_RATE"
  echo "seed: $SEED"
  echo "k6_image: $K6_IMAGE"
  echo "k6_target: $K6_BASE_URL (k6 runs in Docker on the same machine)"
  echo "go: $(go version 2>/dev/null || echo unknown)"
  docker version --format 'docker_server: {{.Server.Version}}' 2>/dev/null || true
  docker info --format 'docker_vm_cpus: {{.NCPU}}{{"\n"}}docker_vm_mem_bytes: {{.MemTotal}}{{"\n"}}docker_os: {{.OperatingSystem}}' 2>/dev/null || true
  if command -v powershell >/dev/null 2>&1; then
    powershell -NoProfile -Command '"host_cpu: " + (Get-CimInstance Win32_Processor).Name; "host_logical_cpus: " + (Get-CimInstance Win32_Processor).NumberOfLogicalProcessors; "host_mem_gb: " + [math]::Round((Get-CimInstance Win32_ComputerSystem).TotalPhysicalMemory/1GB,1); "host_os: " + (Get-CimInstance Win32_OperatingSystem).Caption' 2>/dev/null | tr -d '\r'
  else
    echo "host_cpu: $(grep -m1 'model name' /proc/cpuinfo 2>/dev/null | cut -d: -f2- | sed 's/^ //')"
    echo "host_logical_cpus: $(nproc 2>/dev/null)"
  fi
  echo "postgres_version: $(psql_booking -At -c 'SELECT version()')"
  psql_booking -At -c "SELECT 'postgres_' || name || ': ' || current_setting(name) FROM pg_settings
    WHERE name IN ('max_connections','shared_buffers','work_mem','synchronous_commit','fsync',
                   'default_transaction_isolation','effective_cache_size','max_wal_size') ORDER BY name"
  grep -E '^pgxpool_max_conns ' "$OUT/metrics-booking-after.txt" | sed 's/^/booking_/; s/ /: /'
  echo "gateway_upstream: max idle conns per host 1024, deadline 1s"
} > "$OUT/env.txt"

echo "[$RUN_ID] results in $OUT"
cat "$OUT/k6-stdout.txt"
cat "$OUT/audit-after.csv"
