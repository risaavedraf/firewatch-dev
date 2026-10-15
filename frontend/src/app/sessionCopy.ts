// The single source of the copy that explains a session which is not signed in.
//
// The session model can now say WHY it is closed: `reason === 'rejected'` means
// the transport sent a non-empty bearer and the gateway answered 401, so the
// operator WAS connected and the server refused the token (S2/S3). Every view
// keeps its own action phrase and calls into this module for the reason branch,
// so no view re-implements it and no view can accidentally fall back to the
// first-visit text after a rejection.
import type { SessionState } from '../core/session';

// The sentence that names the rejection. It deliberately does not say "conecta"
// as if nothing had happened: the token existed and the server rejected it.
const REJECTED_SESSION =
  'La API rechazó el token (401). La sesión se cerró; conecta nuevamente un token autorizado';

// The rejection sentence on its own, for the transport-failure path where a view
// renders the failure itself instead of the session's idle state. It keeps the
// instruction the sentence carried before this route: naming the rejection is not
// a reason to stop telling the operator what to do next.
export function rejectedSessionCopy(): string {
  return `${REJECTED_SESSION}.`;
}

// A view's idle copy. `action` is the view's own continuation, for example
// "cargar las zonas analizadas" or "consultar el registro de actividad".
export function idleCopy(session: Pick<SessionState, 'reason'>, action: string): string {
  if (session.reason === 'rejected') {
    return `${REJECTED_SESSION} para ${action}.`;
  }
  return `Conecta un token autorizado para ${action}.`;
}

// The sidebar session card's detail line. The anonymous sentence is the legacy
// "Token requerido para consultar"; a rejected token says so instead.
export function sessionDetailCopy(session: Pick<SessionState, 'reason'>): string {
  return session.reason === 'rejected' ? rejectedSessionCopy() : 'Token requerido para consultar';
}
