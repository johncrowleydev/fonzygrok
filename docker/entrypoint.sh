#!/bin/sh
# Entrypoint script for fonzygrok-server Docker container.
# Translates environment variables to CLI flags, with sensible defaults.
# Supports: domain, apex domain, TLS, TCP port range, rate limiting, database URL.

set -e

# Base command.
CMD="fonzygrok-server serve --data-dir=/data --ssh-addr=:2222 --http-addr=:8080 --admin-addr=0.0.0.0:9090 --domain=${DOMAIN:-tunnel.localhost}"

# Add database URL.
if [ -n "${DATABASE_URL}" ]; then
    CMD="${CMD} --database-url=${DATABASE_URL}"
fi

# Add apex domain if set.
if [ -n "${APEX_DOMAIN}" ]; then
    CMD="${CMD} --apex-domain=${APEX_DOMAIN}"
fi

# Add TLS flags if enabled.
if [ "${TLS_ENABLED}" = "true" ]; then
    CMD="${CMD} --tls --tls-cert-dir=/data/certs"
    echo "TLS enabled: cert-dir=/data/certs domain=${DOMAIN:-tunnel.localhost} apex-domain=${APEX_DOMAIN:-derived}"
fi

# Add TCP port range if set.
if [ -n "${TCP_PORT_RANGE}" ]; then
    CMD="${CMD} --tcp-port-range=${TCP_PORT_RANGE}"
fi

# Add rate limiting flags if set.
if [ -n "${RATE_LIMIT}" ]; then
    CMD="${CMD} --rate-limit=${RATE_LIMIT}"
fi
if [ -n "${RATE_BURST}" ]; then
    CMD="${CMD} --rate-burst=${RATE_BURST}"
fi

exec ${CMD}
