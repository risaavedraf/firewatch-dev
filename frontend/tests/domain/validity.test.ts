import { describe, expect, test } from 'vitest';

import { timing } from '../../src/config';
import { evaluateValidity, validityLabel } from '../../src/domain/validity';

const NOW = Date.parse('2026-01-01T12:00:00.000Z');

describe('batch operational validity', () => {
  test('expires exactly at valid_until minus the clock-skew margin', () => {
    // A bound exactly `validitySkewMs` ahead is already expired: the claim may
    // only ever end early, never survive the server bound it declares.
    const boundary = evaluateValidity(
      { valid_until: new Date(NOW + timing.validitySkewMs).toISOString() },
      NOW,
    );
    expect(boundary.state).toBe('expired');
    expect(boundary.nextDelayMs).toBeNull();

    const justValid = evaluateValidity(
      { valid_until: new Date(NOW + timing.validitySkewMs + 1).toISOString() },
      NOW,
    );
    expect(justValid.state).toBe('valid');
    expect(justValid.nextDelayMs).toBe(1);
  });

  test('caps the re-evaluation timer at timing.validityMaxDelayMs', () => {
    const far = evaluateValidity(
      { valid_until: new Date(NOW + timing.validityMaxDelayMs + 3_600_000).toISOString() },
      NOW,
    );
    expect(far.state).toBe('valid');
    expect(far.nextDelayMs).toBe(timing.validityMaxDelayMs);
  });

  test('reports a past bound as expired and any unusable bound as unknown', () => {
    const past = evaluateValidity(
      { published_at: '2026-01-01T00:00:00.000Z', valid_until: '2025-12-31T00:00:00.000Z' },
      NOW,
    );
    expect(past.state).toBe('expired');
    expect(past.nextDelayMs).toBeNull();

    for (const batch of [{}, { valid_until: 'not-a-date' }, { valid_until: null }, null, 'nope']) {
      const missing = evaluateValidity(batch, NOW);
      expect(missing.state).toBe('missing');
      expect(missing.deadline).toBeNull();
      expect(missing.nextDelayMs).toBeNull();
    }
  });

  test('separates the operational validity label from the synthetic recency notice', () => {
    const live = evaluateValidity(
      { published_at: '2025-12-31T00:00:00.000Z', valid_until: new Date(NOW + 60_000).toISOString() },
      NOW,
    );
    expect(validityLabel(live)).toMatch(/Vigente/);
    expect(validityLabel(live)).toMatch(/publicado/);
    expect(validityLabel(live)).not.toMatch(/vencida/i);

    const expired = evaluateValidity(
      { published_at: '2025-12-31T00:00:00.000Z', valid_until: new Date(NOW - 1).toISOString() },
      NOW,
    );
    expect(validityLabel(expired)).toMatch(/Vencida/);
    expect(validityLabel(evaluateValidity({}, NOW))).toMatch(/sin información/i);
  });
});
