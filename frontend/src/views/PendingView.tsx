import { PanelHeading, StatusCopy, UnimplementedBadge } from '../components/ui';

// N8 — the inventory of capabilities the retired mockup offered and the backend
// cannot back. Declaring them is part of the delivery: naming an absence here is
// the only place they appear, so it can never be mistaken for a working feature.
// Nothing in this list is simulated and no control anywhere on the page offers it.
//
// Each entry is ported verbatim from the retired `NOT_IMPLEMENTED` array in
// `legacy/proposal.js`: the copy is Spanish because it is the copy the page
// already shipped, not a new translation.
export type UnimplementedCapability = {
  readonly capability: string;
  readonly reason: string;
  readonly location: string;
};

// The inventory is exported so the test pins the row count to the rendered
// content instead of a copy that could drift. The react-refresh rule wants a
// view module to export components only; this is a read-only data table, not a
// fast-refresh boundary, so its advisory is suppressed here on purpose.
// eslint-disable-next-line react-refresh/only-export-components
export const NOT_IMPLEMENTED: readonly UnimplementedCapability[] = [
  {
    capability: 'Cobertura y entorno por sector',
    reason:
      'La API no publica cobertura ni entorno por comuna: solo el catálogo, el lote publicado y sus factores.',
    location: 'Tabla de sectores',
  },
  {
    capability: 'Contexto y prioridad por sector',
    reason:
      'El backend clasifica la zona con el lote publicado y no calcula prioridad regional ni contexto por comuna.',
    location: 'Ficha de detalle de la zona',
  },
  {
    capability: 'Estados candidate, in_progress, reviewed y closed',
    reason:
      'El backend informa el estado del trabajo (en cola, publicado) y el estado del dato; esos cuatro estados no existen en ningún contrato.',
    location: 'Chips de estado',
  },
  {
    capability: 'Historial de todas las zonas',
    reason:
      'GET /v1/risk/history exige zone_id y no hay endpoint agregado, así que el historial es siempre el de la zona consultada.',
    location: 'Historial de lotes',
  },
  {
    capability: 'Revisar e incorporar registros, y hallazgos de calidad',
    reason:
      'GET /v1/risk/sources publica la procedencia y la integridad reales; no existe un catálogo de acciones ni de hallazgos.',
    location: 'Fuentes y procedencia',
  },
  {
    capability: 'Publicar lote, avance manual y reevaluación',
    reason:
      'No existe endpoint: el servidor decide y publica por su cuenta; la página solo solicita el trabajo y lee el resultado.',
    location: 'Procesamiento asíncrono',
  },
  {
    capability: 'Nombre del actor, detalle libre e id de evento',
    reason:
      'GET /v1/audit entrega actor_id, action, job_id, trace_id y occurred_at: identificadores opacos, sin nombre ni detalle.',
    location: 'Registro de actividad',
  },
];

export function PendingView() {
  return (
    <section className="view" id="pending" data-view="pending" aria-labelledby="pending-title">
      <PanelHeading
        eyebrow="ALCANCE DE ESTA ENTREGA"
        title="Aún no implementado"
        titleId="pending-title"
        badge={
          <span id="pending-badge">
            <UnimplementedBadge />
          </span>
        }
      />
      <StatusCopy id="pending-status">
        Estas capacidades del mockup retirado no tienen respaldo en el backend actual. Declararlas es parte de la
        entrega: no se simulan y ningún control de esta página las ofrece.
      </StatusCopy>
      <div className="table-wrap">
        <table>
          <caption className="sr-only">
            Capacidades sin respaldo en el backend, con su motivo y su ubicación prevista
          </caption>
          <thead>
            <tr>
              <th scope="col">Capacidad</th>
              <th scope="col">Por qué no está</th>
              <th scope="col">Dónde iría</th>
            </tr>
          </thead>
          <tbody id="pending-rows">
            {NOT_IMPLEMENTED.map(entry => (
              <tr key={entry.capability}>
                <td>{entry.capability}</td>
                <td>{entry.reason}</td>
                <td>{entry.location}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </section>
  );
}
