import { presentBatch } from './batch';
import type { RiskLevel } from '../types/api';
import { isNonEmptyString, isRecord } from './guards';

// Canonical level remembered for every zone whose latest batch this session has
// already read, so the regional map keeps its colours when the selection moves
// away instead of greying out every comuna but one. It is filled from the reads
// the page already performs (no extra requests) and cleared on logout. A zone
// whose newest batch is invalid LOSES any remembered colour: the memory may only
// ever reflect the most recent published read.
export class ZoneLevelMemory {
  private readonly levels = new Map<string, RiskLevel>();

  /** Records the level of a displayable batch; returns the level it stored. */
  remember(batch: unknown): RiskLevel | '' {
    const source = isRecord(batch) ? batch : {};
    const zoneId = isNonEmptyString(source.zone_id) ? source.zone_id : '';
    const presentation = presentBatch(batch);
    const level = presentation.valid ? presentation.level : '';
    if (zoneId === '') return level;
    if (level === '') this.levels.delete(zoneId);
    else this.levels.set(zoneId, level);
    return level;
  }

  levelFor(zone: string): RiskLevel | '' {
    return this.levels.get(zone) ?? '';
  }

  /** The level a polygon is painted with: the live selection, else the memory. */
  displayedLevel(zone: string, selectedZone: string, activeLevel: RiskLevel | ''): RiskLevel | '' {
    return zone === selectedZone ? activeLevel : this.levelFor(zone);
  }

  highCount(): number {
    let count = 0;
    for (const level of this.levels.values()) if (level === 'alto') count += 1;
    return count;
  }

  clear(): void {
    this.levels.clear();
  }

  get size(): number {
    return this.levels.size;
  }
}
