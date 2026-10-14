-- 007: synthetic scenario metadata on batches.
--
-- 001_initial.sql through 006_models_integrity.sql are never edited: their
-- versions are already recorded in schema_migrations on every existing database,
-- so an edit there would never run again and would be silently ineffective.
-- Everything here is guarded so applying the file twice is a no-op, and nothing
-- here destroys data.
--
-- A scenario batch records the demonstration profile that produced its inputs
-- plus the job's server-assigned evaluation instant. Both columns are nullable on
-- purpose: every historical row was written before scenario selection existed,
-- and "no synthetic metadata" is a legitimate state that must stay NULL rather
-- than being backfilled with a guess. An instant is not a measured observation
-- cutoff or a real-data freshness measurement; it is the synthetic evaluation
-- instant the jobs service fixed once for that job.
--
-- The pairing constraint states the one invariant the publication path enforces
-- before acquisition: metadata arrives as a pair. A row can therefore never
-- record an instant for an unnamed scenario, or a scenario without the instant
-- that makes it replayable.
ALTER TABLE batches ADD COLUMN IF NOT EXISTS scenario_id text;
ALTER TABLE batches ADD COLUMN IF NOT EXISTS evaluated_at timestamptz;
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_synthetic_metadata_check;
ALTER TABLE batches ADD CONSTRAINT batches_synthetic_metadata_check
    CHECK ((scenario_id IS NULL) = (evaluated_at IS NULL));
