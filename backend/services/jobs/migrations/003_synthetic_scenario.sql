-- 003: optional synthetic scenario metadata on the stored job.
--
-- Forward-only. 001_initial.sql and 002_zones.sql are frozen: their versions are
-- already recorded in schema_migrations on every existing database, so editing
-- them would never run again. Every statement here is guarded so applying this
-- file twice is a no-op, and nothing here backfills or destroys data: historical
-- jobs keep NULL metadata and are never reinterpreted.
--
-- The known scenario vocabulary is deliberately NOT written as a CHECK here.
-- Extending the profiles is a contracts change, and a schema literal would force
-- a new migration for every new profile while creating a second source for one
-- fact. The transport validates membership against the shared wire enum.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS scenario_id text;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS evaluated_at timestamptz;

-- A scenario and its server-assigned instant are one fact, so they are paired:
-- both NULL (legacy job) or both non-NULL (scenario job). The constraint is
-- dropped before it is added so re-applying the file is a no-op.
ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_synthetic_metadata_pairing;
ALTER TABLE jobs ADD CONSTRAINT jobs_synthetic_metadata_pairing CHECK (
    (scenario_id IS NULL AND evaluated_at IS NULL)
    OR (scenario_id IS NOT NULL AND evaluated_at IS NOT NULL)
);
