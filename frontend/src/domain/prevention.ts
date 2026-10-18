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
