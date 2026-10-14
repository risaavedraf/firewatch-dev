import { UnimplementedNote } from '../components/ui';

// N8 completes this view: the search box, the data-status filter, the visible
// counter and the rows of `/v1/risk/history` for the selected zone.
export function HistoryView() {
  return (
    <section className="view" id="history" data-view="history" aria-labelledby="history-title">
      <p className="eyebrow">TRAZABILIDAD</p>
      <h2 id="history-title">Historial de lotes</h2>
      <UnimplementedNote>
        Historial de todas las zonas: <code>/v1/risk/history</code> exige <code>zone_id</code> y no existe endpoint
        agregado, así que esta tabla muestra siempre la zona consultada.
      </UnimplementedNote>
    </section>
  );
}
