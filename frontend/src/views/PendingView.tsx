// N8 completes this view: the table of capabilities the retired mockup offered
// and the backend cannot back, with the reason and where each one would go.
//
// The inventory exists (and now has its own navigation entry) because naming what
// is missing is part of the delivery: nothing here is simulated, and no control
// on this page offers any of it.
export function PendingView() {
  return (
    <section className="view" id="pending" data-view="pending" aria-labelledby="pending-title">
      <p className="eyebrow">ALCANCE DE ESTA ENTREGA</p>
      <h2 id="pending-title">Aún no implementado</h2>
      <p id="pending-status" className="status-copy">
        Estas capacidades del mockup retirado no tienen respaldo en el backend actual. Declararlas es parte de la
        entrega: no se simulan y ningún control de esta página las ofrece.
      </p>
    </section>
  );
}
