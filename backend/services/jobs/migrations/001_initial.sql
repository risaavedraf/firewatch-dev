CREATE TABLE IF NOT EXISTS jobs (
    id uuid PRIMARY KEY,
    actor_id text NOT NULL,
    idempotency_key uuid NOT NULL,
    zone_id text NOT NULL CHECK (zone_id = 'valparaiso-demo'),
    state text NOT NULL CHECK (state IN ('queued', 'published')),
    batch_id uuid,
    requested_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(actor_id, idempotency_key)
);
CREATE TABLE IF NOT EXISTS outbox (
    id uuid PRIMARY KEY,
    routing_key text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX IF NOT EXISTS outbox_pending ON outbox(created_at) WHERE published_at IS NULL;
CREATE TABLE IF NOT EXISTS inbox (id uuid PRIMARY KEY, processed_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS audit (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id text NOT NULL,
    action text NOT NULL,
    job_id uuid NOT NULL REFERENCES jobs(id),
    trace_id uuid NOT NULL,
    occurred_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT, INSERT, UPDATE ON jobs, outbox TO jobs_app;
GRANT SELECT, INSERT ON inbox, audit TO jobs_app;
GRANT USAGE, SELECT ON SEQUENCE audit_id_seq TO jobs_app;
