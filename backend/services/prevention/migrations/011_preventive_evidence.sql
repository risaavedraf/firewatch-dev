-- 011: preventive evidence references, human sufficiency reviews and linked
-- synthetic reassessments.
--
-- Forward-only and guarded: re-applying this file is a no-op, and nothing here
-- backfills, updates or destroys data. Version 11 is the next free version in the
-- SHARED schema_migrations ledger: risk owns 001..008 and 010, this service owns
-- 009, and 011 is deliberately chosen instead of 010 so two parallel writers can
-- never record one version twice.
--
-- Three append-only tables, all inside the `prevention` schema:
--
--   action_evidence          one admin-recorded snapshot: the sufficiency
--                            criterion plus the bounded, credential-free
--                            references a human is asked to judge. A later
--                            snapshot never edits an earlier one.
--   action_evidence_review   one attributable human decision about exactly one
--                            snapshot. reviewer_sub is the verified token subject
--                            and reviewed_at is the server clock; neither is
--                            client-supplied. Because a decision names its
--                            snapshot, recording new evidence makes an earlier
--                            approval stale instead of silently reusing it.
--   action_reassessment      the link between an action and one REAL published
--                            synthetic batch. Every classification field is
--                            copied from the shared risk database by the service;
--                            the client only ever names the job. There is no FK to
--                            `batches` on purpose: this migration must apply even
--                            when the risk migrations have not run yet (the compose
--                            test runners are independent), so the FK would add a
--                            cross-service startup ordering dependency. The
--                            service validates the batch against `batches` before
--                            inserting here.
--
-- The application role may read and append these rows but never delete or update
-- them: the append-only guarantee is the grants below, not a convention.

CREATE TABLE IF NOT EXISTS prevention.action_evidence (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    action_id uuid NOT NULL REFERENCES prevention.actions(id),
    criterion text NOT NULL CHECK (btrim(criterion) <> ''),
    -- text[] (not jsonb) because the wire shape is a list of strings and the
    -- service scans it directly. cardinality > 0 keeps an empty citation list
    -- unrepresentable, matching the API rule that one reference is required.
    evidence_references text[] NOT NULL CHECK (cardinality(evidence_references) > 0),
    created_by text NOT NULL CHECK (btrim(created_by) <> ''),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS action_evidence_action
    ON prevention.action_evidence(action_id, id);

CREATE TABLE IF NOT EXISTS prevention.action_evidence_review (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    action_id uuid NOT NULL REFERENCES prevention.actions(id),
    evidence_id bigint NOT NULL REFERENCES prevention.action_evidence(id),
    decision text NOT NULL CHECK (decision IN ('suficiente','insuficiente')),
    reviewer_sub text NOT NULL CHECK (btrim(reviewer_sub) <> ''),
    note text NOT NULL DEFAULT '',
    reviewed_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS action_evidence_review_action
    ON prevention.action_evidence_review(action_id, id);

CREATE TABLE IF NOT EXISTS prevention.action_reassessment (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    action_id uuid NOT NULL REFERENCES prevention.actions(id),
    -- Unique so one real publication can never be relabelled under a second
    -- action, or linked twice to the same one.
    job_id uuid NOT NULL UNIQUE,
    batch_id uuid NOT NULL,
    version text NOT NULL CHECK (btrim(version) <> ''),
    risk_level text NOT NULL CHECK (risk_level IN ('bajo','medio','alto','desconocido')),
    -- The batch's own commune, copied from `batches.zone_id`, so a reader can see
    -- the link was checked against this action's community_code.
    zone_id text NOT NULL CHECK (btrim(zone_id) <> ''),
    -- Copied from `batches.demo`. The service refuses a non-synthetic batch, so
    -- an accepted row records the real persisted value instead of stamping true.
    synthetic boolean NOT NULL,
    -- The batch's publication instant, copied from `batches.published_at`; it is
    -- the temporal proof that the classification is later than the reviewed
    -- evidence, not a value the client can supply.
    batch_published_at timestamptz NOT NULL,
    linked_by text NOT NULL CHECK (btrim(linked_by) <> ''),
    linked_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS action_reassessment_action
    ON prevention.action_reassessment(action_id, id);

GRANT SELECT, INSERT ON prevention.action_evidence TO risk_app;
GRANT SELECT, INSERT ON prevention.action_evidence_review TO risk_app;
GRANT SELECT, INSERT ON prevention.action_reassessment TO risk_app;
GRANT USAGE, SELECT ON SEQUENCE prevention.action_evidence_id_seq TO risk_app;
GRANT USAGE, SELECT ON SEQUENCE prevention.action_evidence_review_id_seq TO risk_app;
GRANT USAGE, SELECT ON SEQUENCE prevention.action_reassessment_id_seq TO risk_app;
