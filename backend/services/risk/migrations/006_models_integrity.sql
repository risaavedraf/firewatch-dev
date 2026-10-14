-- 006: make the model registry's two structural invariants the schema's job.
--
-- 001_initial.sql through 005_zone_reference.sql are never edited: their versions
-- are already recorded in schema_migrations on every existing database, so an
-- edit there would never run again and would be silently ineffective. Everything
-- here is guarded so applying the file twice is a no-op, and nothing here
-- destroys data.
--
-- Before this file, two states the rest of the service never expects remained
-- representable in the registry:
--
--   1. kind = 'placeholder' together with active = true: the row would claim a
--      version the classifier ran while carrying no reconstructible parameter
--      set (002 leaves params_hash NULL for placeholders). That is invented
--      provenance, and the CHECK below forbids it.
--
--   2. more than one active model: active expresses the registry's lifecycle, and
--      the selected version already comes from MODEL_ID, so a second active row
--      would make "the model" ambiguous. The partial unique index below admits
--      at most one.
--
-- Both are stated in the schema so the invariant holds for every writer, not just
-- for the code paths that happen to remember it.

-- A placeholder never classified anything and has no parameters, so it can never
-- be active. Dropped before it is added so re-applying this file is a no-op.
ALTER TABLE models DROP CONSTRAINT IF EXISTS models_placeholder_not_active_check;
ALTER TABLE models ADD CONSTRAINT models_placeholder_not_active_check
    CHECK (kind <> 'placeholder' OR active = false);

-- At most one model may be active at a time. The index is partial: rows with
-- active = false are excluded, so any number of inactive versions may coexist and
-- the unique constraint only ever compares the active ones.
CREATE UNIQUE INDEX IF NOT EXISTS models_single_active ON models(active) WHERE active;
