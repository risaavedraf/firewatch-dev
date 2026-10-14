-- 005: batches.zone_id references the zones catalogue instead of a frozen literal.
--
-- 001_initial.sql through 004_zones.sql are never edited: their versions are
-- already recorded in schema_migrations on every existing database, so an edit
-- there would never run again and would be silently ineffective.
--
-- 001 froze a single zone with CHECK (zone_id = 'valparaiso-demo'). That CHECK is
-- replaced here by a foreign key into zones(code), the catalogue created by 004
-- and seeded from the embedded geographic asset. The set of valid zones is data
-- now, not a literal compiled into the schema: extending it is a new row in the
-- catalogue, never an application allowlist or a new CHECK.
--
-- Preconditions satisfied by the migrate path: 004 creates the zones table and
-- the legacy 'valparaiso-demo' row. That row is exactly the only value the dropped
-- CHECK ever allowed, so every existing batch keeps a valid reference and the
-- legacy zone stays usable for historical data. The 38 communes are then loaded
-- by the idempotent seed step. A batch whose zone is absent from zones would make
-- this migration fail, which is the point: the schema refuses to certify a
-- reference the catalogue does not hold.
--
-- Each statement is guarded so that applying the file twice is a no-op: the old
-- CHECK and any previously added foreign key are dropped before the foreign key
-- is (re)added. Nothing here destroys data.
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_zone_id_check;
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_zone_id_fkey;
ALTER TABLE batches ADD CONSTRAINT batches_zone_id_fkey FOREIGN KEY (zone_id) REFERENCES zones(code);
