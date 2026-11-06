#!/usr/bin/env bash
# The approved local naive-baseline matrix (AMB-43): for each concurrency level,
# one warm-up run (not recorded as evidence) and RUNS recorded runs, each with
# reset -> k6 -> audit.
#
#   loadtest/baseline/matrix.sh                 # 50 100 250
#   LEVELS="500" loadtest/baseline/matrix.sh    # 500 only if the machine stays stable
#
# Then: go run ./loadtest/baseline/summarize loadtest/results
set -euo pipefail
cd "$(dirname "$0")"

LEVELS=${LEVELS:-"50 100 250"}
RUNS=${RUNS:-3}
DURATION=${DURATION:-30s}
WARMUP_DURATION=${WARMUP_DURATION:-10s}

for b in $LEVELS; do
  ./run.sh "$b" warmup "$WARMUP_DURATION"
  for r in $(seq 1 "$RUNS"); do
    ./run.sh "$b" "r$r" "$DURATION"
  done
done
