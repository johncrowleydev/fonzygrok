CREATE TABLE IF NOT EXISTS tunnels (
    tunnel_id       TEXT PRIMARY KEY,
    subdomain       TEXT NOT NULL,
    protocol        TEXT NOT NULL,
    token_id        TEXT NOT NULL,
    client_ip       TEXT NOT NULL,
    local_port      INTEGER NOT NULL,
    connected_at    TIMESTAMPTZ NOT NULL,
    disconnected_at TIMESTAMPTZ,
    bytes_in        BIGINT NOT NULL DEFAULT 0,
    bytes_out       BIGINT NOT NULL DEFAULT 0,
    requests_proxied BIGINT NOT NULL DEFAULT 0
);
