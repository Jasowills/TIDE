// TIDE spike with recovery detection. "Does it recover?" is an assertion:
// the post-spike window carries its own p95 threshold — if latency stays
// elevated after load drops, the run fails. Not a comment.
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '1m', target: 20 },   // normal
    { duration: '10s', target: 200 }, // spike to 10x
    { duration: '1m', target: 200 },  // sustain
    { duration: '10s', target: 20 },  // drop
    { duration: '2m', target: 20 },   // recovery
  ],
  thresholds: {
    'http_req_duration{phase:normal}': ['p(95)<1500'],
    'http_req_duration{phase:recovery}': ['p(95)<1800'],
    'http_req_failed{phase:recovery}': ['rate<0.01'],
    http_req_failed: ['rate<0.05'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TENANT = __ENV.TENANT || 'load';

let start = 0;
export function setup() {
  start = Date.now();
  return { start };
}

function phase() {
  const t = (Date.now() - start) / 1000;
  if (t < 60) return 'normal';
  if (t < 140) return 'spike';
  if (t < 150) return 'drop';
  return 'recovery';
}

export default function (data) {
  start = data.start;
  const vehicle = `spike-${__VU}`;
  const batch = Array.from({ length: 5 }, (_, i) => ({
    id: `${vehicle}-${__ITER * 5 + i}-${Date.now()}`,
    tenantId: TENANT,
    vehicleId: vehicle,
    deviceId: vehicle,
    timestamp: new Date().toISOString(),
    location: { lat: 52.5, lng: 13.4 },
    speed: 90,
    raw: {},
    source: { provider: 'k6', protocol: 'http', deviceId: vehicle },
    metadata: { correlationId: 'spike', schemaVersion: 1, sequence: __ITER * 5 + i, quality: 'good' },
  }));
  const res = http.post(`${BASE_URL}/v1/telemetry:batch`, JSON.stringify({ batch }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'ingest-batch', phase: phase() },
  });
  check(res, { 'accepted': (r) => r.status === 202 });
  sleep(0.2);
}
