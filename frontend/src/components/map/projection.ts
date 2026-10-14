// Schematic geospatial projection for the territorial map.
//
// This is NOT a cartographic projection and NOT a basemap. It is a plain
// equirectangular (plate carree) mapping with a cosine correction at the mean
// latitude of the fitted extent, which keeps the communes from looking
// stretched east-west at Valparaiso's latitude. A tile basemap or a vendored
// mapping library cannot be used: the page CSP closes both `connect-src` and
// `img-src`, so no tile provider is reachable and no external asset may load.
// The requirement is to render the ANALYSED ZONES ourselves — the geography we
// are given, not a base map.
//
// Two communes of the region are oceanic archipelagos far outside the mainland
// coastline. Drawing them in the mainland frame would compress the continental
// communes into a sliver, so they are rendered as EXTERIOR INSETS, each with its
// own local projection and its own scale, and the view states plainly that they
// are NOT geographically positioned. Membership is derived from the geometry
// ALONE: a feature whose bounds lie west of the detached-longitude guard is
// insular, so a newly added island commune becomes an inset automatically and
// there is no separate list to keep in sync.
//
// Framework-free and DOM-free by design: the caller decides how to attach the
// produced path data to an SVG element.

import type { GeoGeometry, ZoneCollection, ZoneFeature, ZoneProperties } from '../../types/api';

// Mainland Valparaiso spans lon [-71.9, -70.1]; the archipelagos lie far west.
// This longitude is the SINGLE criterion for "detached", never a zone list.
export const DETACHED_LONGITUDE = -75;

export interface MapView {
  width: number;
  height: number;
  padding: number;
}

export const MAINLAND_VIEW: MapView = { width: 760, height: 420, padding: 14 };
export const INSET_VIEW: MapView = { width: 168, height: 132, padding: 10 };

export interface Bounds {
  minLon: number;
  minLat: number;
  maxLon: number;
  maxLat: number;
}

export interface ViewSize {
  width: number;
  height: number;
}

export interface Point {
  x: number;
  y: number;
}

export interface Projection {
  readonly latMean: number;
  readonly cosLat: number;
  readonly scale: number;
  project(lon: number, lat: number): Point;
}

export interface ZoneEntry {
  feature: ZoneFeature;
  bounds: Bounds;
}

export interface ProjectedZone {
  code: string;
  name: string;
  province: string;
  label: string;
  path: string;
}

export interface MapFrame {
  /** `0 0 width height`, ready to hand to an SVG `viewBox`. */
  viewBox: string;
  width: number;
  height: number;
  shapes: ProjectedZone[];
}

export interface MapLayout {
  /** The continental frame, or `null` when only islands are drawable. */
  mainland: MapFrame | null;
  /** One frame per detached territory, each with its own local projection. */
  insets: MapFrame[];
  totalFeatures: number;
  mainlandCount: number;
  insetCount: number;
}

// Walks any nesting of coordinate arrays and reports every [lon, lat] position.
// It stops at a position node so a coordinate pair is never descended into.
function visitPositions(node: unknown, onPosition: (lon: number, lat: number) => void): void {
  if (!Array.isArray(node)) return;
  const first = node[0];
  const second = node[1];
  if (typeof first === 'number' && typeof second === 'number') {
    onPosition(first, second);
    return;
  }
  for (const child of node) visitPositions(child, onPosition);
}

export function geometryBounds(geometry: GeoGeometry | null | undefined): Bounds | null {
  if (!geometry || !Array.isArray(geometry.coordinates)) return null;
  const bounds: Bounds = { minLon: Infinity, minLat: Infinity, maxLon: -Infinity, maxLat: -Infinity };
  visitPositions(geometry.coordinates, (lon, lat) => {
    if (!Number.isFinite(lon) || !Number.isFinite(lat)) return;
    if (lon < bounds.minLon) bounds.minLon = lon;
    if (lat < bounds.minLat) bounds.minLat = lat;
    if (lon > bounds.maxLon) bounds.maxLon = lon;
    if (lat > bounds.maxLat) bounds.maxLat = lat;
  });
  return Number.isFinite(bounds.minLon) ? bounds : null;
}

export function collectBounds(entries: readonly ZoneEntry[]): Bounds | null {
  const union: Bounds = { minLon: Infinity, minLat: Infinity, maxLon: -Infinity, maxLat: -Infinity };
  for (const entry of entries) {
    const bounds = entry && entry.bounds ? entry.bounds : null;
    if (!bounds) continue;
    if (bounds.minLon < union.minLon) union.minLon = bounds.minLon;
    if (bounds.minLat < union.minLat) union.minLat = bounds.minLat;
    if (bounds.maxLon > union.maxLon) union.maxLon = bounds.maxLon;
    if (bounds.maxLat > union.maxLat) union.maxLat = bounds.maxLat;
  }
  return Number.isFinite(union.minLon) ? union : null;
}

