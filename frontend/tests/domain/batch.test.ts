import { describe, expect, test } from 'vitest';

import { latestOutcome, presentBatch } from '../../src/domain/batch';

function validBatch(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    zone_id: '05101',
    id: 'batch',
    job_id: 'job',
    demo: true,
    risk_level: 'alto',
    factors: [{ name: 'temperatura', known: true, value: 30, contribution: 0.2, note: '' }],
    rationale: 'Regla académica',
    policy: { policy_id: 'academic-rule-v1', params_hash: 'hash', parameters: { weight: '0.2' } },
    notice: 'Sintético',
    ...overrides,
  };
}

describe('fail-closed batch acceptance', () => {
  test('accepts only a complete batch and reports its canonical level', () => {
    const presentation = presentBatch(validBatch());
    expect(presentation.valid).toBe(true);
    expect(presentation.omissions).toEqual([]);
    expect(presentation.policyValid).toBe(true);
    expect(presentation.level).toBe('alto');
  });

  test('names exactly which part of the contract the batch omitted', () => {
    expect(presentBatch(validBatch({ risk_level: 'HIGH' })).omissions).toEqual(['nivel']);
    expect(presentBatch(validBatch({ demo: undefined })).omissions).toEqual(['procedencia demo']);
    expect(presentBatch(validBatch({ factors: [] })).omissions).toEqual(['factores']);
    expect(presentBatch(validBatch({ rationale: '' })).omissions).toEqual(['justificación']);
    expect(presentBatch(validBatch({ policy: {} })).omissions).toEqual(['política']);
  });

  test('a malformed level, a non-boolean demo flag or a broken factor never yields a level', () => {
    for (const override of [
      { risk_level: 'HIGH' },
      { demo: 'true' },
      { demo: 1 },
      { factors: [] },
      { rationale: 42 },
      { policy: {} },
      { policy: { policy_id: 'p', params_hash: '', parameters: { w: '1' } } },
      { policy: { policy_id: 'p', params_hash: 'h', parameters: {} } },
      { policy: { policy_id: 'p', params_hash: 'h', parameters: { w: 1 } } },
    ]) {
      const presentation = presentBatch(validBatch(override));
      expect(presentation.valid).toBe(false);
      expect(presentation.level).toBe('');
    }
  });

  test('a factor without its note string is not a valid measurement', () => {
    const presentation = presentBatch(
      validBatch({ factors: [{ name: 'viento', known: true, value: 1, contribution: 1 }] }),
    );
    expect(presentation.valid).toBe(false);
    expect(presentation.omissions).toContain('factores');
  });

  test('keeps only the factors it can present, in order', () => {
    const presentation = presentBatch(
      validBatch({
        factors: [
          { name: 'ok', known: true, value: 1, contribution: 1, note: '' },
          { name: 'bad', known: 'yes', value: 1, contribution: 1, note: '' },
          { name: 'also-ok', known: false, value: 0, contribution: 0, note: 'sin dato' },
        ],
      }),
    );
    expect(presentation.factors.map((factor) => factor.name)).toEqual(['ok', 'also-ok']);
    expect(presentation.valid).toBe(false);
  });

  test('404 from latest means there is no published batch, never low risk', () => {
    expect(latestOutcome(404)).toBe('no-published-batch');
    expect(latestOutcome(404)).not.toBe('ok');
    // A missing or empty body can never be turned into a level.
    for (const body of [{}, null, undefined, { risk_level: 'bajo' }]) {
      const presentation = presentBatch(body);
      expect(presentation.valid).toBe(false);
      expect(presentation.level).toBe('');
    }
  });

  test('maps the latest HTTP outcomes without inventing a severity', () => {
    expect(latestOutcome(200)).toBe('ok');
    expect(latestOutcome(401)).toBe('unauthorized');
    expect(latestOutcome(403)).toBe('forbidden');
    expect(latestOutcome(500)).toBe('error');
    expect(latestOutcome(404)).toBe('no-published-batch');
  });
});
