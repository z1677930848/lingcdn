#!/usr/bin/env bash
# LingCDN offline node installer — downloads artifacts from the control plane
# (LICENSE_MODE=offline). Do not use when online portal authorization is active.
set -euo pipefail

MASTER_HOST=""
MASTER_TOKEN=""
MASTER_VERSION="latest"
CONTROL_BASE=""
UPGRADE_PUBKEY=""
UPGRADE_CHANNEL="stable"
LISTEN_ADDR="0.0.0.0:80"
OPS_LISTEN_ADDR="127.0.0.1:9101"
CACHE_DIR="/var/lib/lingcdn/cache"
BIN_DIR="/usr/local/bin"
SERVICE_NAME="lingcdn-node"
ES_HOST=""
ES_PORT="9200"
ES_USER=""
ES_PASS=""
ES_PROTOCOL="http"
ES_INDEX_PREFIX="cdn-access"
ES_SSL_VERIFY="false"
ACCESS_LOG_PATH="/var/log/lingcdn/access.log"
FB_MAJOR="8.x"
SKIP_FILEBEAT="false"

log() { echo "[node-install-local] $*"; }
err() { echo "[node-install-local][ERROR] $*" >&2; }

usage() {
  cat <<'EOF'
Usage:
  bash node_install_local.sh \
    --master_host <ip:grpc_port> \
    --master_token <bootstrap-token> \
    --control_base <http://control:9080> \
    [--master_version <latest|x.y.z>] \
    [--upgrade_channel <stable|beta>] \
    [--upgrade_pubkey <base64-ed25519-pubkey>]
EOF
}

require_cmd() {
  command -v "$1" >/dev/null 2>&1 || { err "missing command: $1"; exit 1; }
}

url_origin() {
  local url="$1"
  [[ "$url" =~ ^https?:// ]] || return 1
  local scheme authority
  scheme="$(echo "$url" | sed -E 's#^(https?)://.*#\1#' | tr '[:upper:]' '[:lower:]')"
  authority="$(echo "$url" | sed -E 's#^https?://([^/]+).*$#\1#' | tr '[:upper:]' '[:lower:]')"
  echo "${scheme}://${authority}"
}

json_get() {
  local json="$1" key="$2"
  if command -v jq >/dev/null 2>&1; then
    echo "$json" | jq -r --arg k "$key" '.[$k] // empty'
    return 0
  fi
  python3 - "$key" "$json" <<'PY'
import json, sys
obj = json.loads(sys.argv[2])
v = obj.get(sys.argv[1], "")
print("" if v is None else str(v))
PY
}

sha256_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$file" | awk '{print $1}'
  else
    shasum -a 256 "$file" | awk '{print $1}'
  fi
}

extract_from_tar() {
  local artifact="$1" out="$2" candidate
  candidate="$(tar -tf "$artifact" | grep -E '(^|/)(lingcdn-node)$' | head -n 1 || true)"
  [[ -n "$candidate" ]] || return 1
  tar -xf "$artifact" -C "$out" "$candidate"
  local extracted="$out/$candidate"
  if [[ "$extracted" != "$out/lingcdn-node" || -d "$out/lingcdn-node" ]]; then
    mv -f "$extracted" "$out/lingcdn-node.tmp"
    rm -rf "$out/lingcdn-node"
    mv -f "$out/lingcdn-node.tmp" "$out/lingcdn-node"
  fi
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    --master_host) MASTER_HOST="${2:-}"; shift 2 ;;
    --master_token) MASTER_TOKEN="${2:-}"; shift 2 ;;
    --master_version) MASTER_VERSION="${2:-}"; shift 2 ;;
    --control_base) CONTROL_BASE="${2:-}"; shift 2 ;;
    --upgrade_pubkey) UPGRADE_PUBKEY="${2:-}"; shift 2 ;;
    --upgrade_channel) UPGRADE_CHANNEL="${2:-}"; shift 2 ;;
    --listen_addr) LISTEN_ADDR="${2:-}"; shift 2 ;;
    --ops_listen_addr) OPS_LISTEN_ADDR="${2:-}"; shift 2 ;;
    --cache_dir) CACHE_DIR="${2:-}"; shift 2 ;;
    --bin_dir) BIN_DIR="${2:-}"; shift 2 ;;
    --service_name) SERVICE_NAME="${2:-}"; shift 2 ;;
    --es_host) ES_HOST="${2:-}"; shift 2 ;;
    --es_port) ES_PORT="${2:-}"; shift 2 ;;
    --es_user) ES_USER="${2:-}"; shift 2 ;;
    --es_pass) ES_PASS="${2:-}"; shift 2 ;;
    --es_protocol) ES_PROTOCOL="${2:-}"; shift 2 ;;
    --es_index_prefix) ES_INDEX_PREFIX="${2:-}"; shift 2 ;;
    --es_ssl_verify) ES_SSL_VERIFY="${2:-}"; shift 2 ;;
    --access_log_path) ACCESS_LOG_PATH="${2:-}"; shift 2 ;;
    --fb_major) FB_MAJOR="${2:-}"; shift 2 ;;
    --skip_filebeat) SKIP_FILEBEAT="true"; shift ;;
    -h|--help) usage; exit 0 ;;
    *) err "unknown argument: $1"; usage; exit 1 ;;
  esac
done

if [[ -z "$MASTER_HOST" || -z "$MASTER_TOKEN" || -z "$CONTROL_BASE" ]]; then
  err "--master_host, --master_token and --control_base are required"
  usage
  exit 1
fi

