import { describe, expect, test } from 'vitest';

import {
  batchBelongsToZone,
  expectedBatchInHistory,
  historyBelongsToZone,
  isCurrentRequest,
  matchesBatch,
} from '../../src/domain/supersession';

const expected = { zone: '05101', job: 'job', batch: 'batch' };

function validBatch(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    zone_id: '05101',
    id: 'batch',
    job_id: 'job',
    demo: true,
    risk_level: 'alto',
    factors: [{ name: 'temperatura', known: true, value: 1, contribution: 1, note: '' }],
    rationale: 'r',
    policy: { policy_id: 'p', params_hash: 'h', parameters: { w: '1' } },
    ...overrides,
  };
}

describe('supersession fences', () => {
  test('a correlated batch must match zone, job and batch id', () => {
    expect(matchesBatch(validBatch(), expected)).toBe(true);
    expect(matchesBatch(validBatch({ job_id: 'other' }), expected)).toBe(false);
    expect(matchesBatch(validBatch({ zone_id: '05102' }), expected)).toBe(false);
    expect(matchesBatch(validBatch({ id: 'older' }), expected)).toBe(false);
    expect(matchesBatch(null, expected)).toBe(false);
    expect(matchesBatch({}, expected)).toBe(false);
  });

  test('a response is current only while its sequence and zone still own the view', () => {
    expect(isCurrentRequest(3, 3, '05101', '05101')).toBe(true);
    expect(isCurrentRequest(3, 4, '05101', '05101')).toBe(false);
    expect(isCurrentRequest(3, 3, '05101', '05102')).toBe(false);
  });

  test('a batch computed for another zone is discarded', () => {
    expect(batchBelongsToZone(validBatch(), '05101')).toBe(true);
    expect(batchBelongsToZone(validBatch(), '05102')).toBe(false);
    expect(batchBelongsToZone({ zone_id: '05102' }, '05101')).toBe(false);
    expect(batchBelongsToZone(null, '05101')).toBe(false);
  });

  test('a whole history is discarded when any row belongs to another zone', () => {
    expect(historyBelongsToZone([validBatch(), validBatch({ id: 'b2' })], '05101')).toBe(true);
    expect(historyBelongsToZone([validBatch(), validBatch({ zone_id: '05102' })], '05101')).toBe(false);
    expect(historyBelongsToZone(null, '05101')).toBe(false);
    expect(historyBelongsToZone([], '05101')).toBe(true);
  });

  test('the expected published batch only completes history when it is present and valid', () => {
    expect(expectedBatchInHistory([validBatch()], expected)).toBe(true);
    expect(expectedBatchInHistory([validBatch({ id: 'older', job_id: 'other' })], expected)).toBe(false);
    // Present but malformed: it cannot complete the correlation.
    expect(expectedBatchInHistory([validBatch({ factors: [] })], expected)).toBe(false);
    expect(expectedBatchInHistory(null, expected)).toBe(false);
  });
});
