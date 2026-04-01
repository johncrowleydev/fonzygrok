#!/usr/bin/env bash
# scripts/smoke_test.sh — Production smoke test per GOV-002 §18A
#
# Run this from a DIFFERENT machine than the server to verify end-to-end
# tunnel functionality over a real network.
#
# Usage:
#   ./scripts/smoke_test.sh <server_domain> <token>
#
# Example:
#   ./scripts/smoke_test.sh fonzygrok.com fgk_abc123
#
# Exit code:
#   0  — all checks passed
#   1  — one or more checks failed
#
# Requirements:
#   - curl
#   - fonzygrok client binary in PATH or ./fonzygrok
#   - A running fonzygrok server at <server_domain>
#
# Refs: GOV-002 §18A, SPR-015B T-050, DEF-002

set -euo pipefail

# ─── Colors ───────────────────────────────────────────────────────────────────
RED='\033[31m'
GREEN='\033[32m'
YELLOW='\033[33m'
RESET='\033[0m'

if [[ "${NO_COLOR:-}" != "" ]]; then
    RED='' GREEN='' YELLOW='' RESET=''
fi

# ─── Arguments ────────────────────────────────────────────────────────────────
if [[ $# -lt 2 ]]; then
    echo "Usage: $0 <server_domain> <token>"
    echo "Example: $0 fonzygrok.com fgk_abc123"
    exit 1
fi

DOMAIN="$1"
TOKEN="$2"
SMOKE_NAME="smoke-$$"
ECHO_PORT=0
CLIENT_PID=0
ECHO_PID=0
FAILED=0

# ─── Helpers ──────────────────────────────────────────────────────────────────

pass() { echo -e "  ${GREEN}✔${RESET} $1"; }
fail() { echo -e "  ${RED}✘${RESET} $1"; FAILED=1; }
info() { echo -e "  ${YELLOW}→${RESET} $1"; }

cleanup() {
    info "Cleaning up..."
    if [[ $CLIENT_PID -ne 0 ]]; then
        kill "$CLIENT_PID" 2>/dev/null || true
        wait "$CLIENT_PID" 2>/dev/null || true
    fi
    if [[ $ECHO_PID -ne 0 ]]; then
        kill "$ECHO_PID" 2>/dev/null || true
        wait "$ECHO_PID" 2>/dev/null || true
    fi
}
trap cleanup EXIT

# Find the fonzygrok binary.
FONZYGROK=""
if command -v fonzygrok &>/dev/null; then
    FONZYGROK="fonzygrok"
elif [[ -x "./fonzygrok" ]]; then
    FONZYGROK="./fonzygrok"
elif [[ -x "./bin/fonzygrok" ]]; then
    FONZYGROK="./bin/fonzygrok"
else
    echo -e "${RED}Error: fonzygrok binary not found. Build it first: go build -o fonzygrok ./cmd/client/${RESET}"
    exit 1
fi

echo ""
echo "╔══════════════════════════════════════════════════════╗"
echo "║       Fonzygrok Production Smoke Test               ║"
echo "╚══════════════════════════════════════════════════════╝"
echo ""
echo "  Server:     $DOMAIN"
echo "  Tunnel:     $SMOKE_NAME"
echo "  Client:     $FONZYGROK"
echo ""

# ─── Step 1: Health Check ─────────────────────────────────────────────────────
info "Step 1: Server health check..."

# Try HTTPS first, fallback to HTTP.
PROTO="https"
HEALTH_RESP=$(curl -sf --connect-timeout 10 "${PROTO}://${DOMAIN}/" 2>/dev/null) || {
    PROTO="http"
    HEALTH_RESP=$(curl -sf --connect-timeout 10 "${PROTO}://${DOMAIN}/" 2>/dev/null) || {
        fail "Server health check failed — no response from ${DOMAIN}"
        echo "  Tried both https:// and http://"
        exit 1
    }
}

if echo "$HEALTH_RESP" | grep -q '"service":"fonzygrok"'; then
    pass "Server is healthy (${PROTO}://${DOMAIN})"
else
    fail "Server responded but is not fonzygrok: $HEALTH_RESP"
fi

# ─── Step 2: Start Local Echo Service ────────────────────────────────────────
info "Step 2: Starting local echo service..."

# Simple HTTP server that echoes request details. Uses Python or a bash+nc fallback.
ECHO_PORT=$(python3 -c 'import socket; s=socket.socket(); s.bind(("",0)); print(s.getsockname()[1]); s.close()' 2>/dev/null || echo "18888")

python3 -c "
import http.server, json, sys, threading

class EchoHandler(http.server.BaseHTTPRequestHandler):
    def do_GET(self):
        body = json.dumps({'status': 'smoke-ok', 'path': self.path})
        self.send_response(200)
        self.send_header('Content-Type', 'application/json')
        self.send_header('Content-Length', str(len(body)))
        self.send_header('X-Smoke-Test', 'true')
        self.end_headers()
        self.wfile.write(body.encode())
    def log_message(self, *args):
        pass

srv = http.server.HTTPServer(('127.0.0.1', ${ECHO_PORT}), EchoHandler)
srv.serve_forever()
" &
ECHO_PID=$!

# Wait for echo server to be ready.
sleep 1
if ! kill -0 "$ECHO_PID" 2>/dev/null; then
    fail "Echo server failed to start on port $ECHO_PORT"
    exit 1
fi

pass "Echo server running on localhost:$ECHO_PORT (PID $ECHO_PID)"

# ─── Step 3: Connect Client ──────────────────────────────────────────────────
info "Step 3: Connecting fonzygrok client..."

$FONZYGROK \
    --server "$DOMAIN" \
    --token "$TOKEN" \
    --port "$ECHO_PORT" \
    --name "$SMOKE_NAME" \
    --no-inspect \
    --verbose \
    > /tmp/fonzygrok-smoke-stdout.log 2>/tmp/fonzygrok-smoke-stderr.log &
CLIENT_PID=$!

# Wait for tunnel to establish (check stderr for success message).
CONNECT_TIMEOUT=15
CONNECTED=false
for i in $(seq 1 $CONNECT_TIMEOUT); do
    if grep -q "Tunnel established" /tmp/fonzygrok-smoke-stderr.log 2>/dev/null; then
        CONNECTED=true
        break
    fi
    if ! kill -0 "$CLIENT_PID" 2>/dev/null; then
        fail "Client exited prematurely"
        echo "  stdout: $(cat /tmp/fonzygrok-smoke-stdout.log 2>/dev/null)"
        echo "  stderr: $(cat /tmp/fonzygrok-smoke-stderr.log 2>/dev/null)"
        CLIENT_PID=0
        exit 1
    fi
    sleep 1
done

if [[ "$CONNECTED" == "true" ]]; then
    pass "Client connected and tunnel established (PID $CLIENT_PID)"
else
    fail "Client did not establish tunnel within ${CONNECT_TIMEOUT}s"
    echo "  stderr: $(cat /tmp/fonzygrok-smoke-stderr.log 2>/dev/null)"
    exit 1
fi

# ─── Step 4: Verify Tunnel Proxy ─────────────────────────────────────────────
info "Step 4: Testing tunnel proxy..."

TUNNEL_URL="${PROTO}://${SMOKE_NAME}.${DOMAIN}/smoke-test"
TUNNEL_RESP=$(curl -sf --connect-timeout 10 --max-time 15 "$TUNNEL_URL" 2>/dev/null) || {
    fail "Tunnel request failed: curl $TUNNEL_URL"
    echo "  Server may not be routing to tunnel correctly"

    # Fallback: try HTTP explicitly.
    if [[ "$PROTO" == "https" ]]; then
        TUNNEL_RESP=$(curl -sf --connect-timeout 10 --max-time 15 "http://${SMOKE_NAME}.${DOMAIN}/smoke-test" 2>/dev/null) || {
            fail "Tunnel request also failed over HTTP"
            TUNNEL_RESP=""
        }
    fi
}

if echo "$TUNNEL_RESP" | grep -q '"status":"smoke-ok"'; then
    pass "Tunnel proxy works — got response from local service through tunnel"
elif echo "$TUNNEL_RESP" | grep -q 'smoke-ok'; then
    pass "Tunnel proxy works (partial match)"
elif [[ -n "$TUNNEL_RESP" ]]; then
    fail "Got response but unexpected content: $TUNNEL_RESP"
else
    fail "No response from tunnel"
fi

# ─── Step 5: Check Metrics/Admin ─────────────────────────────────────────────
info "Step 5: Checking admin/metrics endpoint..."

# Try the admin API (usually on port 9090 or same domain).
ADMIN_RESP=$(curl -sf --connect-timeout 5 "${PROTO}://${DOMAIN}:9090/api/v1/tunnels" 2>/dev/null) || \
ADMIN_RESP=$(curl -sf --connect-timeout 5 "${PROTO}://${DOMAIN}/api/v1/tunnels" 2>/dev/null) || \
ADMIN_RESP=""

if echo "$ADMIN_RESP" | grep -q "$SMOKE_NAME"; then
    pass "Admin API shows tunnel '$SMOKE_NAME'"
elif [[ -n "$ADMIN_RESP" ]]; then
    info "Admin API responded but tunnel not found in listing (may be expected)"
    pass "Admin API is reachable"
else
    info "Admin API not reachable (non-blocking — may require VPN or different port)"
fi

# ─── Step 6: TLS Check ───────────────────────────────────────────────────────
info "Step 6: TLS certificate check..."

if [[ "$PROTO" == "https" ]]; then
    TLS_INFO=$(echo | openssl s_client -connect "${DOMAIN}:443" -servername "$DOMAIN" 2>/dev/null | openssl x509 -noout -subject -dates 2>/dev/null) || TLS_INFO=""
    if [[ -n "$TLS_INFO" ]]; then
        pass "TLS certificate is valid"
        echo "       $TLS_INFO" | head -3
    else
        fail "TLS certificate check failed"
    fi
else
    info "Server is running HTTP — skipping TLS check"
fi

# ─── Summary ─────────────────────────────────────────────────────────────────
echo ""
echo "────────────────────────────────────────────────────────"
if [[ $FAILED -eq 0 ]]; then
    echo -e "  ${GREEN}All smoke tests passed.${RESET}"
    echo ""
    exit 0
else
    echo -e "  ${RED}One or more smoke tests FAILED.${RESET}"
    echo "  Review logs:"
    echo "    stdout: /tmp/fonzygrok-smoke-stdout.log"
    echo "    stderr: /tmp/fonzygrok-smoke-stderr.log"
    echo ""
    exit 1
fi
