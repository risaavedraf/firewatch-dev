-- 008: preventive actions and their attributable history.
--
-- Forward-only and guarded: re-applying this file is a no-op, and nothing here
-- backfills or destroys data.
--
-- Version 8, not 1, is deliberate. The frozen contract names this migration
-- 008_preventive_actions.sql, and schema_migrations already holds 001..007 of the
-- risk database, so the preventive tables take the next version of that ledger.
-- They are created inside their own PostgreSQL SCHEMA, `prevention`, which is
-- what the contract names ("esquema PostgreSQL prevention"): the two tables are
-- addressed as prevention.actions and prevention.action_history and are not
-- mixed with the risk service's public tables.
--
-- Because the migration runner is executed by the database superuser while the
-- API connects with the unprivileged application role, the grants below are part
-- of the migration and not of the deployment. The application role is the one
-- issued for the database this schema lives in (risk_app today); an independent
-- `prevention` database would only change this role name and the DATABASE_URL,
-- never the schema below.

CREATE SCHEMA IF NOT EXISTS prevention;

CREATE TABLE IF NOT EXISTS prevention.actions (
    id uuid PRIMARY KEY,
    community_code text NOT NULL CHECK (btrim(community_code) <> ''),
    title text NOT NULL CHECK (btrim(title) <> ''),
    -- description, priority and assignee are free text on purpose: the frozen
    -- contract names the columns but defines no vocabulary or default for them,
    -- and inventing an enum here would reject values the frontend is free to
    -- send. priority defaults to the empty string, which means "not stated",
    -- instead of a fabricated 'media'.
    description text NOT NULL DEFAULT '',
    priority text NOT NULL DEFAULT '',
    status text NOT NULL CHECK (status IN
        ('propuesta','asignada','en_curso','bloqueada','enviada','aceptada','devuelta','cancelada')),
    assignee text,
    due_date date,
    -- Who asked for the action, taken from the verified token subject: an audit
    -- field, never a client-supplied identity.
    created_by text NOT NULL CHECK (btrim(created_by) <> ''),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- The list route filters by community_code and optionally by status and orders
-- by recency, so one index serves both the filtered and the unfiltered read.
CREATE INDEX IF NOT EXISTS actions_community_status
    ON prevention.actions(community_code, status, created_at DESC, id DESC);

-- One row per accepted transition, written in the same transaction as the status
-- change. from_status is NOT NULL because creation is not a transition: the
-- first history row is propuesta -> asignada, so there is no null source state.
-- The identity sequence is the chronological key a detail read orders by.
CREATE TABLE IF NOT EXISTS prevention.action_history (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    action_id uuid NOT NULL REFERENCES prevention.actions(id),
    from_status text NOT NULL,
    to_status text NOT NULL,
    -- Verified token subject of the operator who asked for the transition.
    actor_sub text NOT NULL CHECK (btrim(actor_sub) <> ''),
    -- Optional free-text reason. Empty means the transition carried no note.
    note text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS action_history_action
    ON prevention.action_history(action_id, id);

-- The application role reads and advances actions and appends history. It can
-- never delete either: there is no DELETE grant, which is what keeps the audit
-- trail append-only.
GRANT USAGE ON SCHEMA prevention TO risk_app;
GRANT SELECT, INSERT, UPDATE ON prevention.actions TO risk_app;
GRANT SELECT, INSERT ON prevention.action_history TO risk_app;
GRANT USAGE, SELECT ON SEQUENCE prevention.action_history_id_seq TO risk_app;
