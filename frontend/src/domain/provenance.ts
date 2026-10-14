import { isNonEmptyString, isRecord } from './guards';

// Everything the sources panel fails closed on. A 503, a malformed payload or a
// hash that does not match is explicitly unavailable and never a healthy-looking
// source; the restriction text is only ever reproduced, never reinterpreted.

export interface AcceptedProvenance {
  provider: string;
  attribution: string;
  restriction: string;
  recordCount: number;
  derivedSha256: string;
  integrityLabel: string;
  cutoffLabel: string;
  syntheticInput: boolean;
  riskInputLabel: string;
}

export interface ProvenanceUnavailable {
  available: false;
  code: string | null;
  reason: string;
}

export interface ProvenanceAvailable {
  available: true;
  provenance: AcceptedProvenance;
}

export type ProvenanceDecision = ProvenanceAvailable | ProvenanceUnavailable;

function cutoffLabel(geography: Record<string, unknown>): string {
  const explicit = isNonEmptyString(geography.source_cutoff_at) ? geography.source_cutoff_at : null;
  const cutoff = explicit ?? 'sin fecha de corte real; retrieved_at como sustituto';
  return `${cutoff} · ${String(geography.cutoff_semantics)}`;
}

function riskInputLabel(riskInput: Record<string, unknown> | null): { synthetic: boolean; label: string } {
  if (riskInput !== null && riskInput.synthetic === true) {
    const descriptor = isNonEmptyString(riskInput.display_name)
      ? riskInput.display_name
      : isNonEmptyString(riskInput.id)
        ? riskInput.id
        : 'sin identificar';
    return {
      synthetic: true,
      label: `Entrada de riesgo sintética (${descriptor}); no es riesgo real observado.`,
    };
  }
  return { synthetic: false, label: 'Entrada de riesgo no declarada como sintética por el servicio.' };
}

export function evaluateProvenance(status: number, body: unknown): ProvenanceDecision {
  const root = isRecord(body) ? body : {};
  const geography = isRecord(root.geography) ? root.geography : null;
  const provenance = geography !== null && isRecord(geography.provenance) ? geography.provenance : null;
  const integrity = geography !== null && isRecord(geography.integrity) ? geography.integrity : null;

  const usable =
    status === 200 &&
    provenance !== null &&
    integrity !== null &&
    integrity.matches === true &&
    isNonEmptyString(provenance.provider) &&
    isNonEmptyString(provenance.attribution) &&
    isNonEmptyString(provenance.restriction) &&
    isNonEmptyString(provenance.derived_sha256) &&
    typeof provenance.record_count === 'number' &&
    Number.isFinite(provenance.record_count) &&
    geography !== null &&
    isNonEmptyString(geography.cutoff_semantics);

  const code = isNonEmptyString(root.code) ? root.code : null;
  if (!usable) {
    return {
      available: false,
      code,
      reason: `Procedencia geográfica no disponible (HTTP ${status}${code !== null ? ` · ${code}` : ''}); la integridad no se acredita y no se muestra como fuente saludable.`,
    };
  }

  // Narrowed by the guard above; the casts are local and documented by it.
  const safeProvenance = provenance as Record<string, unknown>;
  const safeIntegrity = integrity as Record<string, unknown>;
  const safeGeography = geography as Record<string, unknown>;
  const riskInput = isRecord(root.risk_input) ? root.risk_input : null;
  const input = riskInputLabel(riskInput);

  return {
    available: true,
    provenance: {
      provider: String(safeProvenance.provider),
      attribution: String(safeProvenance.attribution),
      restriction: String(safeProvenance.restriction),
      recordCount: safeProvenance.record_count as number,
      derivedSha256: String(safeProvenance.derived_sha256),
      integrityLabel: `coincide (${isNonEmptyString(safeIntegrity.algorithm) ? safeIntegrity.algorithm : 'hash'} verificado por el servicio)`,
      cutoffLabel: cutoffLabel(safeGeography),
      syntheticInput: input.synthetic,
      riskInputLabel: input.label,
    },
  };
}
