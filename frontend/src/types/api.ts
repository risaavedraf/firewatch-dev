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
  notice?: unknown;
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

// `/v1/risk/sources` does not serve a flat record: it nests the provenance under
// `geography`, states integrity as an object with a boolean `matches`, and carries
// its own cutoff semantics. The retired page read exactly this shape and failed
// closed on any part of it, so the declaration follows the payload rather than a
// flattened convenience shape.
export interface SourceProvenance {
  geography: {
    provenance: {
      provider: string;
      attribution: string;
      restriction: string;
      derived_sha256: string;
      record_count: number;
    };
    integrity: { matches: boolean; algorithm?: string };
    source_cutoff_at?: string | null;
    cutoff_semantics: string;
  };
  risk_input?: { synthetic?: boolean; display_name?: string; id?: string } | null;
  // Per-source cycle state and freshness (front 1, T4). The real payload always
  // carries a non-null `sources` array of `SourceStatus`; it is typed `unknown`
  // on purpose, because the transport does not re-shape it and `domain/cycle.ts`
  // decides what each entry means.
  sources?: unknown;
  code?: string;
}

// A dependency states its own readiness under `state` (`ready`, `degraded`, …).
// It is never inferred from the enclosing `ready` flag, and a missing, null or
// non-object entry is missing evidence rather than agreement.
export interface DependencyState {
  state?: string;
  [key: string]: unknown;
}

// `/v1/risk/status` and `/v1/jobs/status` share this snapshot shape. A service is
// only ever reported as verified when the payload carries `ready: true`, an empty
// `reason`, a fresh `checked_at`, a known dead-letter depth inside the payload's
// own `max_age_seconds` bound and three dependencies that each state `ready`;
// anything else is unknown.
export interface ServiceSnapshot {
  ready: boolean;
  reason: string;
  checked_at: string;
  dependencies: {
    database?: DependencyState;
    broker?: DependencyState;
    dead_letter?: DependencyState & {
      max_age_seconds?: number;
      age_seconds?: number;
      depth?: number | null;
      max_depth?: number | null;
    };
  };
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

// ── Preventive actions (frozen `prevention` contract) ───────────────────────
//
// The fields are the columns of `prevention.actions` named in
// `odd/tasks/presentation-client-ready.md` §«Contrato congelado». Declarations
// are not guarantees: every payload is validated at the transport boundary and
// `action_history` is required by the detail route.

export interface PreventiveAction {
  id: string;
  community_code: string;
  title: string;
  description?: string;
  priority?: string;
  status: string;
  assignee?: string;
  due_date?: string | null;
  created_by?: string;
  created_at?: string;
  updated_at?: string;
}

export interface ActionHistoryEntry {
  id?: string;
  action_id?: string;
  from_status?: string | null;
  to_status: string;
  actor_sub?: string;
  note?: string;
  created_at?: string;
}

export interface PreventiveActionDetail extends PreventiveAction {
  history: ActionHistoryEntry[];
}
