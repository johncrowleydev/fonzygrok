#!/bin/sh
# Entrypoint script that conditionally enables TLS based on TLS_ENABLED env var.
# Used by docker-compose.yml to support both local dev (no TLS) and production (TLS).

set -e

# Base command.
CMD="fonzygrok-server serve --data-dir=/data --ssh-addr=:2222 --http-addr=:8080 --admin-addr=0.0.0.0:9090 --domain=${DOMAIN:-tunnel.localhost}"

# Add TLS flags if enabled.
if [ "${TLS_ENABLED}" = "true" ]; then
    CMD="${CMD} --tls --tls-cert-dir=/data/certs"
    echo "TLS enabled: cert-dir=/data/certs domain=${DOMAIN:-tunnel.localhost}"
fi

exec ${CMD}
