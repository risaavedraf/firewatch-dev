// Transport for the FireWatch gateway.
//
// One function per consumed route, one outcome type: a call never throws for an
// expected condition and never returns a partial value. Every gateway URL is
// absolute against `gatewayOrigin`; the issuer is never used here, because
// `/token`, `/auth` and `/logout` belong to the session unit and to the identity
// provider alone.
//
// Three properties are load-bearing and travel with this code:
//   1. a monotonic sequence per route kind discards a superseded response, so a
//      slow answer for zone A can never be shown as the answer for zone B;
//   2. a response that lands after the view was reset (401, logout, failed
//      login) is discarded instead of repopulating it;
//   3. every read honours the caller's `AbortSignal` and a bounded deadline, so
//      a hung gateway cannot leave a request pending forever.
import { gatewayOrigin, jobRoute, routes } from '../config';
import type {
  AuditEntry,
  Job,
  RiskBatch,
  Scenario,
  ServiceSnapshot,
  SourceProvenance,
  ZoneCollection,
} from '../types/api';

import { getAccessToken, resetSession } from './session';

export type FailureKind = 'unavailable' | 'unauthorized' | 'forbidden' | 'malformed' | 'missing';

export interface Failure {
  kind: FailureKind;
  reason: string;
  status?: number;
  requestId?: string;
}

export type Outcome<T> = { ok: true; data: T } | { ok: false; failure: Failure };

// `config.routes` carries every GET route and the job read, but not the job
// creation path, so it is named here rather than duplicated as a bare literal.
const JOBS_ROUTE = '/v1/jobs';

// A single bounded deadline for one transport read. The retired page bounded its
// own status cycle at 10 s and fenced a hung fetch on the job path; the port
// applies the same bound to every read so no route can hang the view.
const DEFAULT_DEADLINE_MS = 10_000;

type RequestKind =
  | 'ready'
  | 'zones'
  | 'latest'
  | 'history'
  | 'sources'
  | 'riskStatus'
  | 'jobsStatus'
  | 'scenarios'
  | 'audit'
  | 'createJob'
  | 'job';

// The sequence is per kind on purpose: `latest` and `history` are issued
// together for one selection, so a shared counter would make each treat the
// other's just-started request as a supersession and discard its own answer.
const sequences = new Map<RequestKind, number>();

function nextSequence(kind: RequestKind): number {
  const next = (sequences.get(kind) ?? 0) + 1;
  sequences.set(kind, next);
  return next;
}

function isSuperseded(kind: RequestKind, sequence: number, token: string): boolean {
  return sequence !== (sequences.get(kind) ?? 0) || token !== getAccessToken();
}

function failureSpec(kind: FailureKind, reason: string, status?: number, requestId?: string): Failure {
  const failure: Failure = { kind, reason };
  if (status !== undefined) failure.status = status;
  if (requestId !== undefined && requestId !== '') failure.requestId = requestId;
  return failure;
}

function failureOutcome<T>(failure: Failure): Outcome<T> {
  return { ok: false, failure };
}

interface TransportRequest {
  method?: string;
  body?: string;
  headers?: Record<string, string>;
  // `false` only for the unauthenticated gateway probe; every `/v1/*` read
  // carries the bearer even when the token is empty, exactly as the retired
  // page did.
  authenticated?: boolean;
}

type TransportResult =
  | { state: 'ok'; status: number; body: unknown; requestId: string }
  | { state: 'empty'; status: number; requestId: string }
  | { state: 'httpError'; status: number; body: unknown; requestId: string }
  | { state: 'failed'; failure: Failure };

function abortable<T>(promise: Promise<T>, signal: AbortSignal): Promise<T> {
  if (signal.aborted) return Promise.reject(new DOMException('Aborted', 'AbortError'));
  return new Promise<T>((resolve, reject) => {
    const onAbort = (): void => reject(new DOMException('Aborted', 'AbortError'));
    signal.addEventListener('abort', onAbort, { once: true });
    promise.then(
      (value) => {
        signal.removeEventListener('abort', onAbort);
        resolve(value);
      },
      (error: unknown) => {
        signal.removeEventListener('abort', onAbort);
        reject(error);
      },
    );
  });
}

