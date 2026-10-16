// The territorial map panel: the view surface of the real `/v1/risk/zones`
// catalogue. It is props-driven on purpose — it fetches nothing, knows no
// session and never decides which zone is selected. The caller owns the data,
// the selection and the remembered levels; this component only renders them.
//
// Two honesty rules shape the markup:
//   1. A comuna is painted with a risk colour ONLY when a canonical level was
//      read for it. The catalogue carries no per-commune classification, so the
//      absence of a level stays neutral instead of becoming a plausible (and
//      false) severity. A comuna whose level was read keeps its colour when the
//      selection moves away; the outline, never the fill, marks the selection.
//   2. The mainland frame is drawn only when the catalogue actually carries
//      continental geometry. Until then the SVG stays `hidden` and the live
//      region in `#map-empty` states, in Spanish, why there is nothing to draw.
//
// The element identifiers are the legacy contract preserved for N9: they are
// spelled exactly as the retired page had them.

import { useMemo } from 'react';

import { PanelHeading } from '../ui';
import { idleCopy } from '../../app/sessionCopy';
import type { RiskLevel, ZoneCollection } from '../../types/api';
import { buildMapLayout, countVertices, DETACHED_LONGITUDE, geometryBounds, zoneLabel, type MapFrame, type ProjectedZone } from './projection';
import { zoneShapeClasses } from './zone-classes';

// How the caller describes the catalogue read. `idle` is "there is nothing to
// ask for yet" (typically no token), `loading` is a read in flight, `ready` is
// a settled read and `error` is a failed one. The component picks the honest
// empty message from this state; it never infers progress from `collection`.
export type MapLoadState = 'idle' | 'loading' | 'ready' | 'error';

// The remembered per-zone levels. `ZoneLevelMemory` satisfies this shape as is,
// so the caller can hand the memory object straight through: the map reads a
// canonical level per comuna and paints nothing when the memory has none.
export interface ZoneLevelLookup {
  levelFor(zone: string): RiskLevel | '';
}

export interface MapPanelProps {
  collection: ZoneCollection | null;
  loadState: MapLoadState;
  selectedCode: string;
  zoneLevels: ZoneLevelLookup;
  onSelect: (code: string) => void;
  // The session's rejection reason, so the idle panel can name a refused token
  // instead of the first-visit copy. The panel does not read the session store:
  // the caller hands it the fact (S2).
  sessionReason: 'rejected' | null;
}

const MAINLAND_SVG_LABEL = 'Comunas seleccionables: clic, Enter o Espacio';
const INSETS_LABEL = 'Territorios insulares representados en recuadros aparte, fuera de su posición geográfica';
const LEGEND_LABEL = 'Leyenda de niveles de riesgo';

// Reproduced verbatim from the retired page: this is the provenance the source
// declares, not copy this unit may translate or reformat.
const ATTRIBUTION =
  'Subsecretaria de Desarrollo Regional y Administrativo (SUBDERE), Instituto Geografico Militar (IGM) e Instituto Nacional de Estadisticas (INE)';
const RESTRICTION =
  'las publicaciones y productos derivados de esta cartografia deben ser revisadas y validadas por DIFROL.';

// The five legend states. Four are the canonical levels the API may serve;
// the fifth is the absence of an assessment, which is not a level at all.
const LEGEND_STATES: ReadonlyArray<{ swatch: string; label: string }> = [
  { swatch: 'swatch-bajo', label: 'BAJO' },
  { swatch: 'swatch-medio', label: 'MEDIO' },
  { swatch: 'swatch-alto', label: 'ALTO' },
  { swatch: 'swatch-desconocido', label: 'INFORMACIÓN INSUFICIENTE' },
  { swatch: 'swatch-neutral', label: 'Sin evaluación para esta zona' },
];

// An `Enter` or `Space` keydown selects, because the shape is a `role="button"`
// and a pointer-only map would be unusable with a keyboard. `preventDefault`
// stops the page scrolling on Space, exactly as the retired page did.
function isActivationKey(key: string): boolean {
  return key === 'Enter' || key === ' ';
}

