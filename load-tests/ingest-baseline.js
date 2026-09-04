// TIDE ingest baseline: constant realistic mix of writers + readers.
// Budgets live here as thresholds — a breach fails CI, not a quarterly review.
// Every point carries a unique (device, sequence) so dedup never absorbs load.
import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const ingestBatch = new Trend('ingest_batch_duration', true);

export const options = {
  scenarios: {
    ingest: {
      executor: 'ramping-vus',
      startVUs: 0,
      stages: [
        { duration: '20s', target: 20 }, // warmup: cold containers, pools, caches
        { duration: '100s', target: 20 }, // steady baseline
        { duration: '20s', target: 0 },
      ],
      exec: 'ingest',
      tags: { flow: 'ingest' },
    },
    queries: {
      executor: 'constant-vus', vus: 5, duration: '2m', exec: 'queries',
      tags: { flow: 'queries' },
    },
  },
  thresholds: {
    // Budgets (measured 2026-09-04 local compose + headroom; see docs/benchmarks.md).
    'http_req_duration{flow:ingest}': ['p(95)<1500', 'p(99)<3000'],
    'http_req_duration{flow:queries}': ['p(95)<500', 'p(99)<1500'],
    http_req_failed: ['rate<0.01'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TENANT = __ENV.TENANT || 'load';

function point(vehicle, seq) {
  return {
    id: `${vehicle}-${seq}`,
    tenantId: TENANT,
    vehicleId: vehicle,
    deviceId: vehicle,
    timestamp: new Date().toISOString(),
    location: { lat: 52.52 + Math.random() * 0.01, lng: 13.405 + Math.random() * 0.01 },
    speed: 20 + Math.random() * 60,
    raw: {},
    source: { provider: 'k6', protocol: 'http', deviceId: vehicle },
    metadata: { correlationId: `k6-${Date.now()}`, schemaVersion: 1, sequence: seq, quality: 'good' },
  };
}

export function ingest() {
  const vehicle = `k6-${__VU}`;
  const batch = Array.from({ length: 10 }, (_, i) => point(vehicle, __ITER * 10 + i));
  const res = http.post(`${BASE_URL}/v1/telemetry:batch`, JSON.stringify({ batch }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'ingest-batch' },
  });
  ingestBatch.add(res.timings.duration);
  check(res, {
    'batch accepted': (r) => r.status === 202,
    'batch count': (r) => (r.json('accepted') || 0) === 10,
  });
  sleep(0.5);
}

export function queries() {
  const ev = http.get(`${BASE_URL}/v1/events?tenant=${TENANT}`, { tags: { name: 'events' } });
  check(ev, { 'events 200': (r) => r.status === 200 });
  // Discover a real vehicle from the event stream instead of assuming one:
  // a hardcoded id (k6-1) silently 404s when VU numbering doesn't start the
  // ingest range at 1 — and a 200|404-tolerant check masked it completely.
  let vehicle = null;
  try {
    const list = ev.json();
    if (Array.isArray(list) && list.length > 0) vehicle = list[0].vehicleId;
  } catch (_) { /* malformed body: state check below fails loudly */ }
  if (vehicle) {
    const st = http.get(`${BASE_URL}/v1/vehicles/${vehicle}/state`, { tags: { name: 'state' } });
    check(st, { 'state 200': (r) => r.status === 200 });
  } else {
    const hz = http.get(`${BASE_URL}/healthz`, { tags: { name: 'healthz' } });
    check(hz, { 'healthz 200': (r) => r.status === 200 });
  }
  sleep(1);
}