// Fits `bounds` into a `size` pixel box with a uniform scale, so the whole
// extent stays inside [0, width] x [0, height] even when the extent is a
// degenerate point. The cosine correction keeps east-west distances honest.
export function createProjection(bounds: Bounds, size: ViewSize, padding: number): Projection {
  const innerWidth = Math.max(size.width - padding * 2, 1);
  const innerHeight = Math.max(size.height - padding * 2, 1);
  const latMean = (bounds.minLat + bounds.maxLat) / 2;
  const cosLat = Math.max(Math.cos((latMean * Math.PI) / 180), 1e-6);
  const spanX = Math.max((bounds.maxLon - bounds.minLon) * cosLat, 1e-9);
  const spanY = Math.max(bounds.maxLat - bounds.minLat, 1e-9);
  const scale = Math.min(innerWidth / spanX, innerHeight / spanY);
  const offsetX = padding + (innerWidth - spanX * scale) / 2;
  const offsetY = padding + (innerHeight - spanY * scale) / 2;
  return {
    latMean,
    cosLat,
    scale,
    project(lon: number, lat: number): Point {
      return {
        x: offsetX + (lon - bounds.minLon) * cosLat * scale,
        y: offsetY + (bounds.maxLat - lat) * scale,
      };
    },
  };
}

// A Polygon is one polygon and a MultiPolygon is many; both reduce to the same
// list of polygons so a MultiPolygon of one part degrades exactly like a
// Polygon, and neither geometry kind is treated as a special case downstream.
function polygonList(geometry: GeoGeometry): unknown[] {
  const coordinates: unknown = geometry.coordinates;
  if (!Array.isArray(coordinates)) return [];
  if (geometry.type === 'Polygon') return [coordinates];
  if (geometry.type === 'MultiPolygon') return coordinates;
  return [];
}

export function pathData(geometry: GeoGeometry | null | undefined, projection: Projection): string {
  if (!geometry || !Array.isArray(geometry.coordinates)) return '';
  const parts: string[] = [];
  for (const polygon of polygonList(geometry)) {
    if (!Array.isArray(polygon)) continue;
    for (const ring of polygon) {
      if (!Array.isArray(ring) || ring.length === 0) continue;
      let command = 'M';
      for (const point of ring) {
        if (!Array.isArray(point)) continue;
        const projected = projection.project(point[0], point[1]);
        parts.push(`${command}${projected.x.toFixed(2)} ${projected.y.toFixed(2)}`);
        command = 'L';
      }
      parts.push('Z');
    }
  }
  return parts.join(' ');
}

// Splits the catalogue into mainland features and detached insular features.
// Geometry-less features are dropped: the API already filters them out, and a
// missing geometry is a declared state, not a shape to invent.
export function partitionZones(
  features: readonly (ZoneFeature | null | undefined)[],
): { mainland: ZoneEntry[]; insets: ZoneEntry[] } {
  const mainland: ZoneEntry[] = [];
  const insets: ZoneEntry[] = [];
  for (const feature of features) {
    if (!feature || !feature.geometry) continue;
    const bounds = geometryBounds(feature.geometry);
    if (!bounds) continue;
    const detached = bounds.minLon < DETACHED_LONGITUDE;
    (detached ? insets : mainland).push({ feature, bounds });
  }
  return { mainland, insets };
}

export function countVertices(geometry: GeoGeometry | null | undefined): number {
  if (!geometry || !Array.isArray(geometry.coordinates)) return 0;
  let total = 0;
  visitPositions(geometry.coordinates, () => {
    total += 1;
  });
  return total;
}

export function zoneLabel(properties: ZoneProperties | null | undefined): string {
  const name = properties && properties.name ? String(properties.name) : 'Zona sin nombre';
  const code = properties && properties.code ? String(properties.code) : 'sin código';
  return `${name} (${code})`;
}

function buildFrame(entries: readonly ZoneEntry[], view: MapView): MapFrame {
  const viewBox = `0 0 ${view.width} ${view.height}`;
  const bounds = collectBounds(entries);
  if (!bounds) return { viewBox, width: view.width, height: view.height, shapes: [] };
  const projection = createProjection(bounds, view, view.padding);
  const shapes = entries.map((entry) => {
    const properties = entry.feature.properties;
    return {
      code: typeof properties?.code === 'string' ? properties.code : '',
      name: typeof properties?.name === 'string' ? properties.name : '',
      province: typeof properties?.province === 'string' ? properties.province : '',
      label: zoneLabel(properties),
      path: pathData(entry.feature.geometry, projection),
    };
  });
  return { viewBox, width: view.width, height: view.height, shapes };
}

// Turns a served catalogue into the frames the map renders: one mainland frame
// for the continental features and one frame per detached island, so each island
// gets its own local scale instead of being squeezed into the mainland extent.
export function buildMapLayout(collection: ZoneCollection | null | undefined): MapLayout {
  const features = collection && Array.isArray(collection.features) ? collection.features : [];
  const split = partitionZones(features);
  return {
    mainland: split.mainland.length > 0 ? buildFrame(split.mainland, MAINLAND_VIEW) : null,
    insets: split.insets.map((entry) => buildFrame([entry], INSET_VIEW)),
    totalFeatures: features.length,
    mainlandCount: split.mainland.length,
    insetCount: split.insets.length,
  };
}
