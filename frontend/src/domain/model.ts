// The active model version and its `synthetic` mark (`GET /v1/models`, T6).
//
// The service owns the registry (`risk.migrations.002_models` / `006_models_integrity`:
// `version`, `kind`, `params_hash`, `approved_at`, `metrics`, `active`, with at
// most one active row). This module reads the ONE active entry and reports the
// synthetic mark only when the payload declares it — a top-level `synthetic`
// boolean or `metrics.synthetic`. An undeclared mark is `null`, never `false`:
// the client does not invent a non-synthetic claim for a model it cannot verify.

export interface ActiveModel {
  readonly available: boolean;
  readonly version: string;
  readonly synthetic: boolean | null;
  readonly label: string;
}

const UNAVAILABLE = 'El servicio no informa una versión de modelo activa.';

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === 'object' && !Array.isArray(value);
}

// `true`/`false` are honoured; anything else (absent, null, a string) is
// "undeclared", which is not the same statement as "non-synthetic".
function syntheticMark(entry: Record<string, unknown>): boolean | null {
  if (typeof entry.synthetic === 'boolean') return entry.synthetic;
  const metrics = record(entry.metrics) ? entry.metrics : null;
  if (metrics !== null && typeof metrics.synthetic === 'boolean') return metrics.synthetic;
  return null;
}

export function evaluateActiveModel(value: unknown): ActiveModel {
  if (!Array.isArray(value)) {
    return { available: false, version: '', synthetic: null, label: UNAVAILABLE };
  }
  // A malformed entry never wins by position: only an entry that states a
  // non-empty version and `active === true` is eligible.
  const active = value.find(
    (entry): entry is Record<string, unknown> =>
      record(entry) && entry.active === true && typeof entry.version === 'string' && entry.version.trim() !== '',
  );
  if (active === undefined) {
    return { available: false, version: '', synthetic: null, label: UNAVAILABLE };
  }
  const version = typeof active.version === 'string' ? active.version : '';
  if (version === '') {
    return { available: false, version: '', synthetic: null, label: UNAVAILABLE };
  }
  const synthetic = syntheticMark(active);
  const label =
    synthetic === true
      ? 'La versión activa se declara sintética: no es un modelo entrenado ni aprobado para producción.'
      : synthetic === false
        ? 'La versión activa se declara no sintética según el servicio.'
        : 'La versión activa no declara si es sintética; el servicio no lo informa.';
  return { available: true, version, synthetic, label };
}
