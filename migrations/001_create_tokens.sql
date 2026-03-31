CREATE TABLE IF NOT EXISTS tokens (
    id          TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    token_hash  TEXT UNIQUE NOT NULL,
    created_at  TEXT NOT NULL,
    last_used_at TEXT,
    is_active   INTEGER NOT NULL DEFAULT 1
);
