#!/bin/sh
set -eu

CONFIG=/app/config.yaml

echo "[control] waiting for postgres..."
until pg_isready -h postgres -U lingcdn -d lingcdn >/dev/null 2>&1; do sleep 2; done

echo "[control] migrate"
lingcdn-control migrate --config "$CONFIG" || true

echo "[control] serve"
exec lingcdn-control serve --config "$CONFIG"
