-- 011: register the fixture-backed model version the inference service executes.
--
-- 008 registered the second rule version. This migration registers the model
-- path's fixture, so a local stack can exercise the whole publication route -
-- risk -> inference -> POST /predict -> one stored level - before any trained
-- artifact exists. Without the row the service cannot select it, because
-- batches.model_version is a foreign key into this table.
--
-- Two things this row deliberately is not:
--
--   1. It is NOT ml-v1. ml-v1 is the trained model; it reaches the registry
--      together with the artifact that produced it and it becomes the serving
--      version only through POST /v1/models/{version}/activate. A row named
--      ml-v1 pointing at a committed logistic fixture would claim a training run
--      nobody performed.
--   2. It is NOT active. Like firewatch-rule-v2, it is selectable by MODEL_ID and
--      replaces nothing by default.
--
-- kind is 'fixture' rather than 'rule' or 'placeholder': a rule is reproducible
-- from its frozen parameter set, a placeholder never classified anything at all,
-- and this one is a synthetic artifact that really is executed. metrics states
-- the same fact for a reader who only sees the API.
--
-- The params_hash is the fingerprint of the fixture artifact's description
-- (classify.MLFixtureV1Params), pinned by TestMLFixtureV1MigrationRecordsTheFrozenParamsHash.
-- Describing the artifact - format, kind, its own version string, its declared
-- data hash, its feature list in order - is what makes a stored level
-- attributable to something real, and "trained_on_observed_fires": false is the
-- honest answer to the question a params hash exists to answer. Changing any of
-- those values changes the hash and breaks the build, which is the intended
-- response: a different artifact is a new version, never an edit of this one.
--
-- approved_at stays null because no official source approved a fixture.
-- The statement is guarded so applying this file twice is a no-op, and nothing
-- here destroys data.
INSERT INTO models (version, kind, params_hash, approved_at, metrics, active)
VALUES
    ('ml-fixture-v1', 'fixture', '992bd6ac53c75c2d31829c1126c8d2cb97d5a61cad4348d1047d5c93b15a15b5', NULL::timestamptz, '{"official": false, "trained": false, "fixture": true, "served_by": "inference", "artifact": "services/inference/testdata/logistic.json"}'::jsonb, false)
ON CONFLICT (version) DO NOTHING;
