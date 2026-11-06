-- 012: store the readings history the trained model's daily aggregates come from.
--
-- The model path (L9) sent five of its seven features as explicit nulls because
-- this service kept no history: a provider that delivers instantaneous conditions
-- cannot produce a daily maximum or a seven day precipitation sum. This table is
-- that history. It accumulates from the first cycle, so the features stop being
-- null once the table covers their window, and it survives a restart and is
-- shared by every replica - which is why it is a table and not process memory.
--
-- Identity is (zone_id, variable, observed_at). observations.Validate already
-- uses exactly that key to detect a duplicated reading inside one retrieval, so
-- the uniqueness constraint makes the same rule hold across retrievals: appending
-- the same reading twice is a no-op instead of a second row. That is what keeps
-- re-ingestion idempotent, and it is why a redelivered publication cannot inflate
-- a precipitation sum.
--
-- There is deliberately NO foreign key to zones. A reading is what a source said
-- about a place at an instant; refusing to store it because the catalogue has not
-- caught up yet would lose history that can never be re-fetched, and the zone is
-- only used to select a window. The batches table keeps its foreign key, because a
-- published level is attributed to a commune and that attribution must be valid.
--
-- The aggregates are computed on read, never rolled up here. At roughly 38
-- communes x a handful of variables x 96 readings a day, a rollup table would add
-- a second source of truth, a backfill and a rebuild path, and would still have to
-- answer what a late reading does to an already computed total. This is stated
-- here so a later reader does not think the rollup was forgotten.
--
-- Every statement is guarded so applying this file twice is a no-op, and nothing
-- here destroys data.
CREATE TABLE IF NOT EXISTS readings (
    zone_id     text             NOT NULL,
    variable    text             NOT NULL,
    observed_at timestamptz      NOT NULL,
    value       double precision NOT NULL,
    unit        text             NOT NULL,
    source_id   text             NOT NULL,
    quality     text             NOT NULL,
    ingested_at timestamptz      NOT NULL DEFAULT now(),
    CONSTRAINT readings_identity UNIQUE (zone_id, variable, observed_at)
);

-- The read shape is "the latest N days of one variable in one zone": equality on
-- zone and variable, a range on the instant, newest first. The uniqueness
-- constraint's own index already serves that scan, and this index states the
-- shape explicitly so the intent survives a future change to the identity.
CREATE INDEX IF NOT EXISTS readings_variable_window ON readings(zone_id, variable, observed_at DESC);

-- The application role appends readings and reads windows; only the migration
-- owner creates schema. No UPDATE/DELETE grant: the history is append-only, and a
-- correction arrives as its own reading rather than by rewriting the past.
GRANT SELECT, INSERT ON readings TO risk_app;
