CREATE TABLE IF NOT EXISTS tokens (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    token_hash  TEXT UNIQUE NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL,
    last_used_at TIMESTAMPTZ,
    is_active   BOOLEAN NOT NULL DEFAULT TRUE
);
