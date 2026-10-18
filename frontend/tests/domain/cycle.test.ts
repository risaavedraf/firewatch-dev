import { describe, expect, test } from 'vitest';

import { evaluateCycle } from '../../src/domain/cycle';

// S3 — cycle freshness for `GET /v1/risk/sources`. The service publishes per-source
// status in a `sources` ARRAY (there is no `cycle` object and no `status` field);
// the shape is pinned against the real payload served by
// `backend/services/risk/internal/handler/sources.go` from
// `backend/services/risk/internal/cycle/runner.go` (`SourceStatus`). These tests
// pin that real shape, not an assumed one.

const NOW = Date.parse('2026-01-01T12:00:00.000Z');

function envelope(sources: unknown): Record<string, unknown> {
  return {
    geography: { provenance: {}, integrity: {}, cutoff_semantics: '' },
    risk_input: {},
    sources,
  };
}

function source(overrides: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    source_id: 'fixture',
    last_run_at: '2026-01-01T11:45:00Z',
    next_run_at: '2026-01-01T12:00:00Z',
    complete: true,
    age_seconds: 900,
    ...overrides,
  };
}

describe('cycle freshness', () => {
  test('is unavailable when the sources array is absent, null, not an array or holds a non-object entry', () => {
    for (const body of [
      envelope(undefined),
      envelope(null),
      envelope({}),
      envelope('sources'),
      envelope([{ source_id: 'a' }, 'not-an-entry']),
      {},
    ]) {
      const cycle = evaluateCycle(body, NOW);
      expect(cycle.available).toBe(false);
      expect(cycle.lastRunAt).toBe('');
      expect(cycle.nextRunAt).toBe('');
      expect(cycle.complete).toBeNull();
      expect(cycle.ageSeconds).toBeNull();
      expect(cycle.lastError).toBe('');
      expect(cycle.label).toMatch(/no informa/i);
    }
  });

  test('an empty sources array is reported as no sources, never as a fresh zero', () => {
    const cycle = evaluateCycle(envelope([]), NOW);
    expect(cycle.available).toBe(false);
    expect(cycle.sourceCount).toBe(0);
    expect(cycle.ageSeconds).toBeNull();
    expect(cycle.label).toMatch(/no informa|no reporta/i);
  });

  test('derives the last run as the latest non-null across sources and the next run as the earliest', () => {
    const cycle = evaluateCycle(
      envelope([
        source({ source_id: 'a', last_run_at: '2026-01-01T11:00:00Z', next_run_at: '2026-01-01T12:20:00Z', age_seconds: 3600 }),
        source({ source_id: 'b', last_run_at: '2026-01-01T11:45:00Z', next_run_at: '2026-01-01T12:00:00Z', age_seconds: 900 }),
        source({ source_id: 'c', last_run_at: null, next_run_at: '2026-01-01T12:10:00Z', age_seconds: null }),
      ]),
      NOW,
    );

    expect(cycle.available).toBe(true);
    expect(cycle.sourceCount).toBe(3);
    expect(cycle.lastRunAt).toBe('2026-01-01T11:45:00Z');
    expect(cycle.nextRunAt).toBe('2026-01-01T12:00:00Z');
    // The age belongs to the source that set the last run ("b"), not the others.
    expect(cycle.ageSeconds).toBe(900);
  });

  test('reports the run complete only when every source reports complete', () => {
    const all = evaluateCycle(envelope([source(), source({ source_id: 'b' })]), NOW);
    expect(all.complete).toBe(true);
    expect(all.label).toContain('completa');

    const one = evaluateCycle(envelope([source(), source({ source_id: 'b', complete: false })]), NOW);
    expect(one.complete).toBe(false);
    expect(one.label).toContain('incompleta');

    // A source that does not state `complete` is not a complete run.
    const missing = evaluateCycle(envelope([source({ complete: undefined })]), NOW);
    expect(missing.complete).toBe(false);
  });

  test('derives the age from last_run_at when the selected source omits age_seconds', () => {
    const cycle = evaluateCycle(envelope([source({ age_seconds: undefined })]), NOW);
    expect(cycle.ageSeconds).toBe(900);
  });

  test('never fabricates an age from an unparseable, future or negative value', () => {
    expect(evaluateCycle(envelope([source({ age_seconds: undefined, last_run_at: 'ayer' })]), NOW).ageSeconds).toBeNull();
    expect(
      evaluateCycle(envelope([source({ age_seconds: undefined, last_run_at: '2026-01-01T13:00:00Z' })]), NOW).ageSeconds,
    ).toBeNull();
    // A negative stated age is not a usable number: the age falls back to the
    // parseable last run; with no usable last run there is no age at all.
    expect(evaluateCycle(envelope([source({ age_seconds: -5 })]), NOW).ageSeconds).toBe(900);
    expect(evaluateCycle(envelope([source({ age_seconds: -5, last_run_at: 'ayer' })]), NOW).ageSeconds).toBeNull();
  });

  test('surfaces the last error of the first source that reports one', () => {
    const cycle = evaluateCycle(
      envelope([
        source({ source_id: 'a', complete: true }),
        source({ source_id: 'b', complete: false, last_error: 'timeout upstream' }),
      ]),
      NOW,
    );
    expect(cycle.available).toBe(true);
    expect(cycle.lastError).toContain('b');
    expect(cycle.lastError).toContain('timeout upstream');
    expect(cycle.label).toContain('timeout upstream');
  });

  test('stays silent about last_error when no source reports one', () => {
    const cycle = evaluateCycle(envelope([source()]), NOW);
    expect(cycle.lastError).toBe('');
  });
});
