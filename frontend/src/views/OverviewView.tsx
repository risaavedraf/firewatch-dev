// N8 completes this view: the touchpoints that follow are the real ones — the
// consolidated metrics, the territorial map drawn from `/v1/risk/zones`, the
// latest published batch with its factors, rationale and policy, and the
// "not implemented" notes for what the backend cannot back. This unit installs
// the router and the shell, so the view carries its identity and its place in the
// navigation without inventing panel content.
export function OverviewView() {
  return (
    <section className="view" id="overview" data-view="overview" aria-labelledby="overview-title">
      <div className="intro-row">
        <div>
          <p className="eyebrow">VISTA TERRITORIAL · VALPARAÍSO</p>
          <h2 id="overview-title">Estado de la zona</h2>
          <p className="subheading">Consulta de lote y trazabilidad disponible desde el servicio FireWatch.</p>
        </div>
        <span className="neutral-tag" id="input-scope-badge">
          ENTRADA SINTÉTICA · SIN CALIBRACIÓN
        </span>
      </div>
    </section>
  );
}
