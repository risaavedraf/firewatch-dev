import { describe, expect, test } from 'vitest';

import {
  ZONE_NEUTRAL_CLASS,
  ZONE_SELECTED_CLASS,
  ZONE_SHAPE_CLASS,
  zoneLevelClass,
  zoneShapeClasses,
} from '../../src/components/map/zone-classes';

describe('zone shape classes', () => {
  test('each canonical level paints the zone with its own token', () => {
    expect(zoneLevelClass('bajo')).toBe('level-bajo');
    expect(zoneLevelClass('medio')).toBe('level-medio');
    expect(zoneLevelClass('alto')).toBe('level-alto');
    // A published `desconocido` is a real state, not the same as no level.
    expect(zoneLevelClass('desconocido')).toBe('level-desconocido');
  });

  test('the absence of a published batch is neutral, and so is an unheard-of level', () => {
    expect(zoneLevelClass('')).toBe(ZONE_NEUTRAL_CLASS);
    expect(zoneLevelClass(undefined)).toBe(ZONE_NEUTRAL_CLASS);
    expect(zoneLevelClass(null)).toBe(ZONE_NEUTRAL_CLASS);
    expect(zoneLevelClass('crítico')).toBe(ZONE_NEUTRAL_CLASS);
    expect(zoneLevelClass(7)).toBe(ZONE_NEUTRAL_CLASS);
  });

  test('selection styling is independent of risk validity', () => {
    // An unclassified zone can still be selected; the neutral fill is unchanged.
    expect(zoneShapeClasses('', true)).toEqual([ZONE_SHAPE_CLASS, ZONE_NEUTRAL_CLASS, ZONE_SELECTED_CLASS]);
    // A classified zone keeps its fill and gains the outline.
    expect(zoneShapeClasses('alto', true)).toEqual([ZONE_SHAPE_CLASS, 'level-alto', ZONE_SELECTED_CLASS]);
    // Deselected zones keep the fill only.
    expect(zoneShapeClasses('medio', false)).toEqual([ZONE_SHAPE_CLASS, 'level-medio']);
    expect(zoneShapeClasses('', false)).toEqual([ZONE_SHAPE_CLASS, ZONE_NEUTRAL_CLASS]);
  });
});
