import { describe, expect, test } from 'vitest';

import { evaluatePipeline, evaluateServiceStatus } from '../../src/domain/readiness';

const NOW = Date.parse('2026-01-01T12:00:00.000Z');

interface StatusFixture {
  checked_at: string;
  ready: boolean;
  reason: string;
  dependencies: Record<string, unknown>;
  [key: string]: unknown;
}

function statusBody(overrides: Record<string, unknown> = {}): StatusFixture {
  return {
    checked_at: new Date(NOW).toISOString(),
    ready: true,
    reason: '',
    dependencies: {
      database: { state: 'ready' },
      broker: { state: 'ready' },
      dead_letter: {
        state: 'ready',
        depth: 0,
        sampled_at: new Date(NOW).toISOString(),
        age_seconds: 0,
        max_age_seconds: 120,
        max_depth: 1,
      },
    },
    ...overrides,
  };
}

describe('service readiness', () => {
  test('verifies a service only with ready, a fresh checked_at and consistent dependencies', () => {
    const decision = evaluateServiceStatus({ status: 200, body: statusBody() }, NOW);
    expect(decision.outcome).toBe('verified');
    expect(decision.verified).toBe(true);
    expect(decision.label).toMatch(/verificado/i);
  });

  test('a snapshot dated in the future is never fresh, so it cannot be verified', () => {
    // Found by independent verification: `age <= maxAge` alone accepts a negative
    // age, so a clock skewed ahead would have kept the claim true forever.
    const future = statusBody({ checked_at: new Date(NOW + 60_000).toISOString() });
    const decision = evaluateServiceStatus({ status: 200, body: future }, NOW);
    expect(decision.outcome).toBe('unknown');
    expect(decision.verified).toBe(false);
    expect(decision.label).not.toMatch(/verificado/i);
  });

  test('is fresh only while the snapshot is inside its own max-age bound', () => {
    const onBound = statusBody({ checked_at: new Date(NOW - 120_000).toISOString() });
    expect(evaluateServiceStatus({ status: 200, body: onBound }, NOW).outcome).toBe('verified');

    const overBound = statusBody({ checked_at: new Date(NOW - 120_001).toISOString() });
    const decision = evaluateServiceStatus({ status: 200, body: overBound }, NOW);
    expect(decision.outcome).toBe('unknown');
    expect(decision.label).toMatch(/fuera de la cota/i);
  });

  test('falls back to the client bound when the payload omits its own max age', () => {
    const body = statusBody({ checked_at: new Date(NOW - 31_000).toISOString() });
    delete (body.dependencies.dead_letter as Record<string, unknown>).max_age_seconds;
    const decision = evaluateServiceStatus({ status: 200, body }, NOW);
    expect(decision.outcome).toBe('unknown');
    expect(decision.label).not.toMatch(/verificado/i);
  });

  test('a 503 dead-letter-unknown snapshot stays unknown, never green', () => {
    const degraded = statusBody({
      ready: false,
      reason: 'dead_letter_depth_unknown',
      dependencies: {
        database: { state: 'ready' },
        broker: { state: 'ready' },
        dead_letter: {
          state: 'unknown',
          depth: null,
          sampled_at: new Date(NOW - 60_000).toISOString(),
          age_seconds: 45,
          max_age_seconds: 6,
          max_depth: 1,
        },
      },
    });
    const decision = evaluateServiceStatus({ status: 503, body: degraded }, NOW);
    expect(decision.outcome).toBe('unknown');
    expect(decision.label).toMatch(/no se acredita como saludable/i);
  });

  test('403 reads as not authorized, never down', () => {
    for (const status of [401, 403]) {
      const decision = evaluateServiceStatus({ status, body: { code: 'forbidden' } }, NOW);
      expect(decision.outcome).toBe('unverified');
      expect(decision.unverified).toBe(true);
      expect(decision.label).toMatch(/no autorizado para comprobar/i);
      expect(decision.label).not.toMatch(/ca[ií]do|down/i);
    }
  });

  test('the pipeline is verified only when both services earn it', () => {
    const verified = evaluateServiceStatus({ status: 200, body: statusBody() }, NOW);
    expect(evaluatePipeline(verified, verified).verified).toBe(true);
    const denied = evaluateServiceStatus({ status: 403, body: {} }, NOW);
    const mixed = evaluatePipeline(verified, denied);
    expect(mixed.verified).toBe(false);
    expect(mixed.status).toMatch(/servicio de riesgo verificado/);
    expect(mixed.status).toMatch(/servicio de trabajos no autorizado para comprobar/);
    expect(mixed.status).toMatch(/Gateway/);
  });
});

// The exact copy the panel must show whenever the depth claim is not earned. A
// fully healthy snapshot that still reaches this string was rejected on the
// evidence itself, which is what makes the claim fail-closed.
const depthGap = ' · profundidad de cola no acreditada como conocida y dentro de la cota';
const stateGap = ' · estados de dependencia ausentes o contradictorios con "ready"';
const unknownLabel = (gaps: string) => `Desconocido: el estado no se acredita como saludable (HTTP 200${gaps}).`;
const verifiedLabel = 'Verificado: ready con checked_at fresco y profundidad de cola dentro de la cota.';

