import { RISK_LEVELS, type RiskLevel } from '../types/api';

// The four tokens the API may serve. They are machine values: none is valid
// display text, and a value outside this set is not a level at all, so the guard
// returns the empty string instead of guessing a severity.
export const CANONICAL_LEVELS: readonly RiskLevel[] = RISK_LEVELS;

export function canonicalLevel(level: unknown): RiskLevel | '' {
  return typeof level === 'string' && (CANONICAL_LEVELS as readonly string[]).includes(level)
    ? (level as RiskLevel)
    : '';
}

export function isCanonicalLevel(level: unknown): level is RiskLevel {
  return canonicalLevel(level) !== '';
}

// "desconocido" is INFORMACIÓN INSUFICIENTE: a fourth honest state, never a
// severity and never a lowercase machine token. Anything unheard of is shown as
// explicitly unclassified rather than remapped to a plausible severity.
export function levelLabel(level: unknown): string {
  switch (canonicalLevel(level)) {
    case 'bajo':
      return 'BAJO';
    case 'medio':
      return 'MEDIO';
    case 'alto':
      return 'ALTO';
    case 'desconocido':
      return 'INFORMACIÓN INSUFICIENTE';
    default:
      return 'SIN CLASIFICAR';
  }
}
