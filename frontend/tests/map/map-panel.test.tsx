import { fireEvent, render, screen, within } from '@testing-library/react';
import userEvent from '@testing-library/user-event';
import type { ComponentProps } from 'react';
import { describe, expect, test, vi } from 'vitest';

import { MapPanel, type ZoneLevelLookup } from '../../src/components/map/MapPanel';
import type { RiskLevel, ZoneCollection, ZoneFeature } from '../../src/types/api';

// The copy the retired page shipped, copied here from `frontend/legacy/index.html`
// so a paraphrase, a translation or a reformat in the component fails the test.
// These two strings are the declared SUBDERE/IGM/INE attribution and the DIFROL
// restriction; they are source metadata, not prose this unit may rewrite.
const ATTRIBUTION =
  'Subsecretaria de Desarrollo Regional y Administrativo (SUBDERE), Instituto Geografico Militar (IGM) e Instituto Nacional de Estadisticas (INE)';
const RESTRICTION =
  'las publicaciones y productos derivados de esta cartografia deben ser revisadas y validadas por DIFROL.';

// A square ring is enough: the map is drawn from real geometry and the tests
// only need the shapes to be distinguishable, so no fixture pretends to be the
// served layer. Local longitudes stay east of the detached guard; the island
// fixture lies west of it, which is the only criterion for "insular".
function square(minLon: number, minLat: number, size = 0.1): ZoneFeature['geometry'] {
  return {
    type: 'Polygon',
    coordinates: [
      [
        [minLon, minLat],
        [minLon + size, minLat],
        [minLon + size, minLat + size],
        [minLon, minLat],
      ],
    ],
  };
}

const VALPARAISO: ZoneFeature = {
  type: 'Feature',
  properties: { code: '05101', name: 'Valparaíso', province: 'Valparaíso' },
  geometry: square(-71.7, -33.1),
};
const VINA_DEL_MAR: ZoneFeature = {
  type: 'Feature',
  properties: { code: '05102', name: 'Viña del Mar', province: 'Valparaíso' },
  geometry: square(-71.6, -33.0),
};
const JUAN_FERNANDEZ: ZoneFeature = {
  type: 'Feature',
  properties: { code: '05201', name: 'Juan Fernández', province: 'Valparaíso' },
  geometry: square(-78.9, -33.6, 0.4),
};

const CATALOGUE: ZoneCollection = { type: 'FeatureCollection', features: [VALPARAISO, VINA_DEL_MAR, JUAN_FERNANDEZ] };
const EMPTY_CATALOGUE: ZoneCollection = { type: 'FeatureCollection', features: [] };

const MAINLAND_SVG_NAME = 'Comunas seleccionables: clic, Enter o Espacio';

function noLevels(): ZoneLevelLookup {
  return { levelFor: () => '' };
}

function rememberedLevels(entries: Record<string, RiskLevel>): ZoneLevelLookup {
  return { levelFor: zone => entries[zone] ?? '' };
}

function baseProps(overrides: Partial<ComponentProps<typeof MapPanel>> = {}): ComponentProps<typeof MapPanel> {
  return {
    collection: CATALOGUE,
    loadState: 'ready',
    sessionReason: null,
    selectedCode: '05101',
    zoneLevels: noLevels(),
    onSelect: vi.fn(),
    ...overrides,
  };
}

function mainlandSvg(): Element {
  // `#map-svg` is part of the legacy contract (N9 inventory) and the SVG is a
  // group whose accessible name only resolves while it is visible, so the
  // hidden-state assertions read the contracted id and the role/name assertions
  // go through `getByRole` separately, when the frame is actually drawn.
  const svg = document.getElementById('map-svg');
  if (!svg) throw new Error('missing DOM map-svg');
  expect(svg).toHaveAttribute('role', 'group');
  expect(svg).toHaveAttribute('aria-label', MAINLAND_SVG_NAME);
  return svg;
}

