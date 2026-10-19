import { useCallback, useEffect, useState } from 'react';

import { idleCopy } from '../app/sessionCopy';
import { PanelHeading } from '../components/ui';
import { useSessionState } from '../app/store';
import { audit, type Outcome } from '../core/api';
import type { SessionState } from '../core/session';
import type { AuditEntry } from '../types/api';

// N8b — the activity register (S13/S17). `/v1/audit` is admin-only and accepts no
// parameter: the service bounds the answer itself, so the panel sends no limit or
// offset and never pages. Every entry is rendered with the five fields the
// endpoint carries — `actor_id`, `action`, `job_id`, `trace_id`, `occurred_at` —
// and the retired mockup's columns no backend provides (a display name, a
// free-text detail and its own event id) are declared as not implemented instead
// of being simulated.
//
// The view is reachable for every role on purpose: the server decides, so a
// `viewer` must be able to arrive here and read its own explicit 403. There is no
// client-side role gating anywhere.

const AUDIT_FIELDS = ['actor_id', 'action', 'job_id', 'trace_id', 'occurred_at'] as const;

type AuditPhase = 'idle' | 'loading' | 'read' | 'denied' | 'unavailable';

interface RegisterState {
  phase: AuditPhase;
  badge: string;
  message: string;
  entries: readonly AuditEntry[];
  showTable: boolean;
}

// The view's own action phrase; the reason branch lives in `sessionCopy`.
const AUDIT_IDLE_ACTION = 'consultar el registro de actividad';

const IDLE_STATE: RegisterState = {
  phase: 'idle',
  badge: 'SIN CONSULTAR',
  message: idleCopy({ reason: null }, AUDIT_IDLE_ACTION),
  entries: [],
  showTable: false,
};

const LOADING_STATE: RegisterState = {
  phase: 'loading',
  badge: 'CONSULTANDO',
  message: 'Consultando el registro de actividad en /v1/audit…',
  entries: [],
  showTable: false,
};

function nonempty(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== '';
}

// The transport already fails closed: an entry that does not state WHAT happened
// and WHEN never reaches this function. What is left is presentation, and every
// branch below states an outcome instead of hiding one.
function registerFromOutcome(outcome: Outcome<AuditEntry[]>): RegisterState {
  if (outcome.ok) {
    const entries = outcome.data;
    if (entries.length === 0) {
      return { phase: 'read', badge: 'LEÍDO', message: 'El servicio devolvió un registro de actividad sin eventos.', entries, showTable: false };
    }
    return {
      phase: 'read',
      badge: 'LEÍDO',
      message:
        'Registro real devuelto por /v1/audit. Es un endpoint de solo administradores y no acepta parámetros: el servicio acota la respuesta por sí mismo.',
      entries,
      showTable: true,
    };
  }
  const { failure } = outcome;
  // A denied permission is a stated outcome, never a hidden panel: the 403 body
  // itself names the reason and the request.
  if (failure.kind === 'forbidden') {
    const code = nonempty(failure.reason) ? ` · ${failure.reason}` : '';
    const request = failure.requestId !== undefined ? ` · request_id ${failure.requestId}` : '';
    return {
      phase: 'denied',
      badge: 'SIN PERMISO',
      message: `La API denegó el permiso para consultar el registro de actividad (HTTP 403${code}${request}). Es un endpoint de solo administradores: solicita acceso a un administrador; la página no oculta ni simula los eventos.`,
      entries: [],
      showTable: false,
    };
  }
  if (failure.kind === 'malformed') {
    const code = nonempty(failure.reason) ? ` · ${failure.reason}` : '';
    return {
      phase: 'unavailable',
      badge: 'NO DISPONIBLE',
      message: `La API no devolvió un registro de actividad utilizable (HTTP ${failure.status ?? 200}${code}); no se muestran eventos inventados ni parciales.`,
      entries: [],
      showTable: false,
    };
  }
  return {
    phase: 'unavailable',
    badge: 'NO DISPONIBLE',
    message: 'No fue posible consultar el registro de actividad; se muestra como no disponible.',
    entries: [],
    showTable: false,
  };
}

