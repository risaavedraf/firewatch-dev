import { useEffect, useState } from 'react';

import { idleCopy, rejectedSessionCopy } from '../app/sessionCopy';
import { useSessionState } from '../app/store';
import { PanelHeading, UnimplementedNote } from '../components/ui';
import { timing } from '../config';
import { jobsStatus, riskStatus, sources, type Failure } from '../core/api';
import {
  evaluatePipeline,
  evaluateProvenance,
  evaluateServiceStatus,
  type PipelineDecision,
  type ProvenanceDecision,
  type ServiceStatusDecision,
} from '../domain';

// N8b — the sources + services view. It carries the scope declaration (S15), the
// provenance fields reproduced verbatim, the restriction text untouched, an
// integrity line that only claims what the service verified, and the pipeline
// status: a service is verified ONLY when its own snapshot earns it, a 403 reads
// as "not authorized to check" and never as "down", and the gateway readiness
// card is explicitly excluded from pipeline proof.

type Phase = 'unconsulted' | 'loading' | 'settled';

interface SourcesSnapshot {
  provenance: ProvenanceDecision | null;
  provenanceFailure: Failure | null;
  risk: ServiceStatusDecision | null;
  jobs: ServiceStatusDecision | null;
  pipeline: PipelineDecision | null;
}

function provenanceFailureMessage(failure: Failure): string {
  switch (failure.kind) {
    case 'unauthorized':
      return rejectedSessionCopy();
    case 'forbidden':
      return 'La API denegó el permiso para consultar la procedencia (403). Solicita acceso a un administrador.';
    case 'malformed':
      return 'La API devolvió una procedencia ilegible; no se muestra como fuente saludable.';
    default: {
      const code =
        failure.status !== undefined && failure.reason !== `http_${failure.status}` ? ` · ${failure.reason}` : '';
      const http = failure.status !== undefined ? `HTTP ${failure.status}${code}` : 'sin conexión';
      return `Procedencia geográfica no disponible (${http}); la integridad no se acredita y no se muestra como fuente saludable.`;
    }
  }
}

function provenanceStatusText(phase: Phase, decision: ProvenanceDecision | null, failure: Failure | null, unconsulted: string): string {
  if (phase === 'unconsulted') return unconsulted;
  if (phase === 'loading') return 'Consultando procedencia…';
  if (failure !== null) return provenanceFailureMessage(failure);
  if (decision !== null && decision.available) {
    return 'Procedencia geográfica publicada e integridad del hash verificada por el servicio. La restricción de la fuente se reproduce literal; no se acredita aprobación de DIFROL ni licencia adicional.';
  }
  return decision !== null ? decision.reason : 'Procedencia no disponible.';
}