describe('map panel selector', () => {
  test('the selector offers exactly one option per catalogue comuna', () => {
    render(<MapPanel {...baseProps()} />);

    const select = screen.getByRole('combobox', { name: 'Zona a consultar' });
    const options = within(select).getAllByRole('option');
    expect(options.map(option => option.getAttribute('value'))).toEqual(['05101', '05102', '05201']);
    expect(options.map(option => option.textContent)).toEqual([
      'Valparaíso (05101)',
      'Viña del Mar (05102)',
      'Juan Fernández (05201)',
    ]);
  });

  test('the selector says it has no zones when the catalogue is empty', () => {
    render(<MapPanel {...baseProps({ collection: EMPTY_CATALOGUE })} />);

    const select = screen.getByRole('combobox', { name: 'Zona a consultar' });
    expect(select).toBeDisabled();
    expect(within(select).getByRole('option')).toHaveTextContent('Catálogo sin zonas');
  });

  test('the summary names the selected comuna and counts its geometry honestly', () => {
    render(<MapPanel {...baseProps({ selectedCode: '05201' })} />);

    // The id is part of the legacy contract (inventory requirement), so querying
    // by id here is querying the contract rather than an implementation detail.
    expect(document.getElementById('map-location')?.textContent).toBe('Juan Fernández (05201)');
    // The island's geometry lies west of the detached guard, so the summary says so.
    expect(document.getElementById('map-coordinates')?.textContent).toBe('4 vértices · incluye territorios insulares.');
  });

  test('an unselected catalogue shows the loading explanation, not an invented comuna', () => {
    render(<MapPanel {...baseProps({ collection: null, loadState: 'idle', selectedCode: '' })} />);

    expect(document.getElementById('map-location')?.textContent).toBe('Sin zona seleccionada');
    expect(document.getElementById('map-coordinates')?.textContent).toBe('La capa territorial se carga desde /v1/risk/zones al conectar.');
  });
});

describe('map selection', () => {
  test('click, Enter and Space all report the same comuna code through onSelect', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<MapPanel {...baseProps({ onSelect })} />);

    const vina = screen.getByRole('button', { name: 'Viña del Mar (05102)' });

    await user.click(vina);
    expect(onSelect).toHaveBeenLastCalledWith('05102');

    fireEvent.keyDown(vina, { key: 'Enter' });
    expect(onSelect).toHaveBeenLastCalledWith('05102');

    fireEvent.keyDown(vina, { key: ' ' });
    expect(onSelect).toHaveBeenLastCalledWith('05102');

    expect(onSelect).toHaveBeenCalledTimes(3);
  });

  test('aria-pressed marks exactly the selected comuna and follows a new selection', () => {
    const { rerender } = render(<MapPanel {...baseProps({ selectedCode: '05102' })} />);

    expect(screen.getByRole('button', { name: 'Viña del Mar (05102)' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Valparaíso (05101)' })).toHaveAttribute('aria-pressed', 'false');

    rerender(<MapPanel {...baseProps({ selectedCode: '05101' })} />);

    expect(screen.getByRole('button', { name: 'Valparaíso (05101)' })).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByRole('button', { name: 'Viña del Mar (05102)' })).toHaveAttribute('aria-pressed', 'false');
  });

  test('an island shape is selectable and reports its own catalogue identity', async () => {
    const user = userEvent.setup();
    const onSelect = vi.fn();
    render(<MapPanel {...baseProps({ onSelect })} />);

    await user.click(screen.getByRole('button', { name: 'Juan Fernández (05201)' }));

    expect(onSelect).toHaveBeenCalledExactlyOnceWith('05201');
  });
});

