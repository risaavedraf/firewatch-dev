import { describe, expect, test } from 'vitest';

import { ZoneLevelMemory } from '../../src/domain/zone-levels';

function batch(zone: string, level: unknown, extras: Record<string, unknown> = {}): Record<string, unknown> {
  return {
    zone_id: zone,
    id: 'batch',
    job_id: 'job',
    demo: true,
    risk_level: level,
    factors: [{ name: 'temperatura', known: true, value: 1, contribution: 1, note: '' }],
    rationale: 'r',
    policy: { policy_id: 'p', params_hash: 'h', parameters: { w: '1' } },
    ...extras,
  };
}

describe('per-zone level memory', () => {
  test('a zone keeps the level of its published batch when the selection moves away', () => {
    const memory = new ZoneLevelMemory();
    memory.remember(batch('05102', 'medio'));
    memory.remember(batch('05101', 'alto'));
    // 05101 is no longer the selection, yet its colour survives.
    expect(memory.displayedLevel('05101', '05102', 'medio')).toBe('alto');
    expect(memory.displayedLevel('05102', '05102', 'medio')).toBe('medio');
    expect(memory.highCount()).toBe(1);
  });

  test('a zone whose latest batch is invalid loses any remembered colour', () => {
    const memory = new ZoneLevelMemory();
    memory.remember(batch('05101', 'alto'));
    memory.remember(batch('05101', 'HIGH'));
    expect(memory.levelFor('05101')).toBe('');
    expect(memory.displayedLevel('05101', '05103', '')).toBe('');
  });

  test('a zone with no published batch stays neutral', () => {
    const memory = new ZoneLevelMemory();
    expect(memory.displayedLevel('05101', '05101', '')).toBe('');
    expect(memory.displayedLevel('05101', '05103', '')).toBe('');
    expect(memory.highCount()).toBe(0);
  });

  test('desconocido is remembered as its own honest state', () => {
    const memory = new ZoneLevelMemory();
    expect(memory.remember(batch('05101', 'desconocido'))).toBe('desconocido');
    expect(memory.levelFor('05101')).toBe('desconocido');
    expect(memory.highCount()).toBe(0);
  });

  test('a batch without a usable zone id colours nothing', () => {
    const memory = new ZoneLevelMemory();
    expect(memory.remember(batch('', 'alto'))).toBe('alto');
    expect(memory.size).toBe(0);
  });

  test('logout clears every remembered colour', () => {
    const memory = new ZoneLevelMemory();
    memory.remember(batch('05101', 'alto'));
    memory.remember(batch('05102', 'medio'));
    memory.clear();
    expect(memory.size).toBe(0);
    expect(memory.displayedLevel('05101', '05102', '')).toBe('');
    expect(memory.highCount()).toBe(0);
  });
});
