import type { RiskLevel } from '../types/api';
import { isFiniteNumber, isNonEmptyString, isRecord } from './guards';
import { canonicalLevel } from './risk-level';

// Fail-closed batch acceptance. A batch is only presentable when every part the
// panel relies on is present and well-formed; anything else is explicitly
// invalid so the view can name what was missing instead of showing a plausible
// severity it cannot support.

export interface ValidFactor {
  name: string;
  known: boolean;
  value: number;
  contribution: number;
  note: string;
}

export function isValidFactor(factor: unknown): factor is ValidFactor {
  return (
    isRecord(factor) &&
    isNonEmptyString(factor.name) &&
    typeof factor.known === 'boolean' &&
    typeof factor.note === 'string' &&
    isFiniteNumber(factor.value) &&
    isFiniteNumber(factor.contribution)
  );
}

export type BatchOmission = 'nivel' | 'procedencia demo' | 'factores' | 'justificación' | 'política';

export interface BatchPresentation {
  valid: boolean;
  omissions: BatchOmission[];
  /** Only the factors that are individually valid; the order is preserved. */
  factors: ValidFactor[];
  policyValid: boolean;
  /** The canonical level, or empty when the batch is not valid as a whole. */
  level: RiskLevel | '';
}

function policyIsValid(policy: unknown): boolean {
  return (
    isRecord(policy) &&
    isNonEmptyString(policy.policy_id) &&
    isNonEmptyString(policy.params_hash) &&
    isRecord(policy.parameters) &&
    Object.keys(policy.parameters).length > 0 &&
    Object.entries(policy.parameters).every(
      ([key, value]) => isNonEmptyString(key) && typeof value === 'string',
    )
  );
}

export function presentBatch(batch: unknown): BatchPresentation {
  const source = isRecord(batch) ? batch : {};
  const rawFactors = Array.isArray(source.factors) ? source.factors : [];
  const factors = rawFactors.filter(isValidFactor);
  const policyValid = policyIsValid(source.policy);

  const omissions: BatchOmission[] = [];
  if (canonicalLevel(source.risk_level) === '') omissions.push('nivel');
  if (typeof source.demo !== 'boolean') omissions.push('procedencia demo');
  if (rawFactors.length === 0 || !rawFactors.every(isValidFactor)) omissions.push('factores');
  if (!isNonEmptyString(source.rationale)) omissions.push('justificación');
  if (!policyValid) omissions.push('política');

  const valid = omissions.length === 0;
  return {
    valid,
    omissions,
    factors,
    policyValid,
    level: valid ? canonicalLevel(source.risk_level) : '',
  };
}

// The only outcomes the latest read may take. A 404 is "no published batch" — an
// absence the map paints neutral — and never a low risk that was never computed.
export type LatestOutcome = 'ok' | 'no-published-batch' | 'unauthorized' | 'forbidden' | 'error';

export function latestOutcome(status: number): LatestOutcome {
  if (status === 200) return 'ok';
  if (status === 401) return 'unauthorized';
  if (status === 403) return 'forbidden';
  if (status === 404) return 'no-published-batch';
  return 'error';
}
