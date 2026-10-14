import { describe, expect, test } from 'vitest';

import { canonicalLevel, isCanonicalLevel, levelLabel } from '../../src/domain/risk-level';

describe('risk level presentation', () => {
  test('maps every canonical level, including desconocido, to its display label', () => {
    expect(levelLabel('bajo')).toBe('BAJO');
    expect(levelLabel('medio')).toBe('MEDIO');
    expect(levelLabel('alto')).toBe('ALTO');
    // "desconocido" is honest information insufficiency: a fourth state, never a
    // severity and never the lowercase machine token.
    expect(levelLabel('desconocido')).toBe('INFORMACIÓN INSUFICIENTE');
  });

  test('the canonical guard accepts only the four served tokens', () => {
    for (const level of ['bajo', 'medio', 'alto', 'desconocido']) {
      expect(canonicalLevel(level)).toBe(level);
      expect(isCanonicalLevel(level)).toBe(true);
    }
    for (const malformed of ['HIGH', 'BAJO', 'bajo ', '', null, undefined, 42, {}, []]) {
      expect(canonicalLevel(malformed)).toBe('');
      expect(isCanonicalLevel(malformed)).toBe(false);
    }
  });

  test('an unknown or malformed level is never shown as a level', () => {
    for (const malformed of ['HIGH', 'BAJO', 'bajo ', '', null, undefined, 42, {}, []]) {
      expect(levelLabel(malformed)).toBe('SIN CLASIFICAR');
    }
  });
});
