import { useEffect, useState, type FormEvent } from 'react';

import { idleCopy } from '../app/sessionCopy';
import { useSessionState } from '../app/store';
import { PanelHeading, StatusCopy } from '../components/ui';
import {
  createPreventiveAction,
  preventiveAction,
  preventiveActions,
  transitionPreventiveAction,
  type Failure,
  type Outcome,
} from '../core/api';
import { getSessionSnapshot, subscribeSession } from '../core/session';
import { actionStatusLabel, declaresAdminRole, transitionAffordances } from '../domain';
import type { PreventiveAction, PreventiveActionDetail } from '../types/api';

// S3 / T12 — the preventive-action view, built against the frozen contract in
// `odd/tasks/presentation-client-ready.md` §«Contrato congelado de prevention».
//
// Four rules travel with this code:
//   1. the LIST, the DETAIL with its history, the CREATION and the TRANSITIONS
//      are the four routes of the contract, and nothing is simulated: a refusal
//      (401/403/409/422) is rendered with the code and the `request_id` the
//      server sent;
//   2. the transitions offered are the valid successors of the current status
//      (`domain/prevention.ts`). The map is a transcription, so a server
//      `409 invalid_transition` is still rendered when the two disagree;
//   3. the role that enables them is the role the UNVERIFIED token declares. It
//      is an affordance, never an authorization: the server remains the only
//      authority, and the reason is stated on screen;
//   4. a closed session — by logout or by a refused token — empties the private
//      view; a 401 keeps the `request_id` on screen because that is the evidence
//      the operator needs to report it.

type Phase = 'unconsulted' | 'loading' | 'read' | 'denied' | 'unavailable';

interface ListState {
  phase: Phase;
  badge: string;
  message: string;
  actions: readonly PreventiveAction[];
}

interface DetailState {
  phase: 'loading' | 'read' | 'refused' | 'unavailable';
  message: string;
  action: PreventiveActionDetail | null;
}

const IDLE_LIST: ListState = {
  phase: 'unconsulted',
  badge: 'SIN CONSULTAR',
  message: 'Conecta un token autorizado para consultar las acciones preventivas.',
  actions: [],
};

const LIST_ACTION = 'listar las acciones preventivas';
const CREATE_ACTION = 'crear la acción preventiva';
const DETAIL_ACTION = 'consultar el detalle de la acción';
const TRANSITION_ACTION = 'aplicar la transición';

interface FormState {
  communityCode: string;
  title: string;
  description: string;
  priority: string;
  assignee: string;
  dueDate: string;
}

const EMPTY_FORM: FormState = {
  communityCode: '',
  title: '',
  description: '',
  priority: '',
  assignee: '',
  dueDate: '',
};

function nonempty(value: unknown): value is string {
  return typeof value === 'string' && value.trim() !== '';
}

// One message builder for every refusal, so the list, the detail, the creation
// and the transition all name the HTTP status, the server `code` and the
// `request_id` when they have one. Nothing is swallowed into a generic failure.
function failureMessage(failure: Failure, action: string): string {
  const status = failure.status !== undefined ? `HTTP ${failure.status}` : 'sin conexión';
  const code = nonempty(failure.reason) && failure.reason !== `http_${failure.status}` ? ` · ${failure.reason}` : '';
  const request = failure.requestId !== undefined ? ` · request_id ${failure.requestId}` : '';
  const detail = `${status}${code}${request}`;
  // The frozen error vocabulary of the service, named explicitly so a refusal
  // never reads as a generic failure.
  switch (failure.reason) {
    case 'invalid_transition':
      return `La API rechazó la transición (${detail}). La acción no cambió de estado.`;
    case 'invalid_transition_request':
      return `La API rechazó la transición (${detail}): el estado destino no está declarado por el contrato. La acción no cambió de estado.`;
    case 'invalid_action_request':
      return `La API rechazó la acción (${detail}); revisa community_code y title, que son los únicos campos requeridos.`;
    case 'invalid_query':
      return `La API rechazó el filtro (${detail}); revisa los valores de comunidad y estado.`;
    case 'action_not_found':
      return `La acción consultada no existe (${detail}).`;
    case 'invalid_action_id':
      return `El identificador de la acción no es válido (${detail}).`;
    case 'storage_unavailable':
      return `El almacenamiento de prevención no está disponible (${detail}).`;
    case 'json_required':
      return `La API exige un cuerpo JSON (${detail}).`;
    default:
      break;
  }
  if (failure.kind === 'forbidden') {
    return `La API denegó el permiso para ${action} (${detail}). Solicita acceso a un administrador.`;
  }
  if (failure.kind === 'unauthorized') {
    return `La API rechazó el token (${detail}). La sesión se cerró; conecta nuevamente un token autorizado.`;
  }
  if (failure.status === 409) {
    return `La API rechazó la transición (${detail}). La acción no cambió de estado.`;
  }
  if (failure.status === 422) {
    return `La API rechazó la entrada (${detail}); revisa los campos requeridos.`;
  }
  if (failure.kind === 'missing') {
    return `La acción consultada no existe o no está disponible (${detail}).`;
  }
  if (failure.kind === 'malformed') {
    return `La API no devolvió una respuesta utilizable (${detail}); no se muestran valores parciales ni inventados.`;
  }
  return `No se pudo ${action} (${detail}).`;
}