require_cmd curl
require_cmd file
require_cmd install
require_cmd systemctl
command -v tar >/dev/null 2>&1 || command -v unzip >/dev/null 2>&1 || { err "need tar or unzip"; exit 1; }

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) err "unsupported arch: $ARCH"; exit 1 ;;
esac

if [[ "$MASTER_HOST" == http://* || "$MASTER_HOST" == https://* ]]; then
  CONTROL_ENDPOINT="$MASTER_HOST"
else
  CONTROL_ENDPOINT="http://$MASTER_HOST"
fi

CONTROL_BASE="${CONTROL_BASE%/}"
LATEST_URL="${CONTROL_BASE}/api/local/upgrade/latest?product=node&version=${MASTER_VERSION}&channel=${UPGRADE_CHANNEL}&platform=linux&arch=${ARCH}"
log "fetching build metadata: $LATEST_URL"
META_JSON="$(curl -fsSL "$LATEST_URL")"

DOWNLOAD_URL="$(json_get "$META_JSON" "download_url")"
TARGET_VERSION="$(json_get "$META_JSON" "version")"
CHECKSUM="$(json_get "$META_JSON" "checksum")"

[[ -n "$DOWNLOAD_URL" ]] || { err "download_url is empty"; exit 1; }
[[ -n "$CHECKSUM" ]] || { err "checksum is empty"; exit 1; }

CONTROL_ORIGIN="$(url_origin "$CONTROL_BASE" || true)"
DOWNLOAD_ORIGIN="$(url_origin "$DOWNLOAD_URL" || true)"
[[ -n "$CONTROL_ORIGIN" && -n "$DOWNLOAD_ORIGIN" ]] || { err "invalid control or download URL"; exit 1; }
if [[ "$DOWNLOAD_ORIGIN" != "$CONTROL_ORIGIN" ]]; then
  err "untrusted download_url origin: expected $CONTROL_ORIGIN, got $DOWNLOAD_ORIGIN"
  exit 1
fi

TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

ARTIFACT="$TMP_DIR/node.artifact"
BIN_TMP="$TMP_DIR/bin"
mkdir -p "$BIN_TMP"

log "downloading artifact: $DOWNLOAD_URL"
curl -fsSL -o "$ARTIFACT" "$DOWNLOAD_URL"

ACTUAL_CHECKSUM="$(sha256_file "$ARTIFACT" | tr '[:upper:]' '[:lower:]')"
EXPECT_CHECKSUM="$(echo "$CHECKSUM" | tr '[:upper:]' '[:lower:]')"
if [[ "$ACTUAL_CHECKSUM" != "$EXPECT_CHECKSUM" ]]; then
  err "checksum mismatch: expected=$EXPECT_CHECKSUM actual=$ACTUAL_CHECKSUM"
  exit 1
fi
log "checksum verified"

if tar -tf "$ARTIFACT" >/dev/null 2>&1; then
  extract_from_tar "$ARTIFACT" "$BIN_TMP" || true
fi
if [[ ! -f "$BIN_TMP/lingcdn-node" ]] && file "$ARTIFACT" | grep -qiE 'ELF|executable'; then
  cp "$ARTIFACT" "$BIN_TMP/lingcdn-node"
fi
[[ -f "$BIN_TMP/lingcdn-node" ]] || { err "failed to locate lingcdn-node in artifact"; exit 1; }

chmod +x "$BIN_TMP/lingcdn-node"
id -u lingcdn >/dev/null 2>&1 || useradd -r -s /usr/sbin/nologin -d /var/lib/lingcdn-node lingcdn
install -d -m 0755 /etc/lingcdn /var/lib/lingcdn-node "$CACHE_DIR" /var/log/lingcdn
install -m 0755 "$BIN_TMP/lingcdn-node" "$BIN_DIR/lingcdn-node"
chown -R lingcdn:lingcdn /var/lib/lingcdn-node "$CACHE_DIR" /var/log/lingcdn

cat > /etc/lingcdn/node.env <<EOF
CONTROL_ENDPOINT=${CONTROL_ENDPOINT}
BOOTSTRAP_TOKEN=${MASTER_TOKEN}
NODE_ID=
NODE_VERSION=${TARGET_VERSION:-$MASTER_VERSION}
NODE_CAPABILITIES=http,https,cache
LISTEN_ADDR=${LISTEN_ADDR}
OPS_LISTEN_ADDR=${OPS_LISTEN_ADDR}
CACHE_DIR=${CACHE_DIR}
UPGRADE_CHANNEL=${UPGRADE_CHANNEL}
NODE_ENV_FILE=/etc/lingcdn/node.env
EOF
[[ -n "$UPGRADE_PUBKEY" ]] && echo "UPGRADE_PUBKEY=${UPGRADE_PUBKEY}" >> /etc/lingcdn/node.env
chown lingcdn:lingcdn /etc/lingcdn/node.env
chmod 0600 /etc/lingcdn/node.env

cat > "/etc/systemd/system/${SERVICE_NAME}.service" <<EOF
[Unit]
Description=LingCDN Node
After=network-online.target
Wants=network-online.target

[Service]
User=root
Group=root
EnvironmentFile=/etc/lingcdn/node.env
WorkingDirectory=/var/lib/lingcdn-node
ExecStart=${BIN_DIR}/lingcdn-node
Restart=always
RestartSec=3
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable "$SERVICE_NAME"
systemctl restart "$SERVICE_NAME"

log "installed: ${BIN_DIR}/lingcdn-node version=${TARGET_VERSION:-unknown}"
systemctl is-active "$SERVICE_NAME" || true
