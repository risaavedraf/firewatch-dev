-- 008: register the second deterministic rule version in the models registry.
--
-- 002 registered academic-rule-v1 as the active classifier. This migration adds
-- firewatch-rule-v2 next to it without replacing it: active stays false, so a
-- fresh database starts on the same version it always did and the two versions
-- are selectable by MODEL_ID. The row exists before the service can select v2,
-- because batches.model_version is a foreign key into this table.
--
-- The params_hash is the frozen golden value of DefaultFirewatchRuleV2Params,
-- pinned by TestFirewatchRuleV2ParametersAreFrozen. Changing a v2 parameter
-- breaks the build instead of silently redefining a stored level. approved_at
-- stays null because v2 is our own non-official rule.
--
-- Every statement is guarded so applying this file twice is a no-op, and nothing
-- here destroys data.
INSERT INTO models (version, kind, params_hash, approved_at, metrics, active)
VALUES
    ('firewatch-rule-v2', 'rule', '59259c4c0c470e5a0d512fe09f6c748c38fd29f9bc5d760d49754555c66e699b', NULL::timestamptz, '{"official": false, "calibrated_against_observed_fires": false, "aggregation": "dominant_factor"}'::jsonb, false)
ON CONFLICT (version) DO NOTHING;
