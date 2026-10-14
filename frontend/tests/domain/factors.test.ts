import { describe, expect, test } from 'vitest';

import { computeFactorBars } from '../../src/domain/factors';

function batchWith(factors: unknown[]): Record<string, unknown> {
  return {
    zone_id: '05102',
    id: 'batch',
    job_id: 'job',
    demo: true,
    risk_level: 'alto',
    rationale: 'r',
    factors,
    policy: { policy_id: 'p', params_hash: 'h', parameters: { w: '1' } },
  };
}

describe('factor bars', () => {
  test('scales each contribution against the largest of the same batch', () => {
    const bars = computeFactorBars(
      batchWith([
        { name: 'temperatura', known: true, value: 30, contribution: 0.5, note: '' },
        { name: 'humedad', known: true, value: 10, contribution: 0.25, note: '' },
      ]),
    );
    expect(bars.map((bar) => bar.widthPercent)).toEqual([100, 50]);
  });

  test('a factor without data draws no bar at all', () => {
    const bars = computeFactorBars(
      batchWith([
        { name: 'temperatura', known: true, value: 30, contribution: 0.5, note: '' },
        { name: 'viento', known: false, value: 0, contribution: 0, note: 'sin dato' },
      ]),
    );
    expect(bars[0].widthPercent).toBe(100);
    expect(bars[1].widthPercent).toBeNull();
  });

  test('draws no bar when the batch measured no contribution at all', () => {
    const bars = computeFactorBars(
      batchWith([{ name: 'viento', known: false, value: 0, contribution: 0, note: 'sin dato' }]),
    );
    expect(bars.map((bar) => bar.widthPercent)).toEqual([null]);
  });

  test('lists every valid factor in order and drops invalid ones', () => {
    const bars = computeFactorBars(
      batchWith([
        { name: 'a', known: true, value: 1, contribution: 1, note: '' },
        { name: 'bad', known: 'yes', value: 1, contribution: 1, note: '' },
        { name: 'b', known: true, value: 2, contribution: 0.5, note: '' },
      ]),
    );
    expect(bars.map((bar) => bar.factor.name)).toEqual(['a', 'b']);
    expect(bars.map((bar) => bar.widthPercent)).toEqual([100, 50]);
  });

  test('an incomplete batch still only draws bars for its valid factors', () => {
    const bars = computeFactorBars({
      factors: [{ name: 'temperatura', known: true, value: 30, contribution: 0.4, note: '' }],
    });
    expect(bars.map((bar) => bar.widthPercent)).toEqual([100]);
  });
});
