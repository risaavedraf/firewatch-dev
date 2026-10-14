-- 003: record the effective policy reference next to each assessment.
--
-- A stored level is only reproducible if the parameters that produced it travel
-- with it. policy_ref carries them structured:
--   {"policy_id":"academic-rule-v1","parameters":{...},"params_hash":"<sha256>"}
--
-- Rows written before this migration were classified before the reference
-- existed, so policy_ref stays NULL for them. It is NOT backfilled with the
-- current parameters: that would claim they were produced by parameters nobody
-- recorded, which is exactly the invented provenance the rest of this schema
-- refuses.
--
-- 001_initial.sql and 002_models.sql are never edited: their versions are already
-- recorded in schema_migrations, so an edit there would be silently ineffective.
-- Every statement is guarded so applying the file twice is a no-op.
ALTER TABLE batches ADD COLUMN IF NOT EXISTS policy_ref jsonb;

-- A reference that exists must be usable: without an identifier and a hash it
-- cannot reproduce anything, so it is refused rather than stored half-written.
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_policy_ref_check;
ALTER TABLE batches ADD CONSTRAINT batches_policy_ref_check
    CHECK (policy_ref IS NULL OR (policy_ref ? 'policy_id' AND policy_ref ? 'params_hash'));
