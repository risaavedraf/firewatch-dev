import { describe, expect, test } from 'vitest';

import {
  evaluateProvenance,
  type AcceptedProvenance,
  type ProvenanceDecision,
  type ProvenanceUnavailable,
} from '../../src/domain/provenance';

interface ProvenanceFixture {
  geography: {
    provenance: Record<string, unknown>;
    source_cutoff_at: string | null;
    cutoff_semantics?: string;
    integrity?: Record<string, unknown>;
  };
  risk_input: Record<string, unknown>;
}

function provenanceBody(overrides: Record<string, unknown> = {}): ProvenanceFixture {
  return {
    geography: {
      provenance: {
        provider: 'MOP - IDE Chile (SERVICIO_DPA, capa Comunas)',
        attribution: 'SUBDERE',
        restriction: 'debe ser revisada y validada por DIFROL',
        derived_sha256: 'c'.repeat(64),
        record_count: 38,
        ...overrides,
      },
      source_cutoff_at: null,
      cutoff_semantics: 'retrieval_substitution: stored cutoff_at equals retrieved_at',
      integrity: {
        algorithm: 'sha256',
        recorded_sha256: 'c'.repeat(64),
        computed_sha256: 'c'.repeat(64),
        matches: true,
      },
    },
    risk_input: { id: 'fixture-sample', display_name: 'Muestra sintética', synthetic: true },
  };
}

function unavailable(decision: ProvenanceDecision): ProvenanceUnavailable {
  if (decision.available) throw new Error('expected the payload to degrade to unavailable');
  return decision;
}

function available(decision: ProvenanceDecision): AcceptedProvenance {
  if (!decision.available) throw new Error('expected the payload to be accepted');
  return decision.provenance;
}

describe('provenance acceptance', () => {
  test('accepts a 200 payload with every published field and a matching hash', () => {
    const decision = available(evaluateProvenance(200, provenanceBody()));
    expect(decision.provider).toBe('MOP - IDE Chile (SERVICIO_DPA, capa Comunas)');
    expect(decision.attribution).toBe('SUBDERE');
    expect(decision.restriction).toBe('debe ser revisada y validada por DIFROL');
    expect(decision.recordCount).toBe(38);
    expect(decision.derivedSha256).toBe('c'.repeat(64));
    expect(decision.integrityLabel).toMatch(/coincide/i);
    // A null cutoff is a declared state: it is labelled, never invented.
    expect(decision.cutoffLabel).toMatch(/sin fecha de corte real/i);
  });

  test('fails closed on 503 and surfaces the service reason code', () => {
    const decision = unavailable(evaluateProvenance(503, { code: 'geography_integrity_unavailable' }));
    expect(decision.code).toBe('geography_integrity_unavailable');
    expect(decision.reason).toMatch(/no disponible/i);
  });

  test('a mismatched integrity hash degrades to unavailable even over HTTP 200', () => {
    const body = provenanceBody();
    (body.geography.integrity as Record<string, unknown>).matches = false;
    expect(evaluateProvenance(200, body).available).toBe(false);
  });

  test('requires every field the panel publishes', () => {
    for (const field of ['provider', 'attribution', 'restriction', 'derived_sha256', 'record_count']) {
      const body = provenanceBody();
      delete body.geography.provenance[field];
      expect(evaluateProvenance(200, body).available, field).toBe(false);
    }
    const noSemantics = provenanceBody();
    delete noSemantics.geography.cutoff_semantics;
    expect(evaluateProvenance(200, noSemantics).available).toBe(false);
    const noIntegrity = provenanceBody();
    delete noIntegrity.geography.integrity;
    expect(evaluateProvenance(200, noIntegrity).available).toBe(false);
  });

  test('a non-200 response never becomes available, even with a healthy body', () => {
    expect(evaluateProvenance(500, provenanceBody()).available).toBe(false);
    expect(evaluateProvenance(404, provenanceBody()).available).toBe(false);
  });

  test('malformed payloads degrade to unavailable', () => {
    for (const body of [
      { geography: {} },
      {},
      null,
      undefined,
      'nope',
      { geography: { provenance: { provider: 'x' } } },
    ]) {
      expect(unavailable(evaluateProvenance(200, body)).reason).toMatch(/no disponible/i);
    }
  });

  test('reports the risk input as synthetic only when the service declares it', () => {
    const synthetic = available(evaluateProvenance(200, provenanceBody()));
    expect(synthetic.syntheticInput).toBe(true);
    expect(synthetic.riskInputLabel).toMatch(/no es riesgo real observado/i);

    const body = provenanceBody();
    body.risk_input = { id: 'x' };
    const undeclared = available(evaluateProvenance(200, body));
    expect(undeclared.syntheticInput).toBe(false);
  });
});