describe('the verified branch needs unearned-claim evidence', () => {
  const base = statusBody().dependencies;
  const dead = (extra: Record<string, unknown> = {}) => ({
    state: 'ready',
    depth: 0,
    sampled_at: new Date(NOW).toISOString(),
    age_seconds: 0,
    max_age_seconds: 120,
    max_depth: 1,
    ...extra,
  });
  const withDeadLetter = (dependencies: unknown) => statusBody({ dependencies });
  const without = (object: Record<string, unknown>, key: string): StatusFixture => {
    const copy = { ...object };
    delete copy[key];
    return copy as unknown as StatusFixture;
  };

  const cases: Array<[string, StatusFixture, string]> = [
    ['depth -1 unknown sentinel', withDeadLetter({ ...base, dead_letter: dead({ depth: -1, max_depth: 0 }) }), depthGap],
    ['depth negative', withDeadLetter({ ...base, dead_letter: dead({ depth: -2 }) }), depthGap],
    ['max_depth negative', withDeadLetter({ ...base, dead_letter: dead({ max_depth: -1 }) }), depthGap],
    ['depth null', withDeadLetter({ ...base, dead_letter: dead({ depth: null }) }), depthGap],
    ['max_depth null', withDeadLetter({ ...base, dead_letter: dead({ max_depth: null }) }), depthGap],
    ['depth absent', withDeadLetter({ ...base, dead_letter: without(dead(), 'depth') }), depthGap],
    ['max_depth absent', withDeadLetter({ ...base, dead_letter: without(dead(), 'max_depth') }), depthGap],
    ['depth non-numeric', withDeadLetter({ ...base, dead_letter: dead({ depth: '0' }) }), depthGap],
    ['max_depth non-numeric', withDeadLetter({ ...base, dead_letter: dead({ max_depth: '1' }) }), depthGap],
    ['depth over bound', withDeadLetter({ ...base, dead_letter: dead({ depth: 4 }) }), depthGap],
    ['dependencies absent', without(statusBody(), 'dependencies'), depthGap + stateGap],
    ['dependencies empty', withDeadLetter({}), depthGap + stateGap],
    ['dead letter absent', withDeadLetter({ database: { state: 'ready' }, broker: { state: 'ready' } }), depthGap + stateGap],
    ['dead letter degraded with a depth inside the bound', withDeadLetter({ ...base, dead_letter: dead({ state: 'degraded' }) }), stateGap],
    ['dead letter unknown with a numeric depth', withDeadLetter({ ...base, dead_letter: dead({ state: 'unknown' }) }), stateGap],
    ['broker degraded while ready is true', withDeadLetter({ ...base, broker: { state: 'degraded' } }), stateGap],
    ['broker unknown while ready is true', withDeadLetter({ ...base, broker: { state: 'unknown' } }), stateGap],
    ['broker absent', withDeadLetter({ database: { state: 'ready' }, dead_letter: dead() }), stateGap],
    ['database degraded while ready is true', withDeadLetter({ ...base, database: { state: 'degraded' } }), stateGap],
    ['database unknown while ready is true', withDeadLetter({ ...base, database: { state: 'unknown' } }), stateGap],
    ['database absent', withDeadLetter(without(base, 'database')), stateGap],
    ['database null', withDeadLetter({ ...base, database: null }), stateGap],
    ['database empty', withDeadLetter({ ...base, database: {} }), stateGap],
    ['database non-object', withDeadLetter({ ...base, database: 'ready' }), stateGap],
  ];

  for (const [label, body, gaps] of cases) {
    test(`${label} stays on the explicit unknown presentation`, () => {
      const risk = evaluateServiceStatus({ status: 200, body }, NOW);
      const jobs = evaluateServiceStatus({ status: 200, body }, NOW);
      expect(risk.outcome, label).toBe('unknown');
      expect(jobs.outcome, label).toBe('unknown');
      expect(risk.label, label).toBe(unknownLabel(gaps));
      expect(jobs.label, label).toBe(unknownLabel(gaps));
      expect(evaluatePipeline(risk, jobs).verified, label).toBe(false);
    });
  }

  test('a fully consistent healthy body still earns the label, including depth == max_depth == 0', () => {
    for (const healthy of [statusBody(), withDeadLetter({ ...base, dead_letter: dead({ depth: 0, max_depth: 0 }) })]) {
      const risk = evaluateServiceStatus({ status: 200, body: healthy }, NOW);
      const jobs = evaluateServiceStatus({ status: 200, body: healthy }, NOW);
      expect(risk.label).toBe(verifiedLabel);
      expect(jobs.label).toBe(verifiedLabel);
      expect(evaluatePipeline(risk, jobs).verified).toBe(true);
    }
  });
});
