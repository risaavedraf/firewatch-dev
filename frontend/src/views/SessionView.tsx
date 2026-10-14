// This view carries both the session panel and the permissions matrix (D1).
//
// N8 completes it: the Keycloak sign-in control, the honest note that the access
// token stays in memory, the unverified-role declaration read from
// `realm_access.roles`, and the permission matrix cited guard by guard. There is
// no role selector anywhere: the client cannot grant permissions, and offering a
// choice would be a false authorization boundary.
export function SessionView() {
  return (
    <section className="view" id="session" data-view="session" aria-labelledby="session-title">
      <p className="eyebrow">ACCESO A LA API</p>
      <h2 id="session-title">Sesión y permisos</h2>
    </section>
  );
}
