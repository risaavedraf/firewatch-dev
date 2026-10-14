import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';

import { describe, expect, test } from 'vitest';

import {
  DETACHED_LONGITUDE,
  INSET_VIEW,
  MAINLAND_VIEW,
  buildMapLayout,
  createProjection,
  geometryBounds,
  partitionZones,
  pathData,
  zoneLabel,
} from '../../src/components/map/projection';
import type { GeoGeometry, ZoneCollection, ZoneFeature } from '../../src/types/api';

// The real catalogue the legacy suite read, mapped from the upstream SUBDERE/IGM
// property names to the shape the API serves. Geometry is never synthesised.
// Vitest runs with the frontend package as its root, so the repository root is
// one level up — the same `../../../backend/...` the legacy suite resolved.
const FIXTURE_PATH = resolve(process.cwd(), '../backend/contracts/gen/geo/valparaiso-comunas.geojson');

interface RawFeature {
  geometry: GeoGeometry;
  properties: { CUT_COM: string; COMUNA: string; PROVINCIA: string };
}

const rawCatalogue = JSON.parse(readFileSync(FIXTURE_PATH, 'utf8')) as { features: RawFeature[] };

const realCatalogue: ZoneCollection = {
  type: 'FeatureCollection',
  features: rawCatalogue.features.map(
    (feature): ZoneFeature => ({
      type: 'Feature',
      geometry: feature.geometry,
      properties: {
        code: feature.properties.CUT_COM,
        name: feature.properties.COMUNA,
        province: feature.properties.PROVINCIA,
      },
    }),
  ),
};

function pathPoints(path: string): Array<{ x: number; y: number }> {
  const points: Array<{ x: number; y: number }> = [];
  for (const match of path.matchAll(/[ML](-?\d+(?:\.\d+)?) (-?\d+(?:\.\d+)?)/g)) {
    points.push({ x: Number(match[1]), y: Number(match[2]) });
  }
  return points;
}

function feature(properties: ZoneFeature['properties'], geometry: GeoGeometry): ZoneFeature {
  return { type: 'Feature', properties, geometry };
}

