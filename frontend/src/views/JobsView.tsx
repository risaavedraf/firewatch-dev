import { useCallback, useEffect, useRef, useState } from 'react';

import { idleCopy } from '../app/sessionCopy';
import { getSelectedZone, subscribeZoneState, useSelectedZone, useSessionState } from '../app/store';
import { StatusCopy, UnimplementedNote } from '../components/ui';
import { timing } from '../config';
import { createJob, history, job as readJob, latest, scenarios } from '../core/api';
import type { Failure } from '../core/api';
import { getAccessToken, getSessionSnapshot, subscribeSession } from '../core/session';
import { expectedBatchInHistory, matchesBatch, presentBatch } from '../domain';
import type { Job, Scenario } from '../types/api';

// The processing view: request one synthetic batch for the selected zone, then
// read the job back with a bounded poll. Three rules travel with this code.
//
//   1. The request is administrative. The server decides whether the token may
//      create a job, so the view states the admin-only nature and renders a 403
//      as a permission denial instead of a missing service.
//   2. A job is only "published" for this view when its own batch is present and
//      valid in BOTH the latest read and the history, correlated by zone, job and
//      batch. A slow answer for another zone or a superseded request is discarded
//      whole, never rendered as the current job.
//   3. Polling is bounded twice: at most `maxPollAttempts` reads at
//      `pollIntervalMs`, and never past `pollDeadlineMs`. An expiry is an
//      UNCONFIRMED completion — never a published batch and never low risk. A
//      manual refresh is always offered so the operator can retry by hand.
//
// The synthetic-scenario selector survives (the user kept it by name): the
// served catalogue comes from `/v1/risk/synthetic-scenarios`, the chosen id
// travels only when one was chosen, and a refused or failed catalogue is NOT
// replaced by an invented one — the selector disables and the note says why.
//
// State ownership: the zone and the session are external stores, so this view
// *subscribes* to them and resets in the callback. The job snapshot is nulled
// exactly when the zone or the session changes, which is what makes a late
// answer from another zone impossible to display.

const NO_JOB_STATUS = 'Sin trabajo para la zona seleccionada.';
const DEFAULT_SCENARIO_NOTE =
  'El escenario se elige en esta página. El servidor asigna su instante de evaluación en UTC, una sola vez por trabajo; son perfiles de prueba, no condiciones observadas.';
const CATALOGUE_REJECTED =
  'La API denegó el permiso para listar escenarios (403); el trabajo usaría el fixture analítico heredado.';
const CATALOGUE_INVALID =
  'La API no devolvió un catálogo de escenarios válido; el trabajo usaría el fixture analítico heredado.';
const CATALOGUE_FAILED = 'No fue posible cargar el catálogo de escenarios; el trabajo usaría el fixture analítico heredado.';
const PERMISSION_DENIED = 'La API denegó el permiso para esta acción (403). Solicita acceso a un administrador.';
const OTHER_JOB = 'La API devolvió otro trabajo o zona; se descartó.';
const POLL_EXHAUSTED = 'Se agotó la consulta automática; publicación o lectura sin verificar. Puedes actualizar manualmente.';
const DEADLINE_EXPIRED =
  'Tiempo de consulta automática agotado; publicación o lectura sin verificar. Puedes actualizar manualmente.';

interface JobSnapshot {
  status: string;
  state: string;
  id: string;
  requested: string;
  // True once a job has actually been read back from the API, which is when the
  // manual refresh becomes meaningful.
  known: boolean;
}

function scenarioFallback(failure: Failure): string {
  if (failure.kind === 'forbidden') return CATALOGUE_REJECTED;
  if (failure.reason === 'invalid_shape') return CATALOGUE_INVALID;
  return CATALOGUE_FAILED;
}

// `null` means "leave the panel as it is": an unauthorized answer already reset
// the session, and the session callback owns the copy in that case. The reason
// branch that copy needs now lives in `sessionCopy` (S2).
function failureCopy(failure: Failure, action: string): string | null {
  if (failure.kind === 'unauthorized') return null;
  if (failure.kind === 'forbidden') return PERMISSION_DENIED;
  if (failure.reason === 'zone_mismatch' || failure.reason === 'job_mismatch') return OTHER_JOB;
  const status = failure.status !== undefined && failure.status > 0 ? String(failure.status) : 'sin conexión';
  return `No se pudo ${action} (HTTP ${status}).`;
}

