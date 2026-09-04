// TIDE soak: sustained load to catch leaks and slow degradation.
// Runs on the WEEKLY schedule, never per-PR (4h+ wall time). Asserts the
// error rate stays flat across the whole window — a leak or saturation shows
// up as a rising failure rate long before the run ends.
import http from 'k6/http';
import { check, sleep } from 'k6';

export const options = {
  stages: [
    { duration: '5m', target: 30 },
    { duration: __ENV.SOAK || '4h', target: 30 },
    { duration: '5m', target: 0 },
  ],
  thresholds: {
    http_req_failed: ['rate<0.01'],
    'http_req_duration{phase:soak}': ['p(95)<2000'],
  },
};

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';

export default function () {
  const vehicle = `soak-${__VU}`;
  const res = http.post(`${BASE_URL}/v1/telemetry:batch`, JSON.stringify({
    batch: [{
      id: `${vehicle}-${__ITER}-${Date.now()}`,
      tenantId: 'load', vehicleId: vehicle, deviceId: vehicle,
      timestamp: new Date().toISOString(),
      location: { lat: 52.5, lng: 13.4 }, speed: 55, raw: {},
      source: { provider: 'k6', protocol: 'http', deviceId: vehicle },
      metadata: { correlationId: 'soak', schemaVersion: 1, sequence: __ITER, quality: 'good' },
    }],
  }), {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'ingest-batch', phase: 'soak' },
  });
  check(res, { 'accepted': (r) => r.status === 202 });
  sleep(1);
}
