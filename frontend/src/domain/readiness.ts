import { timing } from '../config';
import { isFiniteNumber, isNonEmptyString, isRecord } from './guards';

// A service is verified ONLY when its own snapshot reports ready with a fresh
// `checked_at` and consistent dependencies. A 403 means "not allowed to check",
// not "down"; anything else is unknown, and a fully healthy-looking snapshot is
// still rejected when the queue-depth evidence is missing or contradictory.

export interface ServiceStatusInput {
  status: number;
  body: unknown;
}

export type ServiceOutcome = 'verified' | 'unknown' | 'unverified';

export interface ServiceStatusDecision {
  outcome: ServiceOutcome;
  verified: boolean;
  unknown: boolean;
  unverified: boolean;
  label: string;
}

export const VERIFIED_LABEL =
  'Verificado: ready con checked_at fresco y profundidad de cola dentro de la cota.';
export const UNVERIFIED_LABEL =
  'No autorizado para comprobar; el servicio queda sin verificar, sin marcarlo como no disponible.';

const DEPTH_GAP = ' · profundidad de cola no acreditada como conocida y dentro de la cota';
const STATE_GAP = ' · estados de dependencia ausentes o contradictorios con "ready"';
const STALE_GAP = ' · instantánea fuera de la cota de antigüedad del cliente';

function stateOf(dependency: unknown): string | null {
  return isRecord(dependency) && isNonEmptyString(dependency.state) ? dependency.state : null;
}

export function evaluateServiceStatus(input: ServiceStatusInput, now: number): ServiceStatusDecision {
  const { status } = input;
  const body = isRecord(input.body) ? input.body : null;

  if (status === 401 || status === 403) {
    return { outcome: 'unverified', verified: false, unknown: false, unverified: true, label: UNVERIFIED_LABEL };
  }

  const dependencies = body !== null && isRecord(body.dependencies) ? body.dependencies : null;
  const deadLetter = dependencies !== null && isRecord(dependencies.dead_letter) ? dependencies.dead_letter : null;

  const maxAge =
    deadLetter !== null && isFiniteNumber(deadLetter.max_age_seconds)
      ? deadLetter.max_age_seconds
      : timing.fallbackStatusMaxAgeSeconds;
  const checkedAt = body !== null ? Date.parse(String(body.checked_at ?? '')) : Number.NaN;
  const ageSeconds = (now - checkedAt) / 1000;
  // A snapshot dated in the FUTURE is not evidence of freshness: without this
  // bound a skewed or broken server clock would keep the claim true forever. The
  // direction is fail-closed on purpose — a slightly fast server clock makes the
  // snapshot unknown rather than verified.
  const fresh = Number.isFinite(checkedAt) && ageSeconds <= maxAge && ageSeconds >= 0;

  const deadAge = deadLetter !== null && isFiniteNumber(deadLetter.age_seconds) ? deadLetter.age_seconds : null;
  const deadMaxAge =
    deadLetter !== null && isFiniteNumber(deadLetter.max_age_seconds) ? deadLetter.max_age_seconds : null;
  const deadStale = deadAge !== null && deadMaxAge !== null && deadAge > deadMaxAge;

  // Depth and dependency evidence are a precondition of the "inside the bound"
  // claim: the backend's unknown sentinel is -1, deliberately outside the range
  // of a real depth (>= 0), so a missing or negative sample is missing evidence.
  const deadDepth =
    deadLetter !== null && isFiniteNumber(deadLetter.depth) && deadLetter.depth >= 0 ? deadLetter.depth : null;
  const deadMaxDepth =
    deadLetter !== null && isFiniteNumber(deadLetter.max_depth) && deadLetter.max_depth >= 0
      ? deadLetter.max_depth
      : null;
  const depthWithinBound = deadDepth !== null && deadMaxDepth !== null && deadDepth <= deadMaxDepth;

  // A dependency that contradicts `ready: true` is not evidence of health. The
  // database entry must be present as an object stating ready, because the
  // backend always serializes it: an absent, null or non-object entry is missing
  // evidence and never implicit agreement.
  const brokerReady = stateOf(dependencies !== null ? dependencies.broker : null) === 'ready';
  const deadLetterReady = stateOf(deadLetter) === 'ready';
  const databaseReady =
    dependencies !== null && isRecord(dependencies.database) && stateOf(dependencies.database) === 'ready';
  const dependenciesConsistent = brokerReady && deadLetterReady && databaseReady;

  if (
    status === 200 &&
    body !== null &&
    body.ready === true &&
    body.reason === '' &&
    fresh &&
    !deadStale &&
    depthWithinBound &&
    dependenciesConsistent
  ) {
    return { outcome: 'verified', verified: true, unknown: false, unverified: false, label: VERIFIED_LABEL };
  }

  const reason = body !== null && isNonEmptyString(body.reason) ? ` · ${body.reason}` : '';
  const stale = fresh ? '' : STALE_GAP;
  const gaps = `${depthWithinBound ? '' : DEPTH_GAP}${dependenciesConsistent ? '' : STATE_GAP}`;
  return {
    outcome: 'unknown',
    verified: false,
    unknown: true,
    unverified: false,
    label: `Desconocido: el estado no se acredita como saludable (HTTP ${status}${reason}${stale}${gaps}).`,
  };
}

export interface PipelineDecision {
  verified: boolean;
  badge: 'VERIFICADO' | 'SIN VERIFICAR';
  status: string;
}

export function evaluatePipeline(
  risk: ServiceStatusDecision,
  jobs: ServiceStatusDecision,
): PipelineDecision {
  const verified = risk.verified && jobs.verified;
  const label = (name: string, result: ServiceStatusDecision): string =>
    result.verified
      ? `${name} verificado`
      : result.unverified
        ? `${name} no autorizado para comprobar`
        : `${name} en estado desconocido`;
  return {
    verified,
    badge: verified ? 'VERIFICADO' : 'SIN VERIFICAR',
    status: verified
      ? 'Ambos servicios reportan ready con marca de tiempo fresca. La disponibilidad del gateway (204) no participa de esta verificación.'
      : `${label('servicio de riesgo', risk)}; ${label('servicio de trabajos', jobs)}. La tarjeta "Gateway" no acredita este estado.`,
  };
}
