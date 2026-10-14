-- 002: the local zones catalogue, and the frozen demo CHECK replaced by a
-- foreign key.
--
-- 001_initial.sql is never edited: its version is already recorded in
-- schema_migrations on every existing database, so an edit there would never run
-- again and would be silently ineffective. Every statement below is guarded so
-- that applying this file twice is a no-op, and nothing here destroys data.
--
-- jobs and risk live in DIFFERENT databases, so "the zones catalogue" is not one
-- shared table: each service owns a local copy. This one is the jobs copy, and it
-- exists for exactly one reason -- to be the target of jobs.zone_id. The 38
-- communes still come from the single shared asset contracts/gen/geo
-- (valparaiso-comunas.geojson), which is embedded in both binaries; the idempotent
-- seed step of the migrate path projects it into this table. The communes are
-- deliberately NOT inserted here: writing them again as 38 SQL rows would create
-- two sources for one datum, which is the silent drift this project has already
-- corrected.
--
-- geometry is deliberately ABSENT from this table. jobs never reads or renders a
-- polygon; its copy only answers "is this code a known zone?", so a geometry
-- column here would be an unused duplicate of the risk catalogue. risk owns the
-- geometry, because risk is the service that performs the spatial work.
CREATE TABLE IF NOT EXISTS zones (
    code     text PRIMARY KEY,
    name     text NOT NULL,
    province text,
    kind     text NOT NULL CHECK (kind IN ('comuna','demo'))
);

-- The historical zone. It is not in the geographic asset, so it is the one row
-- this migration must write: the only row the frozen CHECK ever allowed. Without
-- it, every historical jobs row would fail the new foreign key. DO NOTHING keeps
-- re-applying the file a no-op and prevents a stale literal from overwriting a
-- later correction.
INSERT INTO zones(code,name,province,kind)
VALUES ('valparaiso-demo','Valparaiso (demo)','Valparaiso','demo')
ON CONFLICT (code) DO NOTHING;

-- The application role reads zones; only the migration owner writes them.
GRANT SELECT ON zones TO jobs_app;

-- 001 froze a single zone with CHECK (zone_id = 'valparaiso-demo'). That CHECK is
-- replaced here by a foreign key into the local catalogue. The set of valid zones
-- is data now, not a literal compiled into the schema: extending it is a new row
-- in the catalogue, never an application allowlist or a new CHECK. Both the old
-- CHECK and any previously added foreign key are dropped before the foreign key
-- is (re)added, so applying the file twice is a no-op.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_zone_id_check;
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_zone_id_fkey;
ALTER TABLE jobs ADD CONSTRAINT jobs_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES zones(code);