const jsonFailure = Symbol('json-failure');

async function readJson(response: Response): Promise<unknown> {
  try {
    return await response.json();
  } catch {
    return jsonFailure;
  }
}

async function requestJson(
  kind: RequestKind,
  url: string,
  request: TransportRequest,
  signal?: AbortSignal,
): Promise<TransportResult> {
  const sequence = nextSequence(kind);
  const token = getAccessToken();
  const controller = new AbortController();
  let stop: 'none' | 'aborted' | 'deadline' = 'none';
  const stopReason = (): 'none' | 'aborted' | 'deadline' => stop;
  const onCallerAbort = (): void => {
    stop = 'aborted';
    controller.abort();
  };
  if (signal) {
    if (signal.aborted) return { state: 'failed', failure: failureSpec('unavailable', 'aborted') };
    signal.addEventListener('abort', onCallerAbort, { once: true });
  }
  const deadline = setTimeout(() => {
    stop = 'deadline';
    controller.abort();
  }, DEFAULT_DEADLINE_MS);
  try {
    const headers: Record<string, string> = { ...(request.headers ?? {}) };
    if (request.authenticated !== false) headers.Authorization = `Bearer ${token}`;
    const response = await abortable(
      fetch(url, {
        method: request.method ?? 'GET',
        body: request.body,
        headers,
        cache: 'no-store',
        credentials: 'omit',
        signal: controller.signal,
      }),
      controller.signal,
    );
    // Supersession is checked BEFORE the status: a superseded 401 must not reset
    // the session of the selection that replaced it.
    if (isSuperseded(kind, sequence, token)) {
      return { state: 'failed', failure: failureSpec('unavailable', 'superseded') };
    }
    const requestId = response.headers.get('X-Request-ID') ?? '';
    if (response.status === 401) {
      resetSession();
      return { state: 'failed', failure: failureSpec('unauthorized', 'unauthorized', 401, requestId) };
    }
    if (response.status === 403) {
      // A denied permission is a stated outcome, never "the service is down".
      return { state: 'failed', failure: failureSpec('forbidden', 'forbidden', 403, requestId) };
    }
    if (response.status === 204) return { state: 'empty', status: 204, requestId };
    const body = await abortable(readJson(response), controller.signal);
    if (isSuperseded(kind, sequence, token)) {
      return { state: 'failed', failure: failureSpec('unavailable', 'superseded') };
    }
    if (body === jsonFailure) {
      return { state: 'failed', failure: failureSpec('malformed', 'malformed_json', response.status, requestId) };
    }
    if (response.ok) return { state: 'ok', status: response.status, body, requestId };
    return { state: 'httpError', status: response.status, body, requestId };
  } catch {
    // Read through the accessor: the assignments happen in callbacks, so the
    // control-flow analysis cannot see them and would narrow the variable to its
    // initial value.
    const reason = stopReason();
    if (reason === 'deadline') return { state: 'failed', failure: failureSpec('unavailable', 'deadline') };
    if (reason === 'aborted') return { state: 'failed', failure: failureSpec('unavailable', 'aborted') };
    return { state: 'failed', failure: failureSpec('unavailable', 'network') };
  } finally {
    clearTimeout(deadline);
    if (signal) signal.removeEventListener('abort', onCallerAbort);
  }
}

function reasonFromBody(body: unknown, fallback: string): string {
  if (record(body) && nonempty(body.code)) return body.code.trim();
  return fallback;
}

function httpFailure(result: { status: number; body: unknown; requestId: string }): Failure {
  const reason = reasonFromBody(result.body, `http_${result.status}`);
  const kind: FailureKind = result.status === 400 ? 'malformed' : result.status === 404 ? 'missing' : 'unavailable';
  return failureSpec(kind, reason, result.status, result.requestId);
}

