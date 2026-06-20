#!/bin/sh
set -eu

CONTROL_HTTP="${CONTROL_HTTP:-http://control:8080}"
ADMIN_USER="${ADMIN_USER:-admin}"
ADMIN_PASS="${ADMIN_PASS:-admin123456}"

echo "[node] waiting for control at $CONTROL_HTTP ..."
until curl -sf "$CONTROL_HTTP/api/public/settings" >/dev/null; do sleep 2; done

if [ -z "${BOOTSTRAP_TOKEN:-}" ]; then
  echo "[node] requesting bootstrap token"
  LOGIN=$(curl -sf -X POST "$CONTROL_HTTP/api/auth/login" \
    -H "Content-Type: application/json" \
    -d "{\"identifier\":\"$ADMIN_USER\",\"password\":\"$ADMIN_PASS\"}")
  TOKEN=$(printf '%s' "$LOGIN" | sed -n 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
  if [ -z "$TOKEN" ]; then
    echo "[node] failed to obtain admin token" >&2
    exit 1
  fi
  BT=$(curl -sf -X POST "$CONTROL_HTTP/api/nodes/bootstrap-token" \
    -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"description":"docker node","ttl_minutes":120}')
  export BOOTSTRAP_TOKEN=$(printf '%s' "$BT" | sed -n 's/.*"token"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
fi

export CONTROL_ENDPOINT="${CONTROL_ENDPOINT:-http://control:9443}"
export CONTROL_TLS_ENABLED="${CONTROL_TLS_ENABLED:-false}"
export LISTEN_ADDR="${LISTEN_ADDR:-0.0.0.0:8088}"
export OPS_LISTEN_ADDR="${OPS_LISTEN_ADDR:-0.0.0.0:9101}"
export ACCESS_LOG_PATH="${ACCESS_LOG_PATH:-/var/log/lingcdn/access.log}"
export ERROR_LOG_PATH="${ERROR_LOG_PATH:-/var/log/lingcdn/error.log}"
export CACHE_DIR="${CACHE_DIR:-/var/lib/lingcdn-node/cache}"
export RUST_LOG="${RUST_LOG:-info}"

mkdir -p "$(dirname "$ACCESS_LOG_PATH")" "$(dirname "$ERROR_LOG_PATH")" "$CACHE_DIR"

echo "[node] starting with bootstrap token"
exec lingcdn-node
