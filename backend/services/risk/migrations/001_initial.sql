CREATE EXTENSION IF NOT EXISTS postgis;
CREATE TABLE IF NOT EXISTS batches (
    id uuid PRIMARY KEY,
    job_id uuid NOT NULL UNIQUE,
    zone_id text NOT NULL CHECK (zone_id = 'valparaiso-demo'),
    requested_at timestamptz NOT NULL,
    published_at timestamptz NOT NULL DEFAULT now(),
    valid_until timestamptz NOT NULL,
    model_version text NOT NULL CHECK (model_version = 'demo-fixture-v1'),
    demo boolean NOT NULL CHECK (demo),
    location geometry(Point, 4326) NOT NULL,
    CHECK (valid_until > requested_at)
);
CREATE INDEX IF NOT EXISTS batches_latest ON batches(zone_id, requested_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS batches_location ON batches USING gist(location);
CREATE TABLE IF NOT EXISTS outbox (
    id uuid PRIMARY KEY,
    routing_key text NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX IF NOT EXISTS outbox_pending ON outbox(created_at) WHERE published_at IS NULL;
CREATE TABLE IF NOT EXISTS inbox (id uuid PRIMARY KEY, processed_at timestamptz NOT NULL DEFAULT now());
GRANT SELECT, INSERT ON batches, inbox TO risk_app;
GRANT SELECT, INSERT, UPDATE ON outbox TO risk_app;
