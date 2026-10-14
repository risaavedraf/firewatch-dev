// Payload shapes as the API actually serves them. These are *declarations*, not
// guarantees: the gateway is a separate origin written in another language, so
// nothing here is trusted at runtime. Every consumer validates at the boundary
// and fails closed, exactly as the retired page did — an unreadable or
// incomplete payload renders as "unavailable", never as a plausible value.

export type RiskLevel = 'bajo' | 'medio' | 'alto' | 'desconocido';

export const RISK_LEVELS: readonly RiskLevel[] = ['bajo', 'medio', 'alto', 'desconocido'];

export interface Factor {
  name: string;
  known: boolean;
  value: number;
  contribution: number;
  note?: string;
}

export interface RiskPolicy {
  policy_id: string;
  params_hash: string;
  parameters?: Record<string, unknown>;
}

export interface RiskBatch {
  zone_id: string;
  id: string;
  job_id: string;
  demo: boolean;
  risk_level: RiskLevel | string;
  factors: Factor[];
  rationale: string;
  policy: RiskPolicy;
  notice: string;
  geometry?: GeoGeometry;
  published_at?: string;
  valid_until?: string;
  requested_at?: string;
  model_version?: string;
  data_status?: string;
  scenario_id?: string | null;
  evaluated_at?: string | null;
}

export type GeoGeometry =
  | { type: 'Polygon'; coordinates: number[][][] }
  | { type: 'MultiPolygon'; coordinates: number[][][][] }
  | { type: 'Point'; coordinates: number[] };

export interface ZoneProperties {
  code: string;
  name: string;
  province?: string;
}

export interface ZoneFeature {
  type: 'Feature';
  properties: ZoneProperties;
  geometry: GeoGeometry;
}

export interface ZoneCollection {
  type: 'FeatureCollection';
  features: ZoneFeature[];
}

export interface SourceProvenance {
  provider: string;
  attribution: string;
  restriction: string;
  record_count: number;
  derived_sha256: string;
  integrity: string;
  source_cutoff_at: string;
  geography?: string;
  risk_input?: string;
}

// `/v1/risk/status` and `/v1/jobs/status` share this snapshot shape. A service is
// only ever reported as verified when the payload carries a fresh `checked_at`
// and a ready state with consistent dependencies; anything else is unknown.
export interface ServiceSnapshot {
  checked_at: string;
  ready: boolean;
  dependencies: Record<string, string>;
  dead_letter_depth?: number | null;
}

export interface Job {
  id: string;
  zone_id: string;
  state: string;
  batch_id?: string;
  scenario_id?: string | null;
  evaluated_at?: string | null;
  requested_at?: string;
}

export interface Scenario {
  id: string;
  description: string;
}

export interface AuditEntry {
  actor_id: string;
  action: string;
  job_id: string;
  trace_id: string;
  occurred_at: string;
}