function AuditRegister({ session }: { session: SessionState }) {
  const signedIn = session.status === 'signed-in';
  const [state, setState] = useState<RegisterState>(signedIn ? LOADING_STATE : IDLE_STATE);
  const [revision, setRevision] = useState(0);

  useEffect(() => {
    if (!signedIn) return;
    let cancelled = false;
    void (async () => {
      const outcome = await audit();
      // A reply that lands after the session closed must not repaint the panel:
      // the cleanup marks it fenced and it owns nothing.
      if (cancelled) return;
      setState(registerFromOutcome(outcome));
    })();
    return () => {
      cancelled = true;
    };
  }, [signedIn, revision]);

  const refresh = useCallback((): void => {
    setState(LOADING_STATE);
    setRevision(value => value + 1);
  }, []);

  const count = state.phase === 'read' ? String(state.entries.length) : '—';
  const disabled = !signedIn || state.phase === 'loading';
  // The idle sentence is recomputed from the session, so a reason that changes
  // while no register is open is named instead of showing a stale first-visit copy.
  const message = state.phase === 'idle' ? idleCopy(session, AUDIT_IDLE_ACTION) : state.message;

  return (
    <section className="view" id="audit" data-view="audit" aria-labelledby="audit-title">
      <article className="panel audit-panel">
        <PanelHeading
          eyebrow="COORDINACIÓN"
          title="Registro de actividad"
          titleId="audit-title"
          badge={
            <span className="neutral-tag" id="audit-badge">
              {state.badge}
            </span>
          }
        />
        <p
          id="audit-status"
          className={state.phase === 'read' ? 'status-copy risk-good' : 'status-copy'}
          role="status"
          aria-live="polite"
        >
          {message}
        </p>
        <p className="panel-note">
          Endpoint de solo administradores. No acepta parámetros: el servicio acota la respuesta por sí mismo (hasta 100
          eventos recientes) y esta página no pagina, no filtra ni ordena. Los identificadores son opacos, sin nombre para
          mostrar.
        </p>
        <p id="audit-unimplemented" className="unimplemented-note">
          Aún no implementado: el mockup retirado mostraba el nombre del perfil, un detalle de texto libre y su propio
          identificador de evento. <code>/v1/audit</code> no entrega ninguno de esos datos, así que no se muestran ni se
          simulan.
        </p>
        <div className="table-wrap" id="audit-table" hidden={!state.showTable}>
          <table>
            <caption className="sr-only">Registro de actividad devuelto por /v1/audit</caption>
            <thead>
              <tr>
                <th scope="col">Actor (actor_id)</th>
                <th scope="col">Acción</th>
                <th scope="col">Trabajo (job_id)</th>
                <th scope="col">Traza (trace_id)</th>
                <th scope="col">Ocurrido (occurred_at)</th>
              </tr>
            </thead>
            <tbody id="audit-rows">
              {state.entries.map((entry, index) => (
                // `/v1/audit` carries opaque identifiers, and the service, not the
                // page, decides what is unique; the row position is the only stable
                // key the page can honestly use.
                <tr key={index}>
                  {AUDIT_FIELDS.map(field => (
                    // An identifier the service omits is labelled in place: nothing
                    // is filled in from another field and no name is derived from an id.
                    <td key={field}>{nonempty(entry[field]) ? entry[field] : 'No informado'}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
        <div className="audit-foot">
          <span className="count-badge" id="audit-count">
            {count}
          </span>
          <button id="audit-refresh" className="secondary-button" type="button" disabled={disabled} onClick={refresh}>
            Actualizar registro
          </button>
        </div>
      </article>
    </section>
  );
}

export function AuditView() {
  const session = useSessionState();
  const signedIn = session.status === 'signed-in';
  // The register is keyed to the session phase so a closed session remounts it
  // empty instead of painting the previous session's rows for a frame.
  return <AuditRegister key={signedIn ? 'open' : 'closed'} session={session} />;
}
