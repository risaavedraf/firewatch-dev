// Cycle freshness for `GET /v1/risk/sources`.
//
// The real payload publishes per-source status in a `sources` ARRAY — there is no
// `cycle` object and no `status` field. The shape is pinned against the served
// payload: `backend/services/risk/internal/handler/sources.go` serialises
// `[]cycle.SourceStatus`, and `backend/services/risk/internal/cycle/runner.go`
// declares each entry as
//   { source_id, last_run_at: string|null, next_run_at: string|null,
//     complete: bool, age_seconds: number|null, last_error?: string }.
//
// The derivation, in the orchestrator's words:
//   * `last_run_at`  = the latest non-null across sources;
//   * `next_run_at`  = the earliest non-null;
//   * the run is complete only when EVERY source reports `complete`;
//   * `age_seconds`  = the service's number on the source that set the last run,
//                      otherwise derived from a parseable, non-future last run;
//   * `last_error`   = surfaced when any source reports one.
// A malformed or absent array is "not reported", never a zero value.

export interface CycleFreshness {
  readonly available: boolean;
  readonly lastRunAt: string;
  readonly nextRunAt: string;
  readonly complete: boolean | null;
  readonly ageSeconds: number | null;
  readonly lastError: string;
  readonly sourceCount: number;
  readonly label: string;
}

const UNAVAILABLE = 'El servicio no informa fuentes ni frescura del ciclo.';

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : '';
}

// `null` for an absent, non-string or unparseable timestamp; otherwise the epoch
// milliseconds. A value the service stated as null is simply "not reported".
function parsedAt(value: unknown): number | null {
  if (typeof value !== 'string' || value.trim() === '') return null;
  const parsed = Date.parse(value);
  return Number.isFinite(parsed) ? parsed : null;
}

interface SourceEntry {
  readonly sourceId: string;
  readonly lastRunAt: string;
  readonly lastRunMs: number | null;
  readonly nextRunAt: string;
  readonly nextRunMs: number | null;
  readonly complete: boolean;
  readonly ageSeconds: number | null;
  readonly lastError: string;
}

function readEntry(value: Record<string, unknown>): SourceEntry {
  const statedAge = value.age_seconds;
  return {
    sourceId: text(value.source_id),
    lastRunAt: text(value.last_run_at),
    lastRunMs: parsedAt(value.last_run_at),
    nextRunAt: text(value.next_run_at),
    nextRunMs: parsedAt(value.next_run_at),
    complete: value.complete === true,
    ageSeconds:
      typeof statedAge === 'number' && Number.isFinite(statedAge) && statedAge >= 0 ? Math.floor(statedAge) : null,
    lastError: text(value.last_error),
  };
}

export function evaluateCycle(payload: unknown, nowMs: number): CycleFreshness {
  const unavailable: CycleFreshness = {
    available: false,
    lastRunAt: '',
    nextRunAt: '',
    complete: null,
    ageSeconds: null,
    lastError: '',
    sourceCount: 0,
    label: UNAVAILABLE,
  };

  if (!record(payload) || !Array.isArray(payload.sources)) return unavailable;
  // Every entry must be an object: a malformed entry makes the whole array
  // unusable rather than being skipped into a partial "fresh" reading.
  if (!payload.sources.every(record)) return unavailable;
  if (payload.sources.length === 0) {
    return { ...unavailable, label: 'El servicio no reporta fuentes en el ciclo.' };
  }

  const entries = payload.sources.map(entry => readEntry(entry as Record<string, unknown>));

  // The source that set the latest last run: the age belongs to it.
  let selected: SourceEntry | null = null;
  for (const entry of entries) {
    if (entry.lastRunMs === null) continue;
    if (selected === null || entry.lastRunMs > (selected.lastRunMs as number)) selected = entry;
  }

  let earliestNext: SourceEntry | null = null;
  for (const entry of entries) {
    if (entry.nextRunMs === null) continue;
    if (earliestNext === null || entry.nextRunMs < (earliestNext.nextRunMs as number)) earliestNext = entry;
  }

  const complete = entries.every(entry => entry.complete);

  let ageSeconds: number | null = null;
  if (selected !== null) {
    if (selected.ageSeconds !== null) ageSeconds = selected.ageSeconds;
    else if ((selected.lastRunMs as number) <= nowMs) ageSeconds = Math.floor((nowMs - (selected.lastRunMs as number)) / 1000);
  }

  const errored = entries.find(entry => entry.lastError !== '') ?? null;
  const lastError = errored === null ? '' : errored.sourceId === '' ? errored.lastError : `${errored.sourceId}: ${errored.lastError}`;

  return {
    available: true,
    lastRunAt: selected === null ? '' : selected.lastRunAt,
    nextRunAt: earliestNext === null ? '' : earliestNext.nextRunAt,
    complete,
    ageSeconds,
    lastError,
    sourceCount: entries.length,
    label: `Ejecución del ciclo: ${complete ? 'completa' : 'incompleta'} · Última: ${
      selected === null ? 'no informada' : selected.lastRunAt
    } · Renovación esperada: ${earliestNext === null ? 'no informada' : earliestNext.nextRunAt} · Antigüedad: ${
      ageSeconds === null ? 'no informada' : `${ageSeconds} s`
    } · Fuentes: ${entries.length}.${lastError === '' ? '' : ` Último error: ${lastError}.`}`,
  };
}
