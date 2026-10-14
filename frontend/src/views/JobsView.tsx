import { UnimplementedNote } from '../components/ui';

// N8 completes this view: the synthetic scenario selector (kept — the user asked
// for it by name, renamed if needed), the request button, the bounded polling
// status and the zone/job/batch correlation.
export function JobsView() {
  return (
    <section className="view" id="jobs" data-view="jobs" aria-labelledby="jobs-title">
      <p className="eyebrow">LOTE DE DEMOSTRACIÓN</p>
      <h2 id="jobs-title">Procesamiento asíncrono</h2>
      <UnimplementedNote>
        Publicar lote, el avance manual del estado y la reevaluación: no existen como endpoint; el servidor decide y
        publica por su cuenta, y esta página solo solicita el trabajo y lee el resultado.
      </UnimplementedNote>
    </section>
  );
}
