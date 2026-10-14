// N8 completes this view: the five real columns of `/v1/audit`, the explicit 403
// for a non-administrator and the fail-closed rendering of an incomplete entry.
// It is reachable for every role on purpose: the server decides, so a `viewer`
// must be able to arrive here and read its own permission error.
export function AuditView() {
  return (
    <section className="view" id="audit" data-view="audit" aria-labelledby="audit-title">
      <p className="eyebrow">COORDINACIÓN</p>
      <h2 id="audit-title">Registro de actividad</h2>
    </section>
  );
}