export function SourcesView() {
  const session = useSessionState();
  const signedIn = session.status === 'signed-in';

  // The snapshot is ignored while there is no session, so a closed session shows
  // the explicit unconsulted state even though the last read is still in memory.
  const [snapshot, setSnapshot] = useState<SourcesSnapshot | null>(null);

  useEffect(() => {
    if (!signedIn) return;
    const controller = new AbortController();
    let active = true;
    let timer: ReturnType<typeof setTimeout> | null = null;

    const cycle = async (): Promise<void> => {
      const [sourcesOutcome, riskOutcome, jobsOutcome] = await Promise.all([
        sources(controller.signal),
        riskStatus(controller.signal),
        jobsStatus(controller.signal),
      ]);
      if (!active) return;

      const provenance = sourcesOutcome.ok ? evaluateProvenance(200, sourcesOutcome.data) : null;
      const provenanceFailure = sourcesOutcome.ok ? null : sourcesOutcome.failure;
      const now = Date.now();
      const risk = riskOutcome.ok
        ? evaluateServiceStatus({ status: 200, body: riskOutcome.data }, now)
        : evaluateServiceStatus({ status: riskOutcome.failure.status ?? 0, body: null }, now);
      const jobs = jobsOutcome.ok
        ? evaluateServiceStatus({ status: 200, body: jobsOutcome.data }, now)
        : evaluateServiceStatus({ status: jobsOutcome.failure.status ?? 0, body: null }, now);
      setSnapshot({ provenance, provenanceFailure, risk, jobs, pipeline: evaluatePipeline(risk, jobs) });

      // Recursive setTimeout only: each cycle replaces one bounded, cancelable
      // timer while the session lives, exactly as the retired page did.
      timer = setTimeout(() => {
        void cycle();
      }, timing.pipelineRefreshMs);
    };

    void cycle();

    return () => {
      active = false;
      controller.abort();
      if (timer !== null) clearTimeout(timer);
    };
  }, [signedIn]);

  const phase: Phase = !signedIn ? 'unconsulted' : snapshot === null ? 'loading' : 'settled';
  const current = signedIn ? snapshot : null;
  const provenance = current?.provenance ?? null;
  const provenanceFailure = current?.provenanceFailure ?? null;
  const riskDecision = current?.risk ?? null;
  const jobsDecision = current?.jobs ?? null;
  const pipeline = current?.pipeline ?? null;

  const accepted = provenance !== null && provenance.available ? provenance.provenance : null;
  // A field that was never read states "Sin consultar"; a field whose read was
  // cleared (logout) states "No disponible", exactly as the retired page did.
  const field = (value: string | number | undefined): string => {
    if (value !== undefined && value !== '') return String(value);
    return snapshot === null ? 'Sin consultar' : 'No disponible';
  };

  const badge =
    phase === 'unconsulted'
      ? 'SIN CONSULTAR'
      : phase === 'loading'
        ? 'CONSULTANDO'
        : accepted !== null
          ? 'PUBLICADA'
          : 'NO DISPONIBLE';
  const statusClass =
    accepted !== null ? 'status-copy risk-good' : phase === 'settled' ? 'status-copy risk-error' : 'status-copy';

  const pipelineStatus =
    phase === 'unconsulted'
      ? idleCopy(session, 'comprobar los servicios')
      : phase === 'loading'
        ? 'Consultando servicios…'
        : pipeline !== null
          ? pipeline.status
          : 'Servicios dependientes no verificados.';
  const pipelineClass = pipeline?.verified ? 'status-copy risk-good' : 'status-copy';

  return (
    <section className="view" id="sources" data-view="sources" aria-labelledby="sources-title">
      <p className="eyebrow">PROCEDENCIA Y SERVICIOS</p>
      <h2 id="sources-title">Fuentes y servicios</h2>
      <div className="warning-banner" role="note" aria-label="Alcance de los datos de esta página">
        <span className="warning-icon" aria-hidden="true">
          !
        </span>
        <p>
          <strong>Entrada de demostración: todavía no hay ingestión de datos reales.</strong> La API clasifica un lote
          sintético con una regla académica propia, declarada como no oficial y no calibrada contra incendios
          observados. Esta página no predice fecha ni lugar, no emite alertas y no sustituye a CONAF ni a SENAPRED.
          Mientras esa afirmación siga siendo cierta, se mantiene visible aquí y en el aviso de alcance de la barra
          superior.
        </p>
      </div>

      <article className="panel sources-panel">
        <PanelHeading
          eyebrow="PROCEDENCIA GEOGRÁFICA"
          title="Fuentes y procedencia"
          badge={
            <span className="neutral-tag" id="sources-badge">
              {badge}
            </span>
          }
        />
        <p id="sources-status" className={statusClass} role="status" aria-live="polite">
          {provenanceStatusText(phase, provenance, provenanceFailure, idleCopy(session, 'consultar la procedencia'))}
        </p>
        <dl className="data-list">
          <div>
            <dt>Proveedor</dt>
            <dd id="sources-provider">{accepted ? accepted.provider : field(undefined)}</dd>
          </div>
          <div>
            <dt>Atribución</dt>
            <dd id="sources-attribution">{accepted ? accepted.attribution : field(undefined)}</dd>
          </div>
          <div>
            <dt>Restricción</dt>
            <dd id="sources-restriction">{accepted ? accepted.restriction : field(undefined)}</dd>
          </div>
          <div>
            <dt>Registros de la capa</dt>
            <dd id="sources-record-count">{accepted ? accepted.recordCount : field(undefined)}</dd>
          </div>
          <div>
            <dt>Hash derivado (sha256)</dt>
            <dd id="sources-derived-sha256">{accepted ? accepted.derivedSha256 : field(undefined)}</dd>
          </div>
          <div>
            <dt>Integridad declarada</dt>
            <dd id="sources-integrity">{accepted ? accepted.integrityLabel : field(undefined)}</dd>
          </div>
          <div>
            <dt>Corte de fuente</dt>
            <dd id="sources-cutoff">{accepted ? accepted.cutoffLabel : field(undefined)}</dd>
          </div>
          <div>
            <dt>Entrada de riesgo</dt>
            <dd id="sources-input">{accepted ? accepted.riskInputLabel : field(undefined)}</dd>
          </div>
        </dl>
        <UnimplementedNote>
          Acciones de revisión e incorporación de registros, y hallazgos de calidad: <code>/v1/risk/sources</code>{' '}
          publica la procedencia y la integridad reales, no un catálogo de acciones ni de hallazgos.
        </UnimplementedNote>
      </article>

      <article className="panel pipeline-panel" id="pipeline" aria-labelledby="pipeline-title">
        <PanelHeading
          eyebrow="ESTADO COMPROBABLE DEL PIPELINE"
          titleId="pipeline-title"
          title="Servicios dependientes"
          badge={
            <span className="neutral-tag" id="pipeline-badge">
              {pipeline !== null ? pipeline.badge : 'SIN VERIFICAR'}
            </span>
          }
        />
        <p id="pipeline-status" className={pipelineClass} role="status" aria-live="polite">
          {pipelineStatus}
        </p>
        <dl className="data-list compact">
          <div>
            <dt>Servicio de riesgo</dt>
            <dd id="pipeline-risk">{riskDecision !== null ? riskDecision.label : 'Sin verificar'}</dd>
          </div>
          <div>
            <dt>Servicio de trabajos</dt>
            <dd id="pipeline-jobs">{jobsDecision !== null ? jobsDecision.label : 'Sin verificar'}</dd>
          </div>
          <div>
            <dt>Gateway</dt>
            <dd id="pipeline-gateway">
              La tarjeta "Gateway" solo informa disponibilidad del gateway; un 204 no acredita el pipeline.
            </dd>
          </div>
        </dl>
      </article>
    </section>
  );
}