function resultFailure<T>(result: TransportResult): Outcome<T> {
  if (result.state === 'failed') return failureOutcome<T>(result.failure);
  if (result.state === 'httpError') return failureOutcome<T>(httpFailure(result));
  return failureOutcome<T>(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
}

function guardOutcome<T>(result: TransportResult, guard: (body: unknown) => body is T): Outcome<T> {
  if (result.state !== 'ok') return resultFailure<T>(result);
  if (!guard(result.body)) {
    return failureOutcome<T>(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
  }
  return { ok: true, data: result.body };
}

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function nonempty(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== '';
}

function optionalString(value: unknown): boolean {
  return value === undefined || typeof value === 'string';
}

// --- Boundary guards -------------------------------------------------------
//
// The declarations in `types/api.ts` are not runtime guarantees. Each guard
// accepts only the shape the view can use; anything else is reported as
// `malformed` rather than rendered as a plausible partial value.

function isZoneCollection(value: unknown): value is ZoneCollection {
  return record(value) && value.type === 'FeatureCollection' && Array.isArray(value.features);
}

function isGeometry(value: unknown): boolean {
  if (!record(value)) return false;
  return (value.type === 'Polygon' || value.type === 'MultiPolygon' || value.type === 'Point') && Array.isArray(value.coordinates);
}

function isFactor(value: unknown): boolean {
  return (
    record(value) &&
    nonempty(value.name) &&
    typeof value.known === 'boolean' &&
    Number.isFinite(value.value) &&
    Number.isFinite(value.contribution) &&
    (value.note === undefined || typeof value.note === 'string')
  );
}

function isPolicy(value: unknown): boolean {
  return (
    record(value) &&
    nonempty(value.policy_id) &&
    nonempty(value.params_hash) &&
    (value.parameters === undefined || record(value.parameters))
  );
}

function nullableString(value: unknown): boolean {
  return value === undefined || value === null || typeof value === 'string';
}

function isRiskBatch(value: unknown): value is RiskBatch {
  if (!record(value)) return false;
  if (!nonempty(value.zone_id) || !nonempty(value.id) || !nonempty(value.job_id)) return false;
  if (typeof value.demo !== 'boolean') return false;
  if (typeof value.risk_level !== 'string') return false;
  if (!Array.isArray(value.factors) || !value.factors.every(isFactor)) return false;
  if (typeof value.rationale !== 'string') return false;
  if (!isPolicy(value.policy)) return false;
  if (typeof value.notice !== 'string') return false;
  if (value.geometry !== undefined && !isGeometry(value.geometry)) return false;
  if (!optionalString(value.published_at) || !optionalString(value.valid_until) || !optionalString(value.requested_at)) return false;
  if (!optionalString(value.model_version) || !optionalString(value.data_status)) return false;
  if (!nullableString(value.scenario_id) || !nullableString(value.evaluated_at)) return false;
  return true;
}

function isServiceSnapshot(value: unknown): value is ServiceSnapshot {
  return (
    record(value) &&
    typeof value.checked_at === 'string' &&
    typeof value.ready === 'boolean' &&
    record(value.dependencies) &&
    (value.dead_letter_depth === undefined || value.dead_letter_depth === null || Number.isFinite(value.dead_letter_depth))
  );
}

function isJob(value: unknown): value is Job {
  if (!record(value)) return false;
  if (!nonempty(value.id) || !nonempty(value.zone_id) || typeof value.state !== 'string') return false;
  if (!optionalString(value.batch_id) || !optionalString(value.requested_at)) return false;
  if (!nullableString(value.scenario_id) || !nullableString(value.evaluated_at)) return false;
  return true;
}

// An event is usable only when it states WHAT happened and WHEN. An omitted
// identifier is labelled in place by the view; it is not a reason to discard
// the register, and it is never filled in from another field.
function isAuditEntry(value: unknown): value is AuditEntry {
  return (
    record(value) &&
    nonempty(value.action) &&
    nonempty(value.occurred_at) &&
    optionalString(value.actor_id) &&
    optionalString(value.job_id) &&
    optionalString(value.trace_id)
  );
}

// --- Routes -----------------------------------------------------------------

// Gateway-only availability probe. A 204 proves the gateway process answered;
// it does not accredit the risk or jobs pipeline, and the view states so.
export async function ready(signal?: AbortSignal): Promise<Outcome<null>> {
  const result = await requestJson('ready', `${gatewayOrigin}${routes.ready}`, { authenticated: false }, signal);
  if (result.state === 'ok' || result.state === 'empty') return { ok: true, data: null };
  return resultFailure<null>(result);
}

export async function zones(signal?: AbortSignal): Promise<Outcome<ZoneCollection>> {
  const result = await requestJson('zones', `${gatewayOrigin}${routes.zones}`, { authenticated: true }, signal);
  return guardOutcome(result, isZoneCollection);
}

export async function latest(zoneId: string, signal?: AbortSignal): Promise<Outcome<RiskBatch | null>> {
  const url = `${gatewayOrigin}${routes.latest}?zone_id=${encodeURIComponent(zoneId)}`;
  const result = await requestJson('latest', url, { authenticated: true }, signal);
  // A 404 means no published batch, not low risk: the absence is data, and the
  // view must not invent a level for it.
  if (result.state === 'httpError' && result.status === 404) return { ok: true, data: null };
  if (result.state !== 'ok') return resultFailure<RiskBatch | null>(result);
  if (!isRiskBatch(result.body)) {
    return failureOutcome(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
  }
  if (result.body.zone_id !== zoneId) {
    return failureOutcome(failureSpec('malformed', 'zone_mismatch', result.status, result.requestId));
  }
  return { ok: true, data: result.body };
}

export async function history(zoneId: string, signal?: AbortSignal): Promise<Outcome<RiskBatch[]>> {
  const url = `${gatewayOrigin}${routes.history}?zone_id=${encodeURIComponent(zoneId)}`;
  const result = await requestJson('history', url, { authenticated: true }, signal);
  if (result.state !== 'ok') return resultFailure<RiskBatch[]>(result);
  if (!Array.isArray(result.body)) {
    return failureOutcome(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
  }
  const batches: RiskBatch[] = [];
  for (const entry of result.body) {
    if (!isRiskBatch(entry)) {
      return failureOutcome(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
    }
    if (entry.zone_id !== zoneId) {
      return failureOutcome(failureSpec('malformed', 'zone_mismatch', result.status, result.requestId));
    }
    batches.push(entry);
  }
  return { ok: true, data: batches };
}

// The served payload is nested under `geography` exactly as the declaration
// names it, so the transport only proves that structure and then passes the
// payload through UNCHANGED. It does not flatten it and it does not decide
// integrity: `matches: false`, a missing provider or a stale cutoff is a
// fail-closed decision the domain layer makes on the same body.
function isSourceProvenance(value: unknown): value is SourceProvenance {
  if (!record(value)) return false;
  const geography = record(value.geography) ? value.geography : null;
  const provenance = geography && record(geography.provenance) ? geography.provenance : null;
  const integrity = geography && record(geography.integrity) ? geography.integrity : null;
  if (geography === null || provenance === null || integrity === null) return false;
  return (
    typeof provenance.provider === 'string' &&
    typeof provenance.attribution === 'string' &&
    typeof provenance.restriction === 'string' &&
    typeof provenance.derived_sha256 === 'string' &&
    typeof provenance.record_count === 'number' &&
    Number.isFinite(provenance.record_count) &&
    typeof integrity.matches === 'boolean' &&
    typeof geography.cutoff_semantics === 'string'
  );
}

export async function sources(signal?: AbortSignal): Promise<Outcome<SourceProvenance>> {
  const result = await requestJson('sources', `${gatewayOrigin}${routes.sources}`, { authenticated: true }, signal);
  return guardOutcome(result, isSourceProvenance);
}

export async function riskStatus(signal?: AbortSignal): Promise<Outcome<ServiceSnapshot>> {
  const result = await requestJson('riskStatus', `${gatewayOrigin}${routes.riskStatus}`, { authenticated: true }, signal);
  return guardOutcome(result, isServiceSnapshot);
}

export async function jobsStatus(signal?: AbortSignal): Promise<Outcome<ServiceSnapshot>> {
  const result = await requestJson('jobsStatus', `${gatewayOrigin}${routes.jobsStatus}`, { authenticated: true }, signal);
  return guardOutcome(result, isServiceSnapshot);
}

// Fail-closed catalogue validation: only entries that state a non-empty `id`
// survive, and a missing `description` becomes an empty string rather than an
// invented label. A non-array catalogue is rejected by the caller as malformed.
export function parseScenarios(value: unknown): Scenario[] {
  if (!Array.isArray(value)) return [];
  const scenarios: Scenario[] = [];
  for (const entry of value) {
    if (!record(entry) || typeof entry.id !== 'string' || entry.id === '') continue;
    scenarios.push({ id: entry.id, description: typeof entry.description === 'string' ? entry.description : '' });
  }
  return scenarios;
}

export async function scenarios(signal?: AbortSignal): Promise<Outcome<Scenario[]>> {
  const result = await requestJson('scenarios', `${gatewayOrigin}${routes.scenarios}`, { authenticated: true }, signal);
  if (result.state !== 'ok') return resultFailure<Scenario[]>(result);
  if (!Array.isArray(result.body)) {
    return failureOutcome(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
  }
  return { ok: true, data: parseScenarios(result.body) };
}

// Admin-only and parameterless: the service bounds the answer itself, so the
// page never sends a limit or an offset. A 403 is returned as `forbidden`, and
// the view renders it explicitly instead of hiding the panel.
export async function audit(signal?: AbortSignal): Promise<Outcome<AuditEntry[]>> {
  const result = await requestJson('audit', `${gatewayOrigin}${routes.audit}`, { authenticated: true }, signal);
  if (result.state !== 'ok') return resultFailure<AuditEntry[]>(result);
  if (!Array.isArray(result.body) || !result.body.every(isAuditEntry)) {
    return failureOutcome(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
  }
  return { ok: true, data: result.body };
}

export async function createJob(
  input: { zoneId: string; scenarioId?: string; idempotencyKey: string },
  signal?: AbortSignal,
): Promise<Outcome<Job>> {
  // A new request supersedes any in-flight poll of the previous job.
  nextSequence('job');
  const body: Record<string, string> = { zone_id: input.zoneId };
  // The server rejects an explicit empty `scenario_id` with 400 while an omitted
  // field keeps the legacy fixture, so the field travels only when chosen.
  if (typeof input.scenarioId === 'string' && input.scenarioId !== '') body.scenario_id = input.scenarioId;
  const result = await requestJson(
    'createJob',
    `${gatewayOrigin}${JOBS_ROUTE}`,
    {
      method: 'POST',
      body: JSON.stringify(body),
      headers: { 'Content-Type': 'application/json', 'Idempotency-Key': input.idempotencyKey },
      authenticated: true,
    },
    signal,
  );
  if (result.state !== 'ok') return resultFailure<Job>(result);
  if (!isJob(result.body)) {
    return failureOutcome(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
  }
  if (result.body.zone_id !== input.zoneId) {
    return failureOutcome(failureSpec('malformed', 'zone_mismatch', result.status, result.requestId));
  }
  return { ok: true, data: result.body };
}

export async function job(id: string, signal?: AbortSignal): Promise<Outcome<Job>> {
  const result = await requestJson('job', `${gatewayOrigin}${jobRoute(id)}`, { authenticated: true }, signal);
  if (result.state !== 'ok') return resultFailure<Job>(result);
  if (!isJob(result.body)) {
    return failureOutcome(failureSpec('malformed', 'invalid_shape', result.status, result.requestId));
  }
  if (result.body.id !== id) {
    return failureOutcome(failureSpec('malformed', 'job_mismatch', result.status, result.requestId));
  }
  return { ok: true, data: result.body };
}