describe('map rendering honesty', () => {
  test('the SVG stays hidden while loading and appears once a mainland frame exists', () => {
    const { rerender } = render(<MapPanel {...baseProps({ collection: null, loadState: 'loading' })} />);

    expect(mainlandSvg()).toHaveAttribute('hidden');
    expect(screen.getByRole('status')).toHaveTextContent('Consultando el catálogo de zonas analizadas…');

    rerender(<MapPanel {...baseProps()} />);

    expect(mainlandSvg()).not.toHaveAttribute('hidden');
    // Once the frame is drawn the SVG is reachable through the accessible
    // surface by its role and name, which is how a screen reader reaches it.
    expect(screen.getByRole('group', { name: MAINLAND_SVG_NAME })).toBe(mainlandSvg());
    // The empty/loading paragraph is a live region; once the frame is drawn it
    // must stop being announced, which `getByRole` proves because it skips
    // hidden nodes by default.
    expect(screen.queryByRole('status')).not.toBeInTheDocument();

    // A cleared session reaches this component as a null collection again: the
    // mainland frame must hide rather than keep painting the closed session's
    // geometry (the legacy readback proved this after logout).
    rerender(<MapPanel {...baseProps({ collection: null, loadState: 'idle' })} />);
    expect(mainlandSvg()).toHaveAttribute('hidden');
    expect(screen.getByRole('status')).toHaveTextContent('Conecta un token autorizado para cargar las zonas analizadas.');
  });

  test('a rejected token names the rejection instead of the first-visit copy (S2)', () => {
    render(<MapPanel {...baseProps({ collection: null, loadState: 'idle', sessionReason: 'rejected' })} />);

    const message = screen.getByRole('status').textContent ?? '';
    expect(message).not.toBe('Conecta un token autorizado para cargar las zonas analizadas.');
    expect(message).toMatch(/rechazó el token/i);
  });

  test('an island-only catalogue keeps the mainland hidden and says why', () => {
    const islandOnly: ZoneCollection = { type: 'FeatureCollection', features: [JUAN_FERNANDEZ] };
    render(<MapPanel {...baseProps({ collection: islandOnly })} />);

    // The island is still drawn, in its own inset frame...
    expect(screen.getByRole('button', { name: 'Juan Fernández (05201)' })).toBeInTheDocument();
    // ...but the mainland SVG is not, and the live region states the reason
    // instead of showing an empty continental frame.
    expect(mainlandSvg()).toHaveAttribute('hidden');
    expect(screen.getByRole('status')).toHaveTextContent(
      'El catálogo no informa geometrías continentales; se muestran solo los territorios insulares.',
    );
  });

  test('an empty catalogue renders the message and draws no shapes', () => {
    render(<MapPanel {...baseProps({ collection: EMPTY_CATALOGUE })} />);

    expect(screen.queryAllByRole('button')).toHaveLength(0);
    expect(screen.getByRole('status')).toHaveTextContent('El catálogo de zonas no informa geometrías: no hay nada que dibujar.');
    expect(mainlandSvg()).toHaveAttribute('hidden');
  });

  test('a failed catalogue load stays hidden with its own message and no invented shapes', () => {
    render(<MapPanel {...baseProps({ collection: null, loadState: 'error' })} />);

    expect(mainlandSvg()).toHaveAttribute('hidden');
    expect(screen.queryAllByRole('button')).toHaveLength(0);
    expect(screen.getByRole('status')).toHaveTextContent('No fue posible cargar el catálogo de zonas analizadas.');
  });

  test('the attribution and the restriction are reproduced verbatim from the legacy markup', () => {
    render(<MapPanel {...baseProps()} />);

    expect(document.getElementById('map-attribution')?.textContent).toBe(ATTRIBUTION);
    expect(document.getElementById('map-restriction')?.textContent).toBe(RESTRICTION);
    expect(screen.getByText(ATTRIBUTION)).toBeInTheDocument();
    expect(screen.getByText(RESTRICTION)).toBeInTheDocument();
  });

  test('the legend lists the five states, including the neutral one', () => {
    render(<MapPanel {...baseProps()} />);

    const legend = screen.getByRole('list', { name: 'Leyenda de niveles de riesgo' });
    expect(within(legend).getAllByRole('listitem').map(item => item.textContent)).toEqual([
      'BAJO',
      'MEDIO',
      'ALTO',
      'INFORMACIÓN INSUFICIENTE',
      'Sin evaluación para esta zona',
    ]);
  });

  test('detached territories render in their own aside, each with a figcaption', () => {
    render(<MapPanel {...baseProps()} />);

    const insets = document.getElementById('map-insets');
    expect(insets?.tagName.toLowerCase()).toBe('aside');
    expect(insets).toHaveAttribute(
      'aria-label',
      'Territorios insulares representados en recuadros aparte, fuera de su posición geográfica',
    );
    expect([...(insets?.querySelectorAll('figcaption') ?? [])].map(node => node.textContent)).toEqual([
      'Juan Fernández (05201)',
    ]);
  });
});

describe('remembered per-zone levels', () => {
  test('a read comuna keeps its colour when the selection moves away, and an unread one stays neutral', () => {
    const { rerender } = render(<MapPanel {...baseProps({ selectedCode: '05101', zoneLevels: rememberedLevels({ '05102': 'medio' }) })} />);

    expect(screen.getByRole('button', { name: 'Viña del Mar (05102)' })).toHaveClass('level-medio');
    expect(screen.getByRole('button', { name: 'Valparaíso (05101)' })).toHaveClass('zone-neutral');

    rerender(<MapPanel {...baseProps({ selectedCode: '05102', zoneLevels: rememberedLevels({ '05102': 'medio' }) })} />);

    // The whole point of the memory: moving the selection leaves the colour of
    // every comuna whose batch was already read, and only the outline moves.
    const vina = screen.getByRole('button', { name: 'Viña del Mar (05102)' });
    expect(vina).toHaveClass('level-medio');
    expect(vina).toHaveClass('zone-selected');
    expect(screen.getByRole('button', { name: 'Valparaíso (05101)' })).toHaveClass('zone-neutral');
  });

  test('a selection with no level is neutral and still shown as selected', () => {
    render(<MapPanel {...baseProps({ selectedCode: '05101', zoneLevels: noLevels() })} />);

    const valparaiso = screen.getByRole('button', { name: 'Valparaíso (05101)' });
    expect(valparaiso).toHaveClass('zone-neutral');
    expect(valparaiso).toHaveClass('zone-selected');
    expect(valparaiso).toHaveAttribute('aria-pressed', 'true');
  });
});
