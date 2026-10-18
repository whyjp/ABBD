CREATE TABLE IF NOT EXISTS heartbeats (
    id     TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    at     TIMESTAMPTZ NOT NULL
);

CREATE INDEX IF NOT EXISTS heartbeats_source_idx ON heartbeats (source);
