/**
 * PCMI k6 load test — store + retrieve SLO validation
 *
 * Requirements
 * ────────────
 *   k6 v0.50+  (https://k6.io/docs/get-started/installation/)
 *
 * Required env vars
 * ──────────────────
 *   PCMI_BASE_URL   Base URL of the PCMI API, e.g. http://localhost:8000
 *   PCMI_API_KEY    API key (X-API-Key header)
 *
 * Run
 * ────
 *   k6 run scripts/load/k6_store_retrieve.js
 *
 * With custom env vars:
 *   PCMI_BASE_URL=https://api.example.com PCMI_API_KEY=secret k6 run scripts/load/k6_store_retrieve.js
 *
 * SLO thresholds enforced by this script
 * ────────────────────────────────────────
 *   store   P99 < 50ms
 *   retrieve P99 < 100ms
 */

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Trend } from 'k6/metrics';

const storeLatency = new Trend('store_duration', true);
const retrieveLatency = new Trend('retrieve_duration', true);

const BASE_URL = __ENV.PCMI_BASE_URL || 'http://localhost:8000';
const API_KEY = __ENV.PCMI_API_KEY || '';

export const options = {
  stages: [
    { duration: '1m', target: 50 },   // ramp up to 50 VUs over 1 min
    { duration: '1m', target: 50 },   // ramp up to 50 VUs over next 1 min (total 2 min ramp)
    { duration: '1m', target: 50 },   // sustain 50 VUs for 1 min
  ],
  thresholds: {
    // SLO: store P99 < 50ms
    store_duration: ['p(99)<50'],
    // SLO: retrieve P99 < 100ms
    retrieve_duration: ['p(99)<100'],
    // General HTTP failure rate guard
    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  const tenantID = `bench-tenant-${__VU}`;
  const path = `root.bench.vu_${__VU}.iter_${__ITER % 100}`;
  const content = `VU ${__VU} iteration ${__ITER}: benchmark memory payload for SLO validation`;

  const headers = {
    'Content-Type': 'application/json',
    'X-API-Key': API_KEY,
    'X-Tenant-ID': tenantID,
  };

  // ── Store ────────────────────────────────────────────────────────────────
  const storePayload = JSON.stringify({
    path: path,
    content: content,
    metadata: { source: 'k6-load-test' },
  });

  const storeRes = http.post(`${BASE_URL}/v1/memories`, storePayload, { headers });
  storeLatency.add(storeRes.timings.duration);
  check(storeRes, {
    'store: status 200 or 201': (r) => r.status === 200 || r.status === 201,
  });

  // ── Retrieve ─────────────────────────────────────────────────────────────
  const retrieveRes = http.get(
    `${BASE_URL}/v1/memories?path_prefix=${encodeURIComponent(path)}&limit=5`,
    { headers },
  );
  retrieveLatency.add(retrieveRes.timings.duration);
  check(retrieveRes, {
    'retrieve: status 200': (r) => r.status === 200,
  });

  sleep(0.05); // ~20 iterations/s per VU ceiling to stay within 500 req/s at 50 VUs
}
