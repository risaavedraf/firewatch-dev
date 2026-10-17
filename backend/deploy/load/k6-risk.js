// k6 load benchmark for FireWatch — front 6, S2.
//
// It reproduces the agreed initial scenario: 20 concurrent users, each querying
// the map about once every five seconds, which is the 4 RPS peak the sizing
// document states. The thresholds are the agreed objectives (p95 < 500 ms, error
// rate < 1 %, maximum 2 s in normal conditions); a run turns those objectives
// into a measurement, which is not done yet.
//
// The map query is authenticated, so the harness needs a real bearer token from
// the target realm. Pass it through the environment; never commit one.
//
//   k6 run -e TOKEN="$TOKEN" -e BASE_URL=http://localhost:8080 deploy/load/k6-risk.js
//
// The reported values (p95, p99, RPS, error rate) are what a pass/fail report and
// the capacity validation (S5) read.
import http from 'k6/http';
import { check } from 'k6';

const BASE_URL = __ENV.BASE_URL || 'http://localhost:8080';
const TOKEN = __ENV.TOKEN || '';
const ZONE = __ENV.ZONE_ID || 'valparaiso-demo';

export const options = {
  scenarios: {
    map_query: {
      // constant-arrival-rate models the load directly as a request rate, which is
      // what the 4 RPS objective is stated in, instead of deriving it from VUs.
      executor: 'constant-arrival-rate',
      rate: 4,
      timeUnit: '1s',
      duration: __ENV.DURATION || '3m',
      preAllocatedVUs: 20,
      maxVUs: 40,
    },
  },
  thresholds: {
    // The agreed objectives. p95 catches the steady-state contract; max catches
    // the 2 s ceiling for normal conditions.
    http_req_duration: ['p(95)<500', 'max<2000'],
    http_req_failed: ['rate<0.01'],
  },
};

export default function () {
  const res = http.get(`${BASE_URL}/v1/risk/latest?zone_id=${ZONE}`, {
    headers: { Authorization: `Bearer ${TOKEN}` },
  });
  check(res, {
    'status is 200': (r) => r.status === 200,
    // A 200 with an unreadable body would be a false pass: the map query must
    // return a risk level, not just a status code.
    'body carries a risk level': (r) => typeof r.body === 'string' && r.body.includes('risk_level'),
  });
}
