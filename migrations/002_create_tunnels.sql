CREATE TABLE IF NOT EXISTS tunnels (
    tunnel_id       TEXT PRIMARY KEY,
    subdomain       TEXT NOT NULL,
    protocol        TEXT NOT NULL,
    token_id        TEXT NOT NULL,
    client_ip       TEXT NOT NULL,
    local_port      INTEGER NOT NULL,
    connected_at    TEXT NOT NULL,
    disconnected_at TEXT,
    bytes_in        INTEGER NOT NULL DEFAULT 0,
    bytes_out       INTEGER NOT NULL DEFAULT 0,
    requests_proxied INTEGER NOT NULL DEFAULT 0
);