describe('territorial map projection', () => {
  test('the real catalogue splits into a mainland frame plus two detached island frames', () => {
    const layout = buildMapLayout(realCatalogue);

    // 38 geometry-bearing communes are in the asset; the 39th catalogue row is
    // the demo zone, which has no geometry and is therefore not drawn.
    expect(layout.totalFeatures).toBe(38);
    expect(layout.mainlandCount).toBe(36);
    expect(layout.insetCount).toBe(2);
    expect(layout.mainland?.shapes).toHaveLength(36);
    expect(layout.insets).toHaveLength(2);
    expect(layout.insets.map((frame) => frame.shapes[0].code).sort()).toEqual(['05104', '05201']);

    const mainlandCodes = new Set(layout.mainland?.shapes.map((shape) => shape.code));
    expect(mainlandCodes.has('05104')).toBe(false);
    expect(mainlandCodes.has('05201')).toBe(false);

    // Mainland and insets together account for every drawable zone.
    expect((layout.mainland?.shapes.length ?? 0) + layout.insets.length).toBe(38);
  });

  test('mainland geometry stays inside its viewport', () => {
    const layout = buildMapLayout(realCatalogue);
    expect(layout.mainland).not.toBeNull();
    const frame = layout.mainland!;
    for (const shape of frame.shapes) {
      expect(shape.path).not.toBe('');
      for (const point of pathPoints(shape.path)) {
        expect(point.x).toBeGreaterThanOrEqual(0);
        expect(point.x).toBeLessThanOrEqual(frame.width);
        expect(point.y).toBeGreaterThanOrEqual(0);
        expect(point.y).toBeLessThanOrEqual(frame.height);
      }
    }
  });

  test('each island is projected into its own local frame, out of geographic position', () => {
    const layout = buildMapLayout(realCatalogue);
    for (const frame of layout.insets) {
      expect(frame.viewBox).toBe(`0 0 ${INSET_VIEW.width} ${INSET_VIEW.height}`);
      expect(frame.shapes).toHaveLength(1);
      for (const point of pathPoints(frame.shapes[0].path)) {
        expect(point.x).toBeGreaterThanOrEqual(0);
        expect(point.x).toBeLessThanOrEqual(INSET_VIEW.width);
        expect(point.y).toBeGreaterThanOrEqual(0);
        expect(point.y).toBeLessThanOrEqual(INSET_VIEW.height);
      }
    }
  });

  test('detached membership follows the -75 longitude rule from geometry alone', () => {
    expect(DETACHED_LONGITUDE).toBe(-75);

    const polygonAt = (lon: number): GeoGeometry => ({
      type: 'Polygon',
      coordinates: [
        [
          [lon, -33],
          [lon + 0.5, -33],
          [lon + 0.5, -32],
          [lon, -33],
        ],
      ],
    });

    const { mainland, insets } = partitionZones([
      feature({ code: 'west', name: 'West' }, polygonAt(-75.5)),
      feature({ code: 'east', name: 'East' }, polygonAt(-74.5)),
      feature({ code: 'exactly', name: 'Exactly' }, polygonAt(-75)),
    ]);

    expect(mainland.map((entry) => entry.feature.properties.code)).toEqual(['east', 'exactly']);
    expect(insets.map((entry) => entry.feature.properties.code)).toEqual(['west']);
  });

  test('a MultiPolygon degrades to the same path data as the equivalent Polygon', () => {
    const polygon: GeoGeometry = {
      type: 'Polygon',
      coordinates: [
        [
          [-71, -33],
          [-70, -33],
          [-70, -32],
          [-71, -33],
        ],
      ],
    };
    const multiPolygon: GeoGeometry = { type: 'MultiPolygon', coordinates: [polygon.coordinates] };
    const projection = createProjection(geometryBounds(polygon)!, MAINLAND_VIEW, MAINLAND_VIEW.padding);

    const single = pathData(polygon, projection);
    const wrapped = pathData(multiPolygon, projection);
    expect(wrapped).toBe(single);
    expect(single).toMatch(/^M-?\d+\.\d{2} -?\d+\.\d{2} L/);
    expect(single.endsWith('Z')).toBe(true);

    // A MultiPolygon with two parts keeps both subpaths, one per ring.
    const twoParts: GeoGeometry = {
      type: 'MultiPolygon',
      coordinates: [polygon.coordinates, polygon.coordinates],
    };
    expect(pathData(twoParts, projection).match(/Z/g)).toHaveLength(2);
  });

  test('geography-less and empty catalogues draw nothing instead of inventing a shape', () => {
    const empty = buildMapLayout({ type: 'FeatureCollection', features: [] });
    expect(empty.mainland).toBeNull();
    expect(empty.insets).toEqual([]);
    expect(empty.totalFeatures).toBe(0);

    const noGeometry = buildMapLayout({
      type: 'FeatureCollection',
      features: [feature({ code: '05101', name: 'Valparaíso' }, { type: 'Point', coordinates: [] })],
    });
    expect(noGeometry.mainland).toBeNull();
    expect(noGeometry.insets).toEqual([]);
    expect(noGeometry.totalFeatures).toBe(1);

    // The projection itself keeps a degenerate point inside the frame.
    const pointProjection = createProjection(geometryBounds({ type: 'Point', coordinates: [-71, -33] })!, MAINLAND_VIEW, MAINLAND_VIEW.padding);
    const point = pointProjection.project(-71, -33);
    expect(point.x).toBeGreaterThanOrEqual(0);
    expect(point.x).toBeLessThanOrEqual(MAINLAND_VIEW.width);
    expect(point.y).toBeGreaterThanOrEqual(0);
    expect(point.y).toBeLessThanOrEqual(MAINLAND_VIEW.height);
  });

  test('the schematic projection applies the cosine correction at the mean latitude', () => {
    const bounds = { minLon: -72, minLat: -34, maxLon: -70, maxLat: -32 };
    const projection = createProjection(bounds, MAINLAND_VIEW, MAINLAND_VIEW.padding);
    expect(projection.latMean).toBe(-33);
    expect(projection.cosLat).toBeCloseTo(Math.cos((-33 * Math.PI) / 180), 12);

    // Latitude is flipped (north is up) and longitude grows to the right.
    const southWest = projection.project(-72, -34);
    const northEast = projection.project(-70, -32);
    expect(northEast.x).toBeGreaterThan(southWest.x);
    expect(northEast.y).toBeLessThan(southWest.y);
  });

  test('zone labels carry the catalogue identity for the shape title', () => {
    expect(zoneLabel({ code: '05101', name: 'Valparaíso' })).toBe('Valparaíso (05101)');
    expect(zoneLabel({ code: '', name: '' })).toBe('Zona sin nombre (sin código)');
  });
});
