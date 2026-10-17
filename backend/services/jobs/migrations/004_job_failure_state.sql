-- 004: the terminal failure outcome on the stored job.
--
-- Forward-only. 001, 002 and 003 are frozen: their versions are already recorded
-- in schema_migrations on every existing database, so editing them would never
-- run again. Every statement below is guarded so applying this file twice is a
-- no-op, and nothing here backfills or destroys data: historical jobs keep
-- attempts 0 and NULL failure fields.
--
-- 001 froze the state CHECK with only 'queued' and 'published'. It is dropped
-- and re-added here with the new terminal value instead of being edited in 001,
-- which would never run again. The drop/re-add makes re-applying the file a
-- no-op. attempts is durable evidence: the count no longer lives only in process
-- memory, where a consumer restart silently reset it.
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS attempts integer NOT NULL DEFAULT 0;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS failure_reason text;
ALTER TABLE jobs ADD COLUMN IF NOT EXISTS failed_at timestamptz;

ALTER TABLE jobs DROP CONSTRAINT IF EXISTS jobs_state_check;
ALTER TABLE jobs ADD CONSTRAINT jobs_state_check CHECK (state IN ('queued', 'published', 'failed'));
