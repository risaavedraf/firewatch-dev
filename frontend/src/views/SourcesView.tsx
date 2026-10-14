import { UnimplementedNote } from '../components/ui';

// This is the data-status view. The scope declaration the retirement of the
// demonstration strip moved away from the page headline lives here in full, and
// the pipeline panel joins it: the two used to be separate areas, and the user
// grouped them (D1) precisely because both answer "what is this data and what is
// actually verified".
//
// N8 completes this view: the real provenance fields, the derived hash, the
// integrity declaration, the source cutoff and the dependent-services status.
export function SourcesView() {
  return (
    <section className="view" id="sources" data-view="sources" aria-labelledby="sources-title">
      <p className="eyebrow">PROCEDENCIA Y SERVICIOS</p>
      <h2 id="sources-title">Fuentes y servicios</h2>
      <div className="warning-banner" role="note" aria-label="Alcance de los datos de esta página">
        <span className="warning-icon" aria-hidden="true">
          !
        </span>
        <p>
          <strong>Entrada de demostración: todavía no hay ingestión de datos reales.</strong> La API clasifica un lote
          sintético con una regla académica propia, declarada como no oficial y no calibrada contra incendios
          observados. Esta página no predice fecha ni lugar, no emite alertas y no sustituye a CONAF ni a SENAPRED.
          Mientras esa afirmación siga siendo cierta, se mantiene visible aquí y en el aviso de alcance de la barra
          superior.
        </p>
      </div>
      <UnimplementedNote>
        Acciones de revisión e incorporación de registros, y hallazgos de calidad: <code>/v1/risk/sources</code>{' '}
        publica la procedencia y la integridad reales, no un catálogo de acciones ni de hallazgos.
      </UnimplementedNote>
    </section>
  );
}
