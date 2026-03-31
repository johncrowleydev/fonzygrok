CREATE TABLE IF NOT EXISTS connection_log (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    token_id   TEXT NOT NULL,
    client_ip  TEXT NOT NULL,
    event      TEXT NOT NULL,
    details    TEXT,
    created_at TEXT NOT NULL
);
