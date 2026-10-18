// The preventive-action state machine, transcribed from the frozen contract in
// `odd/tasks/presentation-client-ready.md` §«Contrato congelado de prevention».
//
// Two facts travel with this module:
//   1. it is a *transcription*, not an authority — the service owns the
//      machine, so a `409 invalid_transition` is still rendered honestly when
//      the two ever disagree;
//   2. the role gating below is an affordance built from an UNVERIFIED token
//      claim (`realm_access.roles`), the same fact `SessionView` already shows.
//      It exists so a viewer is not offered an action the server will refuse; it
//      never replaces the server's decision, and the view still renders the
//      server's 403/401/409 as the real answer.

export const ACTION_STATUSES = [
  'propuesta',
  'asignada',
  'en_curso',
  'bloqueada',
  'enviada',
  'aceptada',
  'devuelta',
  'cancelada',
] as const;

export type ActionStatus = (typeof ACTION_STATUSES)[number];

// `aceptada` and `cancelada` are the only terminal states: nothing leaves them.
export const ACTION_TERMINAL_STATUSES: readonly ActionStatus[] = ['aceptada', 'cancelada'];

// The chain of §«Contrato congelado»:
//   propuesta -> asignada -> en_curso -> bloqueada -> en_curso -> enviada
//   -> aceptada | devuelta; devuelta -> en_curso; cancelada from any non-terminal.
const TRANSITIONS: Readonly<Record<ActionStatus, readonly ActionStatus[]>> = {
  propuesta: ['asignada', 'cancelada'],
  asignada: ['en_curso', 'cancelada'],
  en_curso: ['bloqueada', 'enviada', 'cancelada'],
  bloqueada: ['en_curso', 'cancelada'],
  enviada: ['aceptada', 'devuelta', 'cancelada'],
  devuelta: ['en_curso', 'cancelada'],
  aceptada: [],
  cancelada: [],
};

const LABELS: Readonly<Record<ActionStatus, string>> = {
  propuesta: 'Propuesta',
  asignada: 'Asignada',
  en_curso: 'En curso',
  bloqueada: 'Bloqueada',
  enviada: 'Enviada',
  aceptada: 'Aceptada',
  devuelta: 'Devuelta',
  cancelada: 'Cancelada',
};

export function isActionStatus(value: unknown): value is ActionStatus {
  return typeof value === 'string' && (ACTION_STATUSES as readonly string[]).includes(value);
}

// An unknown status has no declared successors, so the view offers no control
// and states the status is not one the contract declares.
export function nextStatuses(status: string): readonly ActionStatus[] {
  return isActionStatus(status) ? TRANSITIONS[status] : [];
}

export function isTransitionAllowed(from: string, to: string): boolean {
  return isActionStatus(to) && nextStatuses(from).includes(to);
}

// A declared status gets its Spanish label; anything else is shown raw rather
// than being silently translated into a value the contract never declared.
export function actionStatusLabel(status: string): string {
  return isActionStatus(status) ? LABELS[status] : status;
}

// The declared `admin` role, read from the token payload WITHOUT verifying its
// signature. `null`, an empty list and a role-less token are all "not admin":
// the client never assumes a permission the token did not declare.
export function declaresAdminRole(roles: readonly string[] | null | undefined): boolean {
  return roles != null && roles.some(role => role === 'admin');
}

export interface TransitionAffordance {
  readonly to: ActionStatus;
  readonly label: string;
  readonly enabled: boolean;
}

// The controls a view may offer for one action: the valid successors, enabled
// only for a signed-in session whose token declares `admin`. The server is still
// the only authority, and every refusal it returns is rendered by the view.
export function transitionAffordances(
  fromStatus: string,
  roles: readonly string[] | null | undefined,
  signedIn: boolean,
): readonly TransitionAffordance[] {
  const enabled = signedIn && declaresAdminRole(roles);
  return nextStatuses(fromStatus).map(to => ({ to, label: actionStatusLabel(to), enabled }));
}

// ── Evidence and sufficiency ──────────────────────────────────────────────
//
// These helpers are a transcription of the S2 server rules for the page's own
// affordances and wording. The server remains the only authority: the page never
// decides sufficiency, it only renders the server's `evidence_sufficient` and the
// refusal the transition route returns.

export const EVIDENCE_DECISIONS = ['suficiente', 'insuficiente'] as const;

export type EvidenceDecision = (typeof EVIDENCE_DECISIONS)[number];

export function isEvidenceDecision(value: unknown): value is EvidenceDecision {
  return typeof value === 'string' && (EVIDENCE_DECISIONS as readonly string[]).includes(value);
}

const EVIDENCE_LABELS: Readonly<Record<EvidenceDecision, string>> = {
  suficiente: 'Suficiente',
  insuficiente: 'Insuficiente',
};

// An unknown decision is shown raw rather than translated into a value the
// contract never declared.
export function evidenceDecisionLabel(decision: string): string {
  return isEvidenceDecision(decision) ? EVIDENCE_LABELS[decision] : decision;
}

// The Spanish sentence the detail renders for the server-computed sufficiency.
// It names the server as the authority and the acceptance consequence, never a
// local decision.
export function sufficiencyCopy(sufficient: boolean): string {
  return sufficient
    ? 'La evidencia vigente tiene una revisión humana suficiente registrada por el servidor; la transición a aceptada está habilitada y el servidor sigue siendo la autoridad.'
    : 'La evidencia vigente es insuficiente para aceptar: no tiene una revisión humana suficiente del lote actual. El servidor rechazará la transición a aceptada hasta que se registre una revisión suficiente de la evidencia vigente.';
}

// One reference per line is the form's input shape. Blanks are dropped, and the
// server re-validates every entry: this only spares the operator an empty submit.
export function parseEvidenceReferences(raw: string): readonly string[] {
  return raw
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(line => line !== '');
}

// Mirrors the server's reference shape for immediate feedback: http(s), a host,
// no embedded credentials, bounded. The server re-validates.
export function isEvidenceReference(raw: string): boolean {
  if (raw === '' || raw.length > 2048 || raw.trim() !== raw) return false;
  // The scheme AND a syntactic authority are required: JS `new URL` would accept
  // `https:///acta` by treating `acta` as the host, which the service's Go parse
  // refuses. The regex keeps the page's rule aligned with the server's.
  if (!/^https?:\/\/[^/?#]+/.test(raw)) return false;
  let parsed: URL;
  try {
    parsed = new URL(raw);
  } catch {
    return false;
  }
  if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') return false;
  if (parsed.hostname === '') return false;
  if (parsed.username !== '' || parsed.password !== '') return false;
  return true;
}

// Shown next to every reassessment: the classification is synthetic and is not
// proof that the action reduced risk.
export const REASSESSMENT_DISCLAIMER =
  'Clasificación sintética e independiente del lote publicado. No es prueba causal de que la acción haya reducido el riesgo; solo registra una reevaluación posterior y trazable.';
