// Zone shape classes for the territorial map.
//
// The class list is split on purpose: the risk fill (`level-*` or the neutral
// fallback) is one decision, and the selection outline is another. A zone with
// no published batch stays neutral, and it must still be able to show that it is
// selected — the selection is a navigation state, not a claim about risk.

import { canonicalLevel } from '../../domain/risk-level';

export const ZONE_SHAPE_CLASS = 'zone-shape';
export const ZONE_NEUTRAL_CLASS = 'zone-neutral';
export const ZONE_SELECTED_CLASS = 'zone-selected';

// A published canonical level paints the zone with its own token. Anything else
// — no level, a malformed value, or `''` — is neutral: the catalogue has no
// per-commune classification, so the view must never imply an assessment the API
// did not compute. `desconocido` is a real published level and does get its own
// class; only the ABSENCE of a level is neutral.
export function zoneLevelClass(level: unknown): string {
  const canonical = canonicalLevel(level);
  return canonical === '' ? ZONE_NEUTRAL_CLASS : `level-${canonical}`;
}

// The full class list a zone shape renders with. Selection is applied after the
// fill and regardless of it, so a selected unclassified zone is
// `zone-shape zone-neutral zone-selected`.
export function zoneShapeClasses(level: unknown, selected: boolean): string[] {
  const classes = [ZONE_SHAPE_CLASS, zoneLevelClass(level)];
  if (selected) classes.push(ZONE_SELECTED_CLASS);
  return classes;
}
