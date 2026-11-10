-- 014: the user directory projection and the action's assignment reference.
--
-- Forward-only and guarded: re-applying this file is a no-op, and nothing here
-- backfills, updates or destroys data. Version 14 is the next free number in the
-- SHARED schema_migrations ledger: risk owns 001..008, 010, 012 and 013, this
-- service owns 009 and 011, so 14 is the next integer with both services
-- counted. The rule and its determination are in
-- backend/docs/migration-ledger.md.
--
-- Identity lives in Keycloak, not in this database, and a foreign key needs a
-- local target. prevention.directory_users is therefore a projection of the
-- realm users this service has seen: it is written from the directory's answer,
-- never from client text, and it is the FK target rather than the authority.
--
-- assignee stays exactly as it is. The free-text values of the rows created
-- before this reference existed are data, and rewriting them would destroy the
-- only record of who those actions were assigned to. New assignments use
-- assignee_id; assignee is legacy on read.
--
-- ON DELETE RESTRICT is deliberate. An assignment states who is responsible for
-- a live action, and deleting the directory row under it would silently
-- unassign work that is in progress. RESTRICT makes the database refuse the
-- delete instead, so an operator must reassign (or the action must be closed)
-- before the projection row can be removed.

CREATE TABLE IF NOT EXISTS prevention.directory_users (
    -- The identifier the identity provider assigns. The primary key is the
    -- unique index the projection upsert uses: ON CONFLICT (id) is one probe.
    id uuid PRIMARY KEY,
    -- The name the directory reported. Never client-supplied and never blank:
    -- an assignment must present someone, and the transport falls back to the
    -- identifier when the directory reports no name.
    display_name text NOT NULL CHECK (btrim(display_name) <> ''),
    -- When this projection was last refreshed from the directory. It is stamped
    -- on every upsert, so a stale row is visible instead of silently trusted.
    synced_at timestamptz NOT NULL DEFAULT now()
);

ALTER TABLE prevention.actions
    ADD COLUMN IF NOT EXISTS assignee_id uuid
    REFERENCES prevention.directory_users(id) ON DELETE RESTRICT;

-- The foreign key's RESTRICT check looks up the referencing rows when a
-- directory user is deleted; without this index that check scans every action.
CREATE INDEX IF NOT EXISTS actions_assignee
    ON prevention.actions(assignee_id);

-- The application role reads the projection and refreshes it on assignment, so
-- the upsert needs INSERT and UPDATE as well as SELECT. There is no DELETE
-- grant: a directory row an action references is the parent of a live
-- assignment, and the database already refuses its removal.
GRANT SELECT, INSERT, UPDATE ON prevention.directory_users TO risk_app;
