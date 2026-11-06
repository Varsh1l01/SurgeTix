// T1-shaped contention against the Phase 2 UNSAFE NAIVE BASELINE (ADR-034).
//
// B concurrent buyers compete for the same 500 seeded seats through the
// Gateway. Each request asks for 2 adjacent seats. Demand far exceeds supply,
// as in T1 (5,000 buyers on 500 seats); the full T1 scale is a future
// distributed run (AMB-43).
//
// Executor: constant-arrival-rate (PDF p.15) configured to SATURATE: it offers
// OFFERED_RATE arrivals/s but may use at most BUYERS VUs, so at most BUYERS
// requests are in flight at once. The run therefore measures the system at B
// concurrent buyers. Arrivals k6 could not start because all B buyers were
// busy are reported as dropped_iterations.
//
// Every request is unique: buyer id = RUN_ID + global iteration number. The
// seat pair is a deterministic function of the iteration number and SEED, so
// a run's request sequence is reproducible. No retries.
//
// Env: BASE_URL, BUYERS, DURATION, OFFERED_RATE, RUN_ID, SEED, SUMMARY_DIR.

import http from 'k6/http';
import exec from 'k6/execution';
import { Counter, Trend } from 'k6/metrics';

const BASE_URL = __ENV.BASE_URL || 'http://host.docker.internal:18080';
const BUYERS = parseInt(__ENV.BUYERS || '50', 10);
const DURATION = __ENV.DURATION || '30s';
const OFFERED_RATE = parseInt(__ENV.OFFERED_RATE || '20000', 10);
const RUN_ID = __ENV.RUN_ID || `run-${Date.now()}`;
const SEED = parseInt(__ENV.SEED || '20261106', 10);
const SUMMARY_DIR = __ENV.SUMMARY_DIR || '/results';

const EVENT_ID = '5e1f0000-0000-4000-8000-000000000001';
const SEAT_COUNT = 500;
const SEATS_PER_REQUEST = 2;
const URL = `${BASE_URL}/v1/baseline/events/${EVENT_ID}/holds`;

const seatID = (n) => `00000000-0000-4000-8000-${String(n).padStart(12, '0')}`;

// mulberry32: small, deterministic PRNG step keyed by (seed, iteration).
function pick(i) {
  let t = (SEED + Math.imul(i + 1, 0x9e3779b9)) >>> 0;
  t = Math.imul(t ^ (t >>> 15), t | 1);
  t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
  t = ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  return 1 + Math.floor(t * (SEAT_COUNT - SEATS_PER_REQUEST + 1)); // first seat of the pair
}

export const options = {
  discardResponseBodies: true,
  summaryTrendStats: ['avg', 'min', 'med', 'p(90)', 'p(95)', 'p(99)', 'max'],
  scenarios: {
    t1_contention: {
      executor: 'constant-arrival-rate',
      rate: OFFERED_RATE,
      timeUnit: '1s',
      duration: DURATION,
      preAllocatedVUs: BUYERS,
      maxVUs: BUYERS,
    },
  },
};

// 201 (booked) and 409 (seats taken) are the two expected answers.
// Anything else (4xx, 5xx, 504 gateway deadline, network errors) is an error.
http.setResponseCallback(http.expectedStatuses(201, 409));

const booked = new Counter('bookings_succeeded');
const rejected = new Counter('bookings_rejected_unavailable');
const failed = new Counter('bookings_failed_other');
const latencyBooked = new Trend('latency_booked', true);
const latencyRejected = new Trend('latency_rejected', true);

export default function () {
  const i = exec.scenario.iterationInTest;
  const first = pick(i);
  const seats = [];
  for (let k = 0; k < SEATS_PER_REQUEST; k++) seats.push(seatID(first + k));

  const res = http.post(URL, JSON.stringify({ seat_ids: seats }), {
    headers: { 'Content-Type': 'application/json', 'X-Baseline-Buyer-ID': `${RUN_ID}-${i}` },
    timeout: '10s',
  });
  if (res.status === 201) {
    booked.add(1);
    latencyBooked.add(res.timings.duration);
  } else if (res.status === 409) {
    rejected.add(1);
    latencyRejected.add(res.timings.duration);
  } else {
    failed.add(1);
  }
}

function v(data, metric, key) {
  const m = data.metrics[metric];
  return m && m.values && key in m.values ? m.values[key] : null;
}

export function handleSummary(data) {
  const reqs = v(data, 'http_reqs', 'count') || 0;
  const key = {
    run_id: RUN_ID,
    buyers: BUYERS,
    duration: DURATION,
    offered_rate: OFFERED_RATE,
    seed: SEED,
    requests: reqs,
    requests_per_second: v(data, 'http_reqs', 'rate'),
    bookings_succeeded: v(data, 'bookings_succeeded', 'count') || 0,
    bookings_rejected_unavailable: v(data, 'bookings_rejected_unavailable', 'count') || 0,
    bookings_failed_other: v(data, 'bookings_failed_other', 'count') || 0,
    error_rate: v(data, 'http_req_failed', 'rate'),
    dropped_iterations: v(data, 'dropped_iterations', 'count') || 0,
    latency_ms: {
      p50: v(data, 'http_req_duration', 'med'),
      p95: v(data, 'http_req_duration', 'p(95)'),
      p99: v(data, 'http_req_duration', 'p(99)'),
      max: v(data, 'http_req_duration', 'max'),
    },
    latency_booked_ms: { p50: v(data, 'latency_booked', 'med'), p95: v(data, 'latency_booked', 'p(95)'), p99: v(data, 'latency_booked', 'p(99)') },
    latency_rejected_ms: { p50: v(data, 'latency_rejected', 'med'), p95: v(data, 'latency_rejected', 'p(95)'), p99: v(data, 'latency_rejected', 'p(99)') },
  };
  return {
    [`${SUMMARY_DIR}/k6-summary.json`]: JSON.stringify(data, null, 2),
    [`${SUMMARY_DIR}/k6-key.json`]: JSON.stringify(key, null, 2),
    stdout: JSON.stringify(key, null, 2) + '\n',
  };
}
