#!/bin/sh
set -eu

ES_URL="${ES_URL:-http://elasticsearch:9200}"
LOG_PATH="${LOG_PATH:-/var/log/lingcdn/access.log}"
STATE="/tmp/access_log_shipper.offset"
BATCH=50

apk add --no-cache curl >/dev/null 2>&1 || true

echo "[shipper] waiting for ES at $ES_URL"
until curl -sf "$ES_URL/_cluster/health" >/dev/null; do sleep 3; done

index_for_ts() {
  # yyyy-MM-dd -> yyyy.MM.dd
  day=$(printf '%s' "$1" | cut -c1-10 | tr '-' '.')
  echo "cdn-access-$day"
}

ship_batch() {
  batch="$1"
  [ -n "$batch" ] || return 0
  body=""
  while IFS= read -r line; do
    [ -n "$line" ] || continue
    ts=$(printf '%s' "$line" | sed -n 's/.*"@timestamp"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p')
    [ -n "$ts" ] || ts=$(date -u +%Y-%m-%dT%H:%M:%SZ)
    idx=$(index_for_ts "$ts")
    body="${body}{\"index\":{\"_index\":\"${idx}\"}}"$'\n'"${line}"$'\n'
  done <<EOF
$batch
EOF
  [ -n "$body" ] || return 0
  resp=$(curl -sf -X POST "$ES_URL/_bulk" -H "Content-Type: application/x-ndjson" --data-binary "$body")
  if printf '%s' "$resp" | grep -q '"errors":true'; then
    echo "[shipper] bulk errors: $resp" >&2
  fi
}

offset=0
if [ -f "$STATE" ]; then
  offset=$(cat "$STATE")
fi

echo "[shipper] tailing $LOG_PATH (offset=$offset)"
while true; do
  if [ ! -f "$LOG_PATH" ]; then
    sleep 3
    continue
  fi
  tmp=$(mktemp)
  tail -c +$((offset + 1)) "$LOG_PATH" > "$tmp" 2>/dev/null || true
  lines=""
  count=0
  while IFS= read -r line; do
    lines="${lines}${line}"$'\n'
    count=$((count + 1))
    if [ "$count" -ge "$BATCH" ]; then
      ship_batch "$lines"
      lines=""
      count=0
    fi
  done < "$tmp"
  if [ -n "$lines" ]; then
    ship_batch "$lines"
  fi
  offset=$(wc -c < "$LOG_PATH" | tr -d ' ')
  printf '%s' "$offset" > "$STATE"
  rm -f "$tmp"
  sleep 5
done
