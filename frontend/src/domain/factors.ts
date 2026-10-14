import { presentBatch, type ValidFactor } from './batch';
import { isFiniteNumber } from './guards';

// Every factor the batch carries comes with a real `contribution`, so the bar is
// a real proportion and never an invention: each known factor is drawn against
// the largest contribution of ITS OWN batch, and a factor the API did not
// measure gets no bar at all instead of a zero drawn as a measurement.

export interface FactorBar {
  factor: ValidFactor;
  /** Percentage of the largest measured contribution, or null when no bar is drawn. */
  widthPercent: number | null;
}

export function computeFactorBars(batch: unknown): FactorBar[] {
  const { factors } = presentBatch(batch);
  const measured = factors
    .filter((factor) => factor.known === true && isFiniteNumber(factor.contribution))
    .map((factor) => factor.contribution);
  const largest = measured.length > 0 ? Math.max(...measured) : 0;

  return factors.map((factor) => ({
    factor,
    widthPercent:
      factor.known === true && largest > 0 ? Math.round((factor.contribution / largest) * 100) : null,
  }));
}