function scenarioLabel(scenario: Scenario): string {
  return scenario.description === '' ? scenario.id : `${scenario.id} — ${scenario.description}`;
}

export function JobsView() {
  const selectedZone = useSelectedZone();
  const session = useSessionState();
  const signedIn = session.status === 'signed-in';

  const [catalogue, setCatalogue] = useState<Scenario[] | null>(null);
  const [scenarioNote, setScenarioNote] = useState(DEFAULT_SCENARIO_NOTE);
  const [scenarioId, setScenarioId] = useState('');
  const [requesting, setRequesting] = useState(false);
  const [job, setJob] = useState<JobSnapshot | null>(null);

  const mountedRef = useRef(true);
  const zoneRef = useRef(selectedZone);
  const signedInRef = useRef(signedIn);
  const lastZoneRef = useRef(selectedZone);
  const lastSessionStatusRef = useRef(session.status);
  const sequenceRef = useRef(0);
  const controllerRef = useRef<AbortController | null>(null);
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const deadlineRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const attemptsRef = useRef(0);
  const currentJobIdRef = useRef('');

  // The async callbacks read the zone and the session at the moment an answer
  // lands, not when the request started, so they are mirrored into refs after
  // every commit. The refs are read inside callbacks and effects only.
  useEffect(() => {
    zoneRef.current = selectedZone;
    signedInRef.current = signedIn;
  });

  // One job cycle's external teardown: fence its answers and clear both timers.
  // Kept out of the `setState` path so it is safe to call from any transition.
  const fenceJob = useCallback((abort: boolean): void => {
    sequenceRef.current += 1;
    if (abort && controllerRef.current) controllerRef.current.abort();
    controllerRef.current = null;
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (deadlineRef.current !== null) {
      clearTimeout(deadlineRef.current);
      deadlineRef.current = null;
    }
    attemptsRef.current = 0;
    currentJobIdRef.current = '';
  }, []);

  // Unmount is a hard stop: no poll may outlive the view.
  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      fenceJob(true);
    };
  }, [fenceJob]);

  // A zone change ends the job that belonged to the previous zone. The sequence
  // is bumped so a late answer is discarded rather than applied to the new zone;
  // the in-flight read is intentionally NOT aborted, so the fence — not a
  // transport error — is what discards it.
  useEffect(
    () =>
      subscribeZoneState(() => {
        const zone = getSelectedZone();
        if (zone === lastZoneRef.current) return;
        lastZoneRef.current = zone;
        fenceJob(false);
        setJob(null);
      }),
    [fenceJob],
  );

  // Logout closes the session: abort the in-flight read, clear the job and the
  // catalogue. The scenario picker returns to its legacy-only disabled state.
  useEffect(
    () =>
      subscribeSession(() => {
        const status = getSessionSnapshot().status;
        if (status === lastSessionStatusRef.current) return;
        lastSessionStatusRef.current = status;
        if (status === 'signed-in') return;
        fenceJob(true);
        setJob(null);
        setCatalogue(null);
        setScenarioId('');
        setScenarioNote(DEFAULT_SCENARIO_NOTE);
        setRequesting(false);
      }),
    [fenceJob],
  );

  // The catalogue is tied to the session: fetched once per connection. A
  // failure never becomes an invented profile.
  useEffect(() => {
    if (!signedIn) return;
    const controller = new AbortController();
    let current = true;
    void (async () => {
      const outcome = await scenarios(controller.signal);
      if (!current) return;
      if (outcome.ok) {
        setCatalogue(outcome.data);
        setScenarioNote(DEFAULT_SCENARIO_NOTE);
      } else {
        setCatalogue(null);
        setScenarioNote(scenarioFallback(outcome.failure));
      }
    })();
    return () => {
      current = false;
      controller.abort();
    };
  }, [signedIn]);

  function ownsJob(sequence: number, zone: string): boolean {
    return (
      mountedRef.current &&
      sequence === sequenceRef.current &&
      zone === zoneRef.current &&
      signedInRef.current &&
      getAccessToken() !== ''
    );
  }

  function patchJob(patch: Partial<JobSnapshot>): void {
    setJob(previous => (previous ? { ...previous, ...patch } : previous));
  }

  function stopDeadline(): void {
    if (deadlineRef.current !== null) {
      clearTimeout(deadlineRef.current);
      deadlineRef.current = null;
    }
  }

  // The single hard deadline for one job cycle. It fires once, aborts whatever is
  // in flight and states an unconfirmed completion: the page cannot know whether
  // the server published and it must not guess.
  function armDeadline(sequence: number, zone: string): void {
    stopDeadline();
    deadlineRef.current = setTimeout(() => {
      deadlineRef.current = null;
      if (!ownsJob(sequence, zone)) return;
      sequenceRef.current += 1;
      if (controllerRef.current) controllerRef.current.abort();
      controllerRef.current = null;
      if (timerRef.current !== null) {
        clearTimeout(timerRef.current);
        timerRef.current = null;
      }
      attemptsRef.current = 0;
      patchJob({ status: DEADLINE_EXPIRED });
    }, timing.pollDeadlineMs);
  }

  function applyFailure(failure: Failure, action: string): void {
    const copy = failureCopy(failure, action);
    if (copy !== null) patchJob({ status: copy });
  }

  function renderJob(fetched: Job): void {
    currentJobIdRef.current = fetched.id;
    const scenario = typeof fetched.scenario_id === 'string' && fetched.scenario_id !== '' ? ` · escenario ${fetched.scenario_id}` : '';
    patchJob({
      status:
        fetched.state === 'published' && typeof fetched.batch_id === 'string' && fetched.batch_id !== ''
          ? `Trabajo publicado · lote ${fetched.batch_id}${scenario}`
          : `Trabajo ${fetched.state}${scenario}; el servidor controla su procesamiento.`,
      state: fetched.state,
      id: fetched.id,
      requested:
        typeof fetched.requested_at === 'string' && fetched.requested_at !== '' ? fetched.requested_at : 'No informado',
      known: true,
    });
  }

  // One poll read. The same controller and attempt counter survive across the
  // cycle so the deadline and the attempt cap fence the whole job, not one call.
  async function loadJob(): Promise<void> {
    const id = currentJobIdRef.current;
    if (id === '') return;
    if (timerRef.current !== null) {
      clearTimeout(timerRef.current);
      timerRef.current = null;
    }
    if (controllerRef.current === null) {
      controllerRef.current = new AbortController();
      attemptsRef.current = 0;
    }
    const controller = controllerRef.current;
    const sequence = sequenceRef.current;
    const zone = zoneRef.current;
    if (deadlineRef.current === null) armDeadline(sequence, zone);
    const attempt = (attemptsRef.current += 1);
    patchJob({ status: 'Consultando estado del trabajo…' });
    const outcome = await readJob(id, controller.signal);
    if (!ownsJob(sequence, zone) || attempt !== attemptsRef.current) return;
    if (!outcome.ok) {
      stopDeadline();
      applyFailure(outcome.failure, 'consultar el trabajo');
      return;
    }
    const fetched = outcome.data;
    if (fetched.id !== id || fetched.zone_id !== zone) {
      stopDeadline();
      patchJob({ status: OTHER_JOB });
      return;
    }
    await followJob(fetched, sequence, zone);
  }

  // Read the published batch back. Completion is claimed only when the batch
  // correlates with the job in BOTH reads.
  async function followJob(fetched: Job, sequence: number, zone: string): Promise<void> {
    const attempt = attemptsRef.current;
    renderJob(fetched);
    if (fetched.state === 'published' && typeof fetched.batch_id === 'string' && fetched.batch_id !== '') {
      const expected = { zone, job: fetched.id, batch: fetched.batch_id };
      const controller = controllerRef.current;
      const signal = controller ? controller.signal : undefined;
      const [latestOutcome, historyOutcome] = await Promise.all([latest(zone, signal), history(zone, signal)]);
      if (!ownsJob(sequence, zone) || attempt !== attemptsRef.current) return;
      const confirmed =
        latestOutcome.ok &&
        matchesBatch(latestOutcome.data, expected) &&
        presentBatch(latestOutcome.data).valid &&
        historyOutcome.ok &&
        expectedBatchInHistory(historyOutcome.data, expected);
      if (confirmed) {
        stopDeadline();
        patchJob({ status: `Trabajo publicado · lote ${fetched.batch_id} verificado en último lote e historial.` });
        return;
      }
    }
    if (attemptsRef.current >= timing.maxPollAttempts) {
      stopDeadline();
      patchJob({ status: POLL_EXHAUSTED });
      return;
    }
    timerRef.current = setTimeout(() => {
      timerRef.current = null;
      void loadJob();
    }, timing.pollIntervalMs);
  }

  function requestBatch(): void {
    if (!signedInRef.current || zoneRef.current === '') return;
    // Both the zone and the scenario are captured at click time: a later change
    // in either control must not alter a request that is already in flight.
    const zone = zoneRef.current;
    const scenario = scenarioId;
    fenceJob(true);
    const sequence = sequenceRef.current;
    setJob({ status: 'Solicitando trabajo a la API…', state: 'Sin solicitud', id: '—', requested: '—', known: false });
    const controller = new AbortController();
    controllerRef.current = controller;
    armDeadline(sequence, zone);
    setRequesting(true);
    void (async () => {
      try {
        const outcome = await createJob(
          {
            zoneId: zone,
            // The server rejects an explicit empty scenario_id with 400 while an
            // omitted field keeps the legacy fixture, so the field travels only
            // when one was actually chosen.
            scenarioId: scenario === '' ? undefined : scenario,
            idempotencyKey: crypto.randomUUID(),
          },
          controller.signal,
        );
        if (!ownsJob(sequence, zone)) return;
        if (!outcome.ok) {
          stopDeadline();
          applyFailure(outcome.failure, 'solicitar el trabajo');
          return;
        }
        const created = outcome.data;
        if (created.zone_id !== zone || created.id === '') {
          stopDeadline();
          patchJob({ status: OTHER_JOB });
          return;
        }
        if (created.state === 'published') await followJob(created, sequence, zone);
        else {
          renderJob(created);
          void loadJob();
        }
      } finally {
        if (mountedRef.current) setRequesting(false);
      }
    })();
  }

  const status = job
    ? job.status
    : signedIn
      ? NO_JOB_STATUS
      : idleCopy(session, 'solicitar el lote de la zona');
  const selectorDisabled = !signedIn || catalogue === null;
  const createDisabled = !signedIn || selectedZone === '' || requesting;

  return (
    <section className="view" id="jobs" data-view="jobs" aria-labelledby="jobs-title">
      <article className="panel jobs-panel">
        <div className="panel-heading">
          <div>
            <p className="eyebrow">LOTE DE DEMOSTRACIÓN</p>
            <h2 id="jobs-title">Procesamiento asíncrono</h2>
          </div>
          <span className="admin-tag">ACCIÓN ADMINISTRATIVA</span>
        </div>
        <div className="job-layout">
          <div>
            <div className="scenario-field">
              <label htmlFor="scenario-select">Escenario sintético</label>
              <select
                id="scenario-select"
                aria-label="Escenario sintético a solicitar"
                value={scenarioId}
                disabled={selectorDisabled}
                onChange={event => setScenarioId(event.target.value)}
              >
                <option value="">Sin escenario (fixture analítico heredado)</option>
                {(catalogue ?? []).map(scenario => (
                  <option key={scenario.id} value={scenario.id}>
                    {scenarioLabel(scenario)}
                  </option>
                ))}
              </select>
              <p className="panel-note" id="scenario-note">
                {scenarioNote}
              </p>
            </div>
            <p className="panel-note">
              Solicita el procesamiento de la zona seleccionada. Esta operación requiere permisos de administrador y crea
              únicamente un trabajo sintético.
            </p>
            <button id="create-job" className="primary-button" type="button" disabled={createDisabled} onClick={requestBatch}>
              Solicitar lote de la zona
            </button>
          </div>
          <div className="job-result">
            <p className="eyebrow">ESTADO DEL TRABAJO</p>
            <StatusCopy id="job-status">{status}</StatusCopy>
            <dl className="data-list compact">
              <div>
                <dt>Estado</dt>
                <dd id="job-state">{job ? job.state : 'Sin solicitud'}</dd>
              </div>
              <div>
                <dt>ID del trabajo</dt>
                <dd id="job-id">{job ? job.id : '—'}</dd>
              </div>
              <div>
                <dt>Solicitado</dt>
                <dd id="job-requested">{job ? job.requested : '—'}</dd>
              </div>
            </dl>
            <button
              id="refresh-job"
              className="secondary-button"
              type="button"
              disabled={!(job !== null && job.known)}
              onClick={() => {
                void loadJob();
              }}
            >
              Actualizar estado
            </button>
          </div>
        </div>
        <UnimplementedNote>
          Publicar lote, el avance manual del estado y la reevaluación: no existen como endpoint; el servidor decide y
          publica por su cuenta, y esta página solo solicita el trabajo y lee el resultado.
        </UnimplementedNote>
      </article>
    </section>
  );
}
