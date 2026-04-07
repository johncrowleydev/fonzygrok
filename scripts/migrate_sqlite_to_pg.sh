#!/bin/bash
# migrate_sqlite_to_pg.sh — Migrates data from SQLite to PostgreSQL.
#
# Usage:
#   ./migrate_sqlite_to_pg.sh <sqlite_db_path> <pg_connection_string>
#
# Example:
#   ./migrate_sqlite_to_pg.sh /data/fonzygrok.db "postgres://fonzygrok:fonzygrok@localhost:5432/fonzygrok?sslmode=disable"
#
# Prerequisites:
#   - sqlite3 CLI installed
#   - psql CLI installed
#   - PostgreSQL database must exist and have migrations applied

set -euo pipefail

SQLITE_DB="${1:?Usage: $0 <sqlite_db_path> <pg_connection_string>}"
PG_CONN="${2:?Usage: $0 <sqlite_db_path> <pg_connection_string>}"

if [ ! -f "$SQLITE_DB" ]; then
    echo "❌ SQLite database not found: $SQLITE_DB"
    exit 1
fi

if ! command -v sqlite3 &>/dev/null; then
    echo "❌ sqlite3 CLI not found. Install it: apt install sqlite3"
    exit 1
fi

if ! command -v psql &>/dev/null; then
    echo "❌ psql CLI not found. Install it: apt install postgresql-client"
    exit 1
fi

echo "=== SQLite → PostgreSQL Migration ==="
echo "Source: $SQLITE_DB"
echo "Target: ${PG_CONN%%\?*}"
echo ""

# Verify PG connection.
if ! psql "$PG_CONN" -c "SELECT 1" &>/dev/null; then
    echo "❌ Cannot connect to PostgreSQL. Check connection string."
    exit 1
fi
echo "✅ PostgreSQL connection OK"

# Count source records.
echo ""
echo "Source records:"
for table in users tokens tunnels connection_log invite_codes; do
    count=$(sqlite3 "$SQLITE_DB" "SELECT COUNT(*) FROM $table;" 2>/dev/null || echo "0")
    echo "  $table: $count"
done

echo ""
echo "Migrating..."

# --- Users ---
echo -n "  users: "
sqlite3 -csv "$SQLITE_DB" "SELECT id, username, email, password_hash, role, created_at, last_login_at, is_active FROM users;" | \
while IFS=, read -r id username email password_hash role created_at last_login_at is_active; do
    # Strip CSV quotes.
    id=$(echo "$id" | tr -d '"')
    username=$(echo "$username" | tr -d '"')
    email=$(echo "$email" | tr -d '"')
    password_hash=$(echo "$password_hash" | tr -d '"')
    role=$(echo "$role" | tr -d '"')
    created_at=$(echo "$created_at" | tr -d '"')
    last_login_at=$(echo "$last_login_at" | tr -d '"')
    is_active=$(echo "$is_active" | tr -d '"')

    # Convert SQLite integer booleans to PG booleans.
    [ "$is_active" = "1" ] && is_active="true" || is_active="false"
    # Handle NULL last_login_at.
    if [ -z "$last_login_at" ]; then
        last_login_at="NULL"
    else
        last_login_at="'$last_login_at'"
    fi

    psql "$PG_CONN" -q -c \
        "INSERT INTO users (id, username, email, password_hash, role, created_at, last_login_at, is_active)
         VALUES ('$id', '$username', '$email', '$password_hash', '$role', '$created_at', $last_login_at, $is_active)
         ON CONFLICT (id) DO NOTHING;" 2>/dev/null
done
user_count=$(psql "$PG_CONN" -t -c "SELECT COUNT(*) FROM users;" | tr -d ' ')
echo "$user_count migrated"

# --- Tokens ---
echo -n "  tokens: "
sqlite3 -csv "$SQLITE_DB" "SELECT id, name, token_hash, user_id, created_at, last_used_at, is_active FROM tokens;" | \
while IFS=, read -r id name token_hash user_id created_at last_used_at is_active; do
    id=$(echo "$id" | tr -d '"')
    name=$(echo "$name" | tr -d '"')
    token_hash=$(echo "$token_hash" | tr -d '"')
    user_id=$(echo "$user_id" | tr -d '"')
    created_at=$(echo "$created_at" | tr -d '"')
    last_used_at=$(echo "$last_used_at" | tr -d '"')
    is_active=$(echo "$is_active" | tr -d '"')

    [ "$is_active" = "1" ] && is_active="true" || is_active="false"
    [ -z "$user_id" ] && user_id="NULL" || user_id="'$user_id'"
    [ -z "$last_used_at" ] && last_used_at="NULL" || last_used_at="'$last_used_at'"

    psql "$PG_CONN" -q -c \
        "INSERT INTO tokens (id, name, token_hash, user_id, created_at, last_used_at, is_active)
         VALUES ('$id', '$name', '$token_hash', $user_id, '$created_at', $last_used_at, $is_active)
         ON CONFLICT (id) DO NOTHING;" 2>/dev/null