function listFromOutcome(outcome: Outcome<PreventiveAction[]>): ListState {
  if (outcome.ok) {
    if (outcome.data.length === 0) {
      return {
        phase: 'read',
        badge: 'LEÍDO',
        message: 'El servicio no devolvió acciones preventivas para el filtro elegido.',
        actions: [],
      };
    }
    return {
      phase: 'read',
      badge: 'LEÍDO',
      message: `Acciones preventivas devueltas por /v1/prevention/actions (${outcome.data.length}).`,
      actions: outcome.data,
    };
  }
  const { failure } = outcome;
  if (failure.kind === 'forbidden') {
    return { phase: 'denied', badge: 'SIN PERMISO', message: failureMessage(failure, LIST_ACTION), actions: [] };
  }
  return { phase: 'unavailable', badge: 'NO DISPONIBLE', message: failureMessage(failure, LIST_ACTION), actions: [] };
}

function detailFromOutcome(outcome: Outcome<PreventiveActionDetail>): DetailState {
  if (outcome.ok) {
    return {
      phase: 'read',
      message: `Detalle e historial devueltos por /v1/prevention/actions/{id}; ${outcome.data.history.length} movimiento(s) de historial.`,
      action: outcome.data,
    };
  }
  const { failure } = outcome;
  return {
    phase: failure.kind === 'forbidden' ? 'refused' : 'unavailable',
    message: failureMessage(failure, DETAIL_ACTION),
    action: null,
  };
}

function field(value: string | null | undefined): string {
  return nonempty(value) ? value : 'No informado';
}

function historyMove(fromStatus: string | null | undefined, toStatus: string): string {
  return `${nonempty(fromStatus) ? fromStatus : '—'} → ${toStatus}`;
}

