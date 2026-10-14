import { timing } from '../config';
import { isNonEmptyString, isRecord } from './guards';

// Operational validity is a property of the batch window (`published_at` /
// `valid_until`), deliberately separate from any synthetic-input recency notice:
// a `published_at` timestamp is not observation freshness.

export type ValidityState = 'valid' | 'expired' | 'missing';

export interface ValidityResult {
  state: ValidityState;
  /** `valid_until` minus the clock-skew margin, or null when unparseable. */
  deadline: number | null;
  /** Bounded delay for the single re-evaluation timer, or null when there is none. */
  nextDelayMs: number | null;
  publishedAt: string | null;
  validUntil: string | null;
}

export function evaluateValidity(batch: unknown, now: number): ValidityResult {
  const source = isRecord(batch) ? batch : {};
  const publishedAt = isNonEmptyString(source.published_at) ? source.published_at : null;
  const validUntil = isNonEmptyString(source.valid_until) ? source.valid_until : null;
  const until = validUntil === null ? Number.NaN : Date.parse(validUntil);
  if (!Number.isFinite(until)) {
    return { state: 'missing', deadline: null, nextDelayMs: null, publishedAt, validUntil };
  }
  // Expire EARLY by the skew margin: a skewed client clock must never keep
  // asserting validity past the server bound the batch itself declares.
  const deadline = until - timing.validitySkewMs;
  if (deadline <= now) {
    return { state: 'expired', deadline, nextDelayMs: null, publishedAt, validUntil };
  }
  // A far expiry is re-checked in bounded steps instead of scheduling one huge
  // timer, so a clock jump cannot outlive the claim.
  const nextDelayMs = Math.min(deadline - now, timing.validityMaxDelayMs);
  return { state: 'valid', deadline, nextDelayMs, publishedAt, validUntil };
}

export function validityLabel(result: ValidityResult): string {
  const published =
    result.publishedAt !== null ? `publicado ${result.publishedAt}` : 'publicación sin fecha informada';
  const until = result.deadline !== null ? `válido hasta ${result.validUntil}` : 'sin vencimiento informado';
  if (result.state === 'missing') {
    return `Vigencia operacional del lote: sin información (${published}; ${until}).`;
  }
  return `Vigencia operacional del lote: ${result.state === 'expired' ? 'Vencida' : 'Vigente'} (${published}; ${until}).`;
}
