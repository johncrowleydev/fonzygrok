CREATE TABLE IF NOT EXISTS connection_log (
    id         SERIAL PRIMARY KEY,
    token_id   TEXT NOT NULL,
    client_ip  TEXT NOT NULL,
    event      TEXT NOT NULL,
    details    TEXT,
    created_at TIMESTAMPTZ NOT NULL
);