export function ActionsView() {
  const session = useSessionState();
  const signedIn = session.status === 'signed-in';
  // The role is a token FACT, read without verifying the signature. It only
  // decides which affordances this page shows; the server decides the action.
  const admin = signedIn && declaresAdminRole(session.roles);

  const [list, setList] = useState<ListState>(IDLE_LIST);
  const [selectedId, setSelectedId] = useState('');
  const [detail, setDetail] = useState<DetailState | null>(null);
  const [notice, setNotice] = useState('');
  const [form, setForm] = useState<FormState>(EMPTY_FORM);
  const [transitionNote, setTransitionNote] = useState('');
  const [communityFilter, setCommunityFilter] = useState('');
  const [statusFilter, setStatusFilter] = useState('');
  const [applied, setApplied] = useState<{ communityCode: string; status: string }>({ communityCode: '', status: '' });
  const [revision, setRevision] = useState(0);
  const [detailRevision, setDetailRevision] = useState(0);
  const [busy, setBusy] = useState(false);

  // The list, re-read on every session, filter and revision change. A refused
  // token closes the session from inside the transport, so the refusal is
  // captured even though this effect's cleanup has already fenced the answer:
  // the request id is the evidence the operator needs.
  useEffect(() => {
    if (!signedIn) return undefined;
    const controller = new AbortController();
    let active = true;
    void (async () => {
      const outcome = await preventiveActions(applied, controller.signal);
      if (!active) {
        if (!outcome.ok && outcome.failure.kind === 'unauthorized') {
          setNotice(failureMessage(outcome.failure, LIST_ACTION));
        }
        return;
      }
      setList(listFromOutcome(outcome));
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [signedIn, revision, applied]);

  // The detail is authored by the server (including its history), so it is
  // re-read after a transition instead of being patched locally.
  useEffect(() => {
    if (!signedIn || selectedId === '') return undefined;
    const controller = new AbortController();
    let active = true;
    void (async () => {
      const outcome = await preventiveAction(selectedId, controller.signal);
      if (!active) {
        if (!outcome.ok && outcome.failure.kind === 'unauthorized') {
          setNotice(failureMessage(outcome.failure, DETAIL_ACTION));
        }
        return;
      }
      setDetail(detailFromOutcome(outcome));
    })();
    return () => {
      active = false;
      controller.abort();
    };
  }, [signedIn, selectedId, detailRevision]);

  // Closing the session empties the private view. The reset runs from the session
  // subscription callback (not synchronously inside an effect body), the same
  // shape `JobsView` uses. A refused token keeps the notice — it carries the
  // request id — while an ordinary logout clears it.
  useEffect(
    () =>
      subscribeSession(() => {
        if (getSessionSnapshot().status === 'signed-in') return;
        const reason = getSessionSnapshot().reason;
        setList(IDLE_LIST);
        setSelectedId('');
        setDetail(null);
        setForm(EMPTY_FORM);
        setTransitionNote('');
        setCommunityFilter('');
        setStatusFilter('');
        setApplied({ communityCode: '', status: '' });
        setBusy(false);
        if (reason !== 'rejected') setNotice('');
      }),
    [],
  );

  function reload(): void {
    setRevision(value => value + 1);
  }

  function applyFilters(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    setApplied({ communityCode: communityFilter.trim(), status: statusFilter.trim() });
  }

  function openDetail(id: string): void {
    setNotice('');
    setTransitionNote('');
    setSelectedId(id);
  }

  function submitCreate(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    if (!signedIn || busy) return;
    setBusy(true);
    setNotice('');
    void (async () => {
      const outcome = await createPreventiveAction({
        communityCode: form.communityCode,
        title: form.title,
        description: form.description,
        priority: form.priority,
        assignee: form.assignee,
        dueDate: form.dueDate,
      });
      setBusy(false);
      if (!outcome.ok) {
        setNotice(failureMessage(outcome.failure, CREATE_ACTION));
        return;
      }
      setForm(EMPTY_FORM);
      setNotice(
        `Acción preventiva creada (${outcome.data.id}) en estado ${outcome.data.status}; la lista se vuelve a leer del servicio.`,
      );
      setSelectedId(outcome.data.id);
      reload();
    })();
  }

  function applyTransition(to: string): void {
    if (!signedIn || busy || selectedId === '') return;
    setBusy(true);
    setNotice('');
    void (async () => {
      const outcome = await transitionPreventiveAction(selectedId, { to, note: transitionNote });
      setBusy(false);
      if (!outcome.ok) {
        setNotice(failureMessage(outcome.failure, TRANSITION_ACTION));
        return;
      }
      setNotice(`Transición aplicada: la acción quedó en estado ${outcome.data.status}.`);
      setTransitionNote('');
      setDetailRevision(value => value + 1);
      reload();
    })();
  }

  // Only the detail of the action currently selected is shown: a late answer for
  // a previous selection is a mismatch, not the new action's detail.
  const selected = detail !== null && detail.action !== null && detail.action.id === selectedId ? detail.action : null;
  const detailMessage =
    selectedId === ''
      ? ''
      : selected !== null
        ? detail?.message ?? ''
        : 'Consultando el detalle y el historial…';
  const transitions = selected === null ? [] : transitionAffordances(selected.status, session.roles, signedIn);

  const roleNote = !signedIn
    ? 'Conecta una sesión para operar: la página no concede permisos.'
    : admin
      ? 'Tu token declara el rol admin, así que esta página ofrece el alta y las transiciones válidas de la máquina de estados. La firma del token no se verifica aquí: el servidor sigue siendo la única autoridad y su respuesta se muestra tal cual.'
      : session.roles === null
        ? 'Tu token no declara un rol utilizable, así que el alta y las transiciones quedan deshabilitadas. La página no supone ningún permiso: solicita un token con rol admin. El servidor es la única autoridad y una negativa se mostraría tal cual.'
        : 'Tu token no declara el rol admin, así que el alta y las transiciones quedan deshabilitadas en esta página. La página no concede permisos: solicita un token con rol admin. El servidor es la única autoridad y una negativa se mostraría tal cual.';

  return (
    <section className="view" id="actions" data-view="actions" aria-labelledby="actions-title">
      <PanelHeading
        eyebrow="PREVENCIÓN"
        title="Acciones preventivas"
        titleId="actions-title"
        badge={
          <span className="neutral-tag" id="actions-badge">
            {signedIn ? list.badge : 'SIN CONSULTAR'}
          </span>
        }
      />
      <p
        id="actions-status"
        className={list.phase === 'read' ? 'status-copy risk-good' : 'status-copy'}
        role="status"
        aria-live="polite"
      >
        {list.phase === 'unconsulted' ? idleCopy(session, LIST_ACTION) : list.message}
      </p>
      <p id="actions-role-note" className="panel-note">
        {roleNote}
      </p>
      <p id="actions-notice" className="status-copy" role="status" aria-live="polite">
        {notice}
      </p>
      <p className="panel-note">
        El servicio acota la lista a 200 filas y ambos filtros son opcionales: un filtro vacío no acota nada. La
        prioridad es texto libre validado por el servicio.
      </p>

      <form className="filter-bar actions-filters" onSubmit={applyFilters}>
        <label className="filter-label" htmlFor="actions-community-filter">
          Comunidad (CUT)
          <input
            id="actions-community-filter"
            type="text"
            value={communityFilter}
            disabled={!signedIn}
            onChange={event => setCommunityFilter(event.target.value)}
          />
        </label>
        <label className="filter-label" htmlFor="actions-status-filter">
          Estado
          <select
            id="actions-status-filter"
            value={statusFilter}
            disabled={!signedIn}
            onChange={event => setStatusFilter(event.target.value)}
          >
            <option value="">Todos</option>
            <option value="propuesta">Propuesta</option>
            <option value="asignada">Asignada</option>
            <option value="en_curso">En curso</option>
            <option value="bloqueada">Bloqueada</option>
            <option value="enviada">Enviada</option>
            <option value="aceptada">Aceptada</option>
            <option value="devuelta">Devuelta</option>
            <option value="cancelada">Cancelada</option>
          </select>
        </label>
        <button id="actions-apply-filters" className="secondary-button" type="submit" disabled={!signedIn}>
          Aplicar filtros
        </button>
        <button
          id="actions-refresh"
          className="secondary-button"
          type="button"
          disabled={!signedIn || list.phase === 'loading'}
          onClick={reload}
        >
          Actualizar lista
        </button>
      </form>

      <div className="table-wrap">
        <table id="actions-table">
          <caption className="sr-only">Acciones preventivas devueltas por el servicio</caption>
          <thead>
            <tr>
              <th scope="col">Acción</th>
              <th scope="col">Comunidad</th>
              <th scope="col">Estado</th>
              <th scope="col">Prioridad</th>
              <th scope="col">Responsable</th>
              <th scope="col">Fecha límite</th>
              <th scope="col">Detalle</th>
            </tr>
          </thead>
          <tbody id="actions-rows">
            {list.actions.map(action => (
              <tr key={action.id} className={action.id === selectedId ? 'is-selected' : undefined}>
                <td>{action.title}</td>
                <td>{action.community_code}</td>
                <td>
                  <span className="chip" data-status={action.status}>
                    <span className="chip-dot" aria-hidden="true" />
                    {actionStatusLabel(action.status)}
                  </span>
                </td>
                <td>{field(action.priority)}</td>
                <td>{field(action.assignee)}</td>
                <td>{field(action.due_date)}</td>
                <td>
                  <button
                    type="button"
                    className="button-text"
                    data-action-id={action.id}
                    onClick={() => openDetail(action.id)}
                  >
                    Ver detalle
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>

      {selectedId !== '' ? (
        <article className="panel actions-detail" id="action-detail" aria-labelledby="action-detail-title">
          <PanelHeading
            eyebrow="ACCIÓN PREVENTIVA"
            title={selected !== null ? selected.title : 'Detalle de la acción'}
            titleId="action-detail-title"
          />
          <StatusCopy id="action-detail-status">{detailMessage}</StatusCopy>
          <dl className="data-list compact" id="action-detail-fields">
            <div>
              <dt>Identificador</dt>
              <dd>{selected !== null ? selected.id : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Comunidad (CUT)</dt>
              <dd>{selected !== null ? selected.community_code : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Estado</dt>
              <dd>{selected !== null ? selected.status : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Prioridad</dt>
              <dd>{selected !== null ? field(selected.priority) : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Responsable</dt>
              <dd>{selected !== null ? field(selected.assignee) : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Fecha límite</dt>
              <dd>{selected !== null ? field(selected.due_date) : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Creada por</dt>
              <dd>{selected !== null ? field(selected.created_by) : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Creada el</dt>
              <dd>{selected !== null ? field(selected.created_at) : 'No disponible'}</dd>
            </div>
            <div>
              <dt>Actualizada el</dt>
              <dd>{selected !== null ? field(selected.updated_at) : 'No disponible'}</dd>
            </div>
          </dl>

          <h4>Historial</h4>
          <div className="table-wrap">
            <table id="action-history">
              <caption className="sr-only">Historial de estados de la acción preventiva</caption>
              <thead>
                <tr>
                  <th scope="col">Movimiento</th>
                  <th scope="col">Actor</th>
                  <th scope="col">Nota</th>
                  <th scope="col">Fecha</th>
                </tr>
              </thead>
              <tbody id="action-history-rows">
                {(selected?.history ?? []).map((entry, index) => (
                  <tr key={entry.id ?? `${entry.to_status}-${index}`}>
                    <td>{historyMove(entry.from_status, entry.to_status)}</td>
                    <td>{field(entry.actor_sub)}</td>
                    <td>{field(entry.note)}</td>
                    <td>{field(entry.created_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>

          <h4>Transiciones</h4>
          <label className="filter-label" htmlFor="transition-note">
            Nota de la transición
            <input
              id="transition-note"
              type="text"
              value={transitionNote}
              disabled={!admin}
              onChange={event => setTransitionNote(event.target.value)}
            />
          </label>
          <div className="actions-transitions" id="action-transitions">
            {transitions.length === 0 ? (
              <p className="panel-note">
                El estado actual no tiene transiciones declaradas por el contrato, o no es un estado que el contrato
                declare.
              </p>
            ) : (
              transitions.map(item => (
                <button
                  key={item.to}
                  type="button"
                  className="secondary-button"
                  data-transition-to={item.to}
                  disabled={!item.enabled || busy}
                  onClick={() => applyTransition(item.to)}
                >
                  {item.label}
                </button>
              ))
            )}
          </div>
        </article>
      ) : null}

      <article className="panel actions-create" aria-labelledby="action-create-title">
        <PanelHeading eyebrow="ALTA" title="Nueva acción preventiva" titleId="action-create-title" />
        <p className="panel-note">
          El alta requiere el rol admin y crea una acción vinculada a la comunidad indicada. El servidor decide y
          responde con el estado real.
        </p>
        <form className="actions-form" onSubmit={submitCreate}>
          <label htmlFor="create-community">
            Comunidad (CUT)
            <input
              id="create-community"
              type="text"
              required
              value={form.communityCode}
              disabled={!admin || busy}
              onChange={event => setForm(previous => ({ ...previous, communityCode: event.target.value }))}
            />
          </label>
          <label htmlFor="create-title">
            Título
            <input
              id="create-title"
              type="text"
              required
              value={form.title}
              disabled={!admin || busy}
              onChange={event => setForm(previous => ({ ...previous, title: event.target.value }))}
            />
          </label>
          <label htmlFor="create-description">
            Descripción
            <textarea
              id="create-description"
              value={form.description}
              disabled={!admin || busy}
              onChange={event => setForm(previous => ({ ...previous, description: event.target.value }))}
            />
          </label>
          <label htmlFor="create-priority">
            Prioridad (texto libre validado por el servicio)
            <input
              id="create-priority"
              type="text"
              value={form.priority}
              disabled={!admin || busy}
              onChange={event => setForm(previous => ({ ...previous, priority: event.target.value }))}
            />
          </label>
          <label htmlFor="create-assignee">
            Responsable
            <input
              id="create-assignee"
              type="text"
              value={form.assignee}
              disabled={!admin || busy}
              onChange={event => setForm(previous => ({ ...previous, assignee: event.target.value }))}
            />
          </label>
          <label htmlFor="create-due-date">
            Fecha límite
            <input
              id="create-due-date"
              type="date"
              value={form.dueDate}
              disabled={!admin || busy}
              onChange={event => setForm(previous => ({ ...previous, dueDate: event.target.value }))}
            />
          </label>
          <button id="create-action" className="primary-button" type="submit" disabled={!admin || busy}>
            Crear acción
          </button>
        </form>
      </article>
    </section>
  );
}
