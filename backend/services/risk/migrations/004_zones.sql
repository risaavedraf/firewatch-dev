-- 004: the zones table, and the one legacy row that predates the geographic asset.
--
-- 001_initial.sql, 002_models.sql and 003_policy_ref.sql are never edited: their
-- versions are already recorded in schema_migrations on every existing database,
-- so an edit there would never run again and would be silently ineffective.
-- Every statement below is guarded so that applying this file twice is a no-op,
-- and nothing here destroys data.
--
-- The 38 communes of Valparaiso are deliberately NOT inserted here. Their CUT,
-- name, province and geometry already live in the versioned derived asset
-- contracts/gen/geo/valparaiso-comunas.geojson, exposed as the shared module
-- firewatch.local/contracts/geo and embedded in the binary. Writing
-- them again as 38 SQL rows would create two sources for one datum, which is the
-- silent drift this project has corrected three times. This file creates what
-- only it can create -- the table, the index and the legacy row -- and the
-- idempotent seed step of the migrate path loads the rows from the asset.

-- geometry is nullable on purpose. Two states are legitimate, not accidents:
-- a database that starts without data must be a valid state (the system then
-- reports insufficient information instead of inventing a classification), and
-- the legacy demo zone below has no geometry because it was created before the
-- asset existed. A NOT NULL column would force a fake geometry into either.
--
-- The declared type is the generic geometry, not Polygon or Point: batches keep a
-- Point while zones hold polygons, and island communes are MultiPolygon, so a
-- narrower type would reject a legitimate source shape.
CREATE TABLE IF NOT EXISTS zones (
    code     text PRIMARY KEY,
    name     text NOT NULL,
    province text,
    kind     text NOT NULL CHECK (kind IN ('comuna','demo')),
    geometry geometry(Geometry,4326)
);

-- Spatial lookups by bounding box are the reason this table exists.
CREATE INDEX IF NOT EXISTS zones_geometry ON zones USING gist(geometry);

-- The historical zone. It is not in the geographic asset, so it is the one row
-- this migration must write. DO NOTHING keeps re-applying the file a no-op and
-- prevents a stale literal from overwriting a later correction.
INSERT INTO zones(code,name,province,kind,geometry)
VALUES ('valparaiso-demo','Valparaiso (demo)','Valparaiso','demo',NULL)
ON CONFLICT (code) DO NOTHING;

-- The application role reads zones; only the migration owner writes them.
GRANT SELECT ON zones TO risk_app;