// The view's own action phrase; the reason branch lives in `sessionCopy`, so
// this panel never re-implements it.
const IDLE_ACTION = 'cargar las zonas analizadas';

function emptyMessageFor(loadState: MapLoadState, hasZones: boolean, idleMessage: string): string {
  if (loadState === 'loading') return 'Consultando el catálogo de zonas analizadas…';
  if (loadState === 'error') return 'No fue posible cargar el catálogo de zonas analizadas.';
  if (loadState === 'ready' || hasZones) return 'El catálogo de zonas no informa geometrías: no hay nada que dibujar.';
  return idleMessage;
}

export function MapPanel({ collection, loadState, selectedCode, zoneLevels, onSelect, sessionReason }: MapPanelProps) {
  const layout = useMemo(() => buildMapLayout(collection), [collection]);
  const features = collection?.features ?? [];
  const hasZones = features.length > 0;

  const zoneOptions = useMemo(() => {
    const options: Array<{ code: string; label: string }> = [];
    for (const feature of features) {
      const code = feature?.properties?.code;
      if (typeof code !== 'string' || code === '') continue;
      options.push({ code, label: zoneLabel(feature.properties) });
    }
    options.sort((left, right) => left.code.localeCompare(right.code));
    return options;
    // `features` is re-derived from `collection` on every render, so the
    // dependency below is the one that actually changes identity.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [collection]);

  const selectedFeature = selectedCode === '' ? null : features.find(feature => feature?.properties?.code === selectedCode) ?? null;

  let locationText = 'Sin zona seleccionada';
  let coordinatesText = 'La capa territorial se carga desde /v1/risk/zones al conectar.';
  if (selectedFeature) {
    locationText = zoneLabel(selectedFeature.properties);
    // Same longitude rule as the partition: insular is a property of the
    // geometry, never of a hardcoded code list.
    const bounds = geometryBounds(selectedFeature.geometry);
    const insular = bounds && bounds.minLon < DETACHED_LONGITUDE ? ' · incluye territorios insulares' : '';
    coordinatesText = `${countVertices(selectedFeature.geometry)} vértices${insular}.`;
  }

  const mainland: MapFrame | null = layout.mainland && layout.mainland.shapes.length > 0 ? layout.mainland : null;
  const showEmpty = mainland === null;
  // TypeScript's `SVGProps` does not declare the global `hidden` attribute, yet
  // the legacy contract (and the N9 readback) requires the SVG to carry exactly
  // that attribute until there is a mainland frame. Spreading keeps the runtime
  // attribute the retired page had while staying type-safe at the call site.
  const hiddenAttribute: { hidden?: boolean } = mainland === null ? { hidden: true } : {};
  const emptyMessage = mainland
    ? ''
    : layout.insetCount > 0
      ? 'El catálogo no informa geometrías continentales; se muestran solo los territorios insulares.'
      : emptyMessageFor(loadState, hasZones, idleCopy({ reason: sessionReason }, IDLE_ACTION));

  const renderShape = (shape: ProjectedZone, key: string) => {
    const selected = shape.code !== '' && shape.code === selectedCode;
    const level = zoneLevels.levelFor(shape.code);
    return (
      <path
        key={key}
        className={zoneShapeClasses(level, selected).join(' ')}
        d={shape.path}
        data-zone={shape.code}
        role="button"
        tabIndex={0}
        aria-label={shape.label}
        aria-pressed={selected}
        onClick={() => onSelect(shape.code)}
        onKeyDown={event => {
          if (!isActivationKey(event.key)) return;
          event.preventDefault();
          onSelect(shape.code);
        }}
      >
        <title>{shape.label}</title>
      </path>
    );
  };

  return (
    <article className="panel map-panel">
      <PanelHeading
        eyebrow="ZONAS ANALIZADAS · REGIÓN DE VALPARAÍSO"
        title="Vista territorial de las comunas"
        badge={<span className="neutral-tag">Proyección esquemática</span>}
      />

      <div className="map-toolbar">
        <label htmlFor="zone-select">Zona consultada</label>
        <select
          id="zone-select"
          aria-label="Zona a consultar"
          value={zoneOptions.length === 0 ? '' : selectedCode}
          onChange={event => onSelect(event.target.value)}
          disabled={zoneOptions.length === 0}
        >
          {zoneOptions.length === 0 ? (
            <option value="">{loadState === 'ready' ? 'Catálogo sin zonas' : 'Selecciona una zona analizada'}</option>
          ) : (
            zoneOptions.map(zone => (
              <option key={zone.code} value={zone.code}>
                {zone.label}
              </option>
            ))
          )}
        </select>
        <p className="map-summary">
          <strong id="map-location">{locationText}</strong>
          <span id="map-coordinates">{coordinatesText}</span>
        </p>
      </div>

      <div className="map-stage">
        <div className="map-viewport">
          <svg
            id="map-svg"
            className="map-svg"
            role="group"
            aria-label={MAINLAND_SVG_LABEL}
            viewBox={mainland ? mainland.viewBox : undefined}
            preserveAspectRatio={mainland ? 'xMidYMid meet' : undefined}
            {...hiddenAttribute}
          >
            <title id="map-title">Mapa esquemático de las comunas de la Región de Valparaíso</title>
            <desc id="map-desc">
              Cada comuna se dibuja con una proyección esquemática propia, sin mapa base ni teselas. Cada comuna con un
              lote publicado conserva su color según el último lote de esa zona, aunque no esté seleccionada; el gris
              neutro significa que esa comuna no tiene lote publicado.
            </desc>
            {mainland ? <g className="zone-layer">{mainland.shapes.map((shape, index) => renderShape(shape, `${shape.code}-${index}`))}</g> : null}
          </svg>
          <p id="map-empty" className="map-empty" role="status" aria-live="polite" hidden={!showEmpty}>
            {emptyMessage}
          </p>
        </div>
        <aside className="map-insets" id="map-insets" aria-label={INSETS_LABEL}>
          {layout.insets.map((frame, frameIndex) => {
            const shape = frame.shapes[0];
            if (!shape) return null;
            return (
              <figure className="map-inset" key={`${shape.code}-${frameIndex}`}>
                <svg viewBox={frame.viewBox} preserveAspectRatio="xMidYMid meet" role="group" aria-label={shape.label}>
                  <g className="zone-layer">{renderShape(shape, `${shape.code}-${frameIndex}`)}</g>
                </svg>
                <figcaption>{shape.label}</figcaption>
              </figure>
            );
          })}
        </aside>
      </div>

      <p className="map-inset-note">
        Los recuadros insulares corresponden a comunas de la región que están lejos del continente, Juan Fernández e
        Isla de Pascua. Se dibujan aparte, con su propia escala, y NO están posicionados geográficamente respecto del
        continente.
      </p>

      <ul className="map-legend" aria-label={LEGEND_LABEL}>
        {LEGEND_STATES.map(state => (
          <li key={state.swatch}>
            <span className={`legend-swatch ${state.swatch}`} aria-hidden="true" />
            <span>{state.label}</span>
          </li>
        ))}
      </ul>
      <p className="map-legend-note">
        Cada comuna con un lote publicado conserva su color según el último lote de esa zona, aunque no esté
        seleccionada; el gris neutro significa que esa comuna no tiene lote publicado. El contorno indica la selección.
        El color requiere un lote con campos de clasificación válidos; geografía real no significa riesgo observado.
      </p>

      <div className="map-source">
        <p>
          <span className="source-label">ATRIBUCIÓN</span>
          <span id="map-attribution">{ATTRIBUTION}</span>
        </p>
        <p>
          <span className="source-label">RESTRICCIÓN</span>
          <span id="map-restriction">{RESTRICTION}</span>
        </p>
        <p className="map-caption">
          Atribución y restricción declaradas por la fuente IDE Chile. Cartografía esquemática para una vista regional:
          no es un mapa base ni una proyección cartográfica, y no sustituye a la cartografía oficial.
        </p>
      </div>
    </article>
  );
}