done
token_count=$(psql "$PG_CONN" -t -c "SELECT COUNT(*) FROM tokens;" | tr -d ' ')
echo "$token_count migrated"

# --- Invite codes ---
echo -n "  invite_codes: "
sqlite3 -csv "$SQLITE_DB" "SELECT id, code, created_by, used_by, used_at, created_at, is_active FROM invite_codes;" 2>/dev/null | \
while IFS=, read -r id code created_by used_by used_at created_at is_active; do
    id=$(echo "$id" | tr -d '"')
    code=$(echo "$code" | tr -d '"')
    created_by=$(echo "$created_by" | tr -d '"')
    used_by=$(echo "$used_by" | tr -d '"')
    used_at=$(echo "$used_at" | tr -d '"')
    created_at=$(echo "$created_at" | tr -d '"')
    is_active=$(echo "$is_active" | tr -d '"')

    [ "$is_active" = "1" ] && is_active="true" || is_active="false"
    [ -z "$used_by" ] && used_by="NULL" || used_by="'$used_by'"
    [ -z "$used_at" ] && used_at="NULL" || used_at="'$used_at'"

    psql "$PG_CONN" -q -c \
        "INSERT INTO invite_codes (id, code, created_by, used_by, used_at, created_at, is_active)
         VALUES ('$id', '$code', '$created_by', $used_by, $used_at, '$created_at', $is_active)
         ON CONFLICT (id) DO NOTHING;" 2>/dev/null
done
invite_count=$(psql "$PG_CONN" -t -c "SELECT COUNT(*) FROM invite_codes;" | tr -d ' ')
echo "$invite_count migrated"

# --- Tunnels (historical) ---
echo -n "  tunnels: "
sqlite3 -csv "$SQLITE_DB" "SELECT tunnel_id, subdomain, protocol, token_id, client_ip, local_port, connected_at, disconnected_at, bytes_in, bytes_out, requests_proxied FROM tunnels;" 2>/dev/null | \
while IFS=, read -r tunnel_id subdomain protocol token_id client_ip local_port connected_at disconnected_at bytes_in bytes_out requests_proxied; do
    tunnel_id=$(echo "$tunnel_id" | tr -d '"')
    subdomain=$(echo "$subdomain" | tr -d '"')
    protocol=$(echo "$protocol" | tr -d '"')
    token_id=$(echo "$token_id" | tr -d '"')
    client_ip=$(echo "$client_ip" | tr -d '"')
    local_port=$(echo "$local_port" | tr -d '"')
    connected_at=$(echo "$connected_at" | tr -d '"')
    disconnected_at=$(echo "$disconnected_at" | tr -d '"')
    bytes_in=$(echo "$bytes_in" | tr -d '"')
    bytes_out=$(echo "$bytes_out" | tr -d '"')
    requests_proxied=$(echo "$requests_proxied" | tr -d '"')

    [ -z "$disconnected_at" ] && disconnected_at="NULL" || disconnected_at="'$disconnected_at'"

    psql "$PG_CONN" -q -c \
        "INSERT INTO tunnels (tunnel_id, subdomain, protocol, token_id, client_ip, local_port, connected_at, disconnected_at, bytes_in, bytes_out, requests_proxied)
         VALUES ('$tunnel_id', '$subdomain', '$protocol', '$token_id', '$client_ip', $local_port, '$connected_at', $disconnected_at, $bytes_in, $bytes_out, $requests_proxied)
         ON CONFLICT (tunnel_id) DO NOTHING;" 2>/dev/null
done
tunnel_count=$(psql "$PG_CONN" -t -c "SELECT COUNT(*) FROM tunnels;" | tr -d ' ')
echo "$tunnel_count migrated"

echo ""
echo "=== Migration complete ==="
echo ""
echo "Verify with:"
echo "  psql \"$PG_CONN\" -c \"SELECT id, username, email, role FROM users;\""
