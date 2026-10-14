-- 002: replace the two frozen CHECK constraints with a real model registry, and
-- give every batch its own assessment.
--
-- 001_initial.sql is deliberately NOT edited. Version 1 is already recorded in
-- schema_migrations on every existing database, so a change there would never run
-- again and would be silently ineffective. Everything here is an ALTER or a
-- CREATE, and every statement is guarded so applying the file twice is a no-op.

-- 1. These two CHECKs froze the placeholder era: a single pinned model version,
--    and a demo flag that could only ever be true. Both are replaced by real data.
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_model_version_check;
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_demo_check;

-- 2. Level, factors and rationale are properties of one evaluation, so they live
--    on the batch that records it.
ALTER TABLE batches ADD COLUMN IF NOT EXISTS risk_level text;
ALTER TABLE batches ADD COLUMN IF NOT EXISTS factors jsonb;
ALTER TABLE batches ADD COLUMN IF NOT EXISTS rationale text;

-- Rows written before this migration were never classified by any classifier: no
-- level was ever computed for them, it was synthesised as a literal at read time.
-- The honest value is "desconocido" - undetermined - and never "bajo".
UPDATE batches SET risk_level = 'desconocido' WHERE risk_level IS NULL;

ALTER TABLE batches ALTER COLUMN risk_level SET NOT NULL;
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_risk_level_check;
ALTER TABLE batches ADD CONSTRAINT batches_risk_level_check
    CHECK (risk_level IN ('bajo', 'medio', 'alto', 'desconocido'));

-- 3. Model identity and lifecycle.
--
--    params_hash is the fingerprint of the effective parameters of that version
--    (classify.ParamsHash over classify.NewAcademicRuleV1().Parameters()). A
--    version therefore identifies a BEHAVIOUR, not a label, which is what makes a
--    stored level reproducible. version is UNIQUE on purpose: a version is
--    immutable, and that unique index is what makes two rows with the same
--    version impossible.
--
--    Factors and rationale are deliberately NOT here: they belong to each
--    evaluation, in batches.
CREATE TABLE IF NOT EXISTS models (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    version     text NOT NULL UNIQUE,
    kind        text NOT NULL,
    params_hash text,
    approved_at timestamptz,
    metrics     jsonb NOT NULL DEFAULT '{}'::jsonb,
    active      boolean NOT NULL DEFAULT false
);

-- 4. The versions that must exist before the foreign key can be added.
--
--    academic-rule-v1 is the classifier this service runs. Its params_hash is the
--    frozen golden value: a test asserts that the literal below still equals
--    classify.AcademicRuleV1ParamsHash, so changing a parameter breaks the build
--    instead of silently redefining an already stored level. approved_at stays
--    null because this is our own rule, explicitly declared non-official.
--
--    demo-fixture-v1 is the placeholder 001 could write. Databases created from
--    001 already contain it in batches, so the foreign key cannot be added without
--    registering it. No classifier ever produced it, so its kind is "placeholder"
--    and it has no reconstructible parameter set: params_hash stays null rather
--    than being invented.
--
--    The nulls are explicitly cast. In a multi-row VALUES list every row in a
--    column is null here, so PostgreSQL would otherwise resolve the untyped NULL
--    literal to text and reject it for the timestamptz column.
INSERT INTO models (version, kind, params_hash, approved_at, metrics, active)
VALUES
    ('academic-rule-v1', 'rule', 'e17aaa5600eb67c9a91340e90057aec100f4e80ef0895662f373cffee66564c7', NULL::timestamptz, '{"official": false, "calibrated_against_observed_fires": false}'::jsonb, true),
    ('demo-fixture-v1', 'placeholder', NULL::text, NULL::timestamptz, '{"official": false, "synthetic": true}'::jsonb, false)
ON CONFLICT (version) DO NOTHING;

-- Any other historical model_version must keep its batches valid, so it is
-- registered with no invented metadata.
INSERT INTO models (version, kind, params_hash, approved_at, metrics, active)
SELECT DISTINCT b.model_version, 'placeholder', NULL::text, NULL::timestamptz, '{}'::jsonb, false
FROM batches AS b
ON CONFLICT (version) DO NOTHING;

-- 5. batches.model_version now points at the registry instead of at a literal.
ALTER TABLE batches DROP CONSTRAINT IF EXISTS batches_model_version_fkey;
ALTER TABLE batches ADD CONSTRAINT batches_model_version_fkey
    FOREIGN KEY (model_version) REFERENCES models(version);

-- The application role reads the registry; only the migration owner writes it.
GRANT SELECT ON models TO risk_app;
