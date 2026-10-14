import { presentBatch } from './batch';
import { isRecord } from './guards';

// The pure part of the anti-race fences. A response belongs to the view only
// while the sequence and zone it started with are still current, and a batch may
// only be applied to the zone it was computed for. Keeping these decisions pure
// lets the transport layer stay dumb about what "superseded" means.

export interface ExpectedBatch {
  zone: string;
  job: string;
  batch: string;
}

export function matchesBatch(batch: unknown, expected: ExpectedBatch): boolean {
  return (
    isRecord(batch) &&
    batch.zone_id === expected.zone &&
    batch.job_id === expected.job &&
    batch.id === expected.batch
  );
}

export function batchBelongsToZone(batch: unknown, zone: string): boolean {
  return isRecord(batch) && batch.zone_id === zone;
}

export function historyBelongsToZone(history: unknown, zone: string): boolean {
  return Array.isArray(history) && history.every((batch) => batchBelongsToZone(batch, zone));
}

/** The published job completes only when its own batch is present and valid. */
export function expectedBatchInHistory(history: unknown, expected: ExpectedBatch): boolean {
  return (
    Array.isArray(history) &&
    history.some((batch) => matchesBatch(batch, expected) && presentBatch(batch).valid)
  );
}

export function isCurrentRequest(
  sequence: number,
  currentSequence: number,
  requestedZone: string,
  selectedZone: string,
): boolean {
  return sequence === currentSequence && requestedZone === selectedZone;
}
