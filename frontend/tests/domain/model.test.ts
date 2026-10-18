import { describe, expect, test } from 'vitest';

import { evaluateActiveModel } from '../../src/domain/model';

// S3 — the active model version and its `synthetic` mark (T6). The service owns
// the registry; this module reads the ONE active entry and reports the synthetic
// mark only when the payload declares it, never by inference.

describe('active model version', () => {
  test('is unavailable unless a single active entry states its version', () => {
    for (const body of [undefined, null, {}, [], [{ version: 'academic-rule-v1', active: false }]]) {
      const model = evaluateActiveModel(body);
      expect(model.available).toBe(false);
      expect(model.version).toBe('');
      expect(model.synthetic).toBeNull();
      expect(model.label).toMatch(/no informa/i);
    }
  });

  test('reads the active version and a declared synthetic mark from metrics', () => {
    const model = evaluateActiveModel([
      { version: 'academic-rule-v1', kind: 'rule', active: false, metrics: {} },
      { version: 'demo-fixture-v1', kind: 'placeholder', active: true, metrics: { synthetic: true } },
    ]);

    expect(model.available).toBe(true);
    expect(model.version).toBe('demo-fixture-v1');
    expect(model.synthetic).toBe(true);
    expect(model.label).toMatch(/sintétic/i);
  });

  test('honours a top-level synthetic mark as well as the one inside metrics', () => {
    expect(evaluateActiveModel([{ version: 'v', active: true, synthetic: false }]).synthetic).toBe(false);
    expect(evaluateActiveModel([{ version: 'v', active: true, metrics: { synthetic: false } }]).synthetic).toBe(false);
  });

  test('reports an undeclared synthetic mark as unknown, never as non-synthetic', () => {
    const model = evaluateActiveModel([{ version: 'academic-rule-v1', kind: 'rule', active: true, metrics: {} }]);
    expect(model.available).toBe(true);
    expect(model.version).toBe('academic-rule-v1');
    expect(model.synthetic).toBeNull();
    expect(model.label).toMatch(/no declara/i);
  });

  test('ignores malformed entries instead of choosing one by position', () => {
    const model = evaluateActiveModel([
      { active: true },
      { version: '', active: true },
      { version: 'firewatch-rule-v2', active: true, metrics: { synthetic: false } },
    ]);
    expect(model.available).toBe(true);
    expect(model.version).toBe('firewatch-rule-v2');
  });
});
