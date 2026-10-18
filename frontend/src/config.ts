// Public configuration of the browser client. A bundled client has no secrets:
// every value here is visible to any visitor and none of them is a credential.
// The three values mirror the retired page exactly, because they are registered
// in services this repository does not own.

// Prefixes every gateway call. The page is served from its own origin (:4173)
// while the gateway stays on :8080, so every API URL is absolute.
export const gatewayOrigin = 'http://localhost:8080';
// The identity provider is NOT the gateway: `/token`, `/auth` and `/logout`
// resolve against this issuer and must never receive `gatewayOrigin`.
export const issuer = 'http://localhost:8081/realms/firewatch';
// Must match the realm client's registered valid redirect URI, and the gateway's
// `WEB_ORIGIN` must be exactly this string. The `localhost` spelling is
// load-bearing: `127.0.0.1:4173` is a different origin to a browser and fails
// both the login round trip and the cross-origin API calls.
export const redirectURI = 'http://localhost:4173/';
export const clientID = 'firewatch-demo';

// The PKCE transaction (`state` + `code_verifier`) is the only value the app
// writes to browser storage. It lives just long enough to survive the redirect
// and is removed both when the callback consumes it and on logout. No token is
// ever persisted.
export const pkceTransactionKey = 'firewatch.pkce.transaction';

export const routes = {
  ready: '/health/ready',
  zones: '/v1/risk/zones',
  latest: '/v1/risk/latest',
  history: '/v1/risk/history',
  sources: '/v1/risk/sources',
  riskStatus: '/v1/risk/status',
  jobsStatus: '/v1/jobs/status',
  scenarios: '/v1/risk/synthetic-scenarios',
  audit: '/v1/audit',
  // The frozen `prevention` contract (T12) and the model registry (T13). Both
  // are reached through the gateway, the only origin the page's `connect-src`
  // allows besides the identity provider.
  preventiveActions: '/v1/prevention/actions',
  models: '/v1/models',
} as const;

export function jobRoute(id: string): string {
  return `/v1/jobs/${encodeURIComponent(id)}`;
}

export function preventiveActionRoute(id: string): string {
  return `/v1/prevention/actions/${encodeURIComponent(id)}`;
}

export function preventiveTransitionRoute(id: string): string {
  return `${preventiveActionRoute(id)}/transitions`;
}

// Bounds and deadlines the retired page enforced. They are policy, not
// decoration: they are what keeps a stale or superseded answer from being shown
// as current, so they travel with the code that honours them.
export const timing = {
  // Job polling.
  pollIntervalMs: 2000,
  pollDeadlineMs: 65_000,
  maxPollAttempts: 30,
  // Gateway readiness refresh.
  pipelineRefreshMs: 30_000,
  pipelineDeadlineMs: 10_000,
  // A batch is treated as expired this long before its own `valid_until`, so the
  // page never claims validity it has just lost.
  validitySkewMs: 1000,
  // Upper bound for the single re-evaluation timer; a longer remaining lifetime
  // is re-checked on the next tick instead of scheduling one huge timer.
  validityMaxDelayMs: 3_600_000,
  // Age after which a snapshot without its own freshness bound is not trusted.
  fallbackStatusMaxAgeSeconds: 30,
} as const;
