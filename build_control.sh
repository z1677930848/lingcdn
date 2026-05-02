#!/usr/bin/env bash
set -euo pipefail

# ============================================================
#  LingCDN 主控构建打包脚本
#  将二进制 + 前端打包成 tar.gz，用于上传到授权站
#
#  用法: bash build_control.sh [--version 1.0.2] [--channel stable]
# ============================================================

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${SCRIPT_DIR}"

VERSION=""
CHANNEL="stable"
ARCH_LIST="amd64"
OUTPUT_DIR="${PROJECT_ROOT}/dist"

log() { echo -e "\033[36m[build]\033[0m $*"; }
ok()  { echo -e "\033[32m[build]\033[0m $*"; }
err() { echo -e "\033[31m[build][ERROR]\033[0m $*" >&2; }

while [[ $# -gt 0 ]]; do
  case "$1" in
    --version) VERSION="${2:-}"; shift 2 ;;
    --channel) CHANNEL="${2:-stable}"; shift 2 ;;
    --arch) ARCH_LIST="${2:-amd64}"; shift 2 ;;
    --output) OUTPUT_DIR="${2:-dist}"; shift 2 ;;
    -h|--help)
      echo "用法: bash build_control.sh [--version x.y.z] [--channel stable|beta] [--arch 'amd64 arm64']"
      exit 0
      ;;
    *) err "未知参数: $1"; exit 1 ;;
  esac
done

# 自动获取版本号
#
# 版本的真相源是 internal/buildinfo/buildinfo.go 里的 appVersion 常量。
# build_control.sh 同时做两件事：
#   1) 从 buildinfo.go 读出一个默认版本，用作 tar 包命名；
#   2) 用 -ldflags "-X .../buildinfo.appVersion=${VERSION}" 在链接时把版本
#      注入到二进制里，这样运行时不再依赖 /etc/lingcdn/lingcdn.env 或
#      APP_VERSION 环境变量——曾经的 "升级完成但 portal 仍显示旧版本" 正是
#      因为老脚本没做注入、运行时又错把 env 当成权威源。
if [[ -z "$VERSION" ]]; then
  BUILDINFO_FILE="${PROJECT_ROOT}/services/control/internal/buildinfo/buildinfo.go"
  if [[ -f "$BUILDINFO_FILE" ]]; then
    VERSION="$(grep -oE 'var appVersion = "[^"]+"' "$BUILDINFO_FILE" 2>/dev/null | grep -oE '[0-9]+\.[0-9]+\.[0-9]+[A-Za-z0-9.+-]*' | head -1 || true)"
  fi
  if [[ -z "$VERSION" ]]; then
    # Fallback (never the intended release path): stamp with today's date so
    # accidentally-published artifacts are at least distinguishable.
    VERSION="$(date +%Y%m%d)"
  fi
fi

log "版本: ${VERSION}"
log "通道: ${CHANNEL}"
log "架构: ${ARCH_LIST}"

# ---- 构建前端 ----
UI_DIR="${PROJECT_ROOT}/services/control/ui"
UI_DIST="${UI_DIR}/dist"

if [[ -f "${UI_DIR}/package.json" ]]; then
  log "正在构建前端..."
  cd "$UI_DIR"
  if [[ ! -d "node_modules" ]]; then
    npm install --silent 2>/dev/null || true
  fi
  npx vite build 2>/dev/null
  log "前端构建完成"
  cd "$PROJECT_ROOT"
else
  warn "未找到前端项目，跳过前端构建"
fi

# ---- 构建后端 + 打包 ----
mkdir -p "$OUTPUT_DIR"

for ARCH in $ARCH_LIST; do
  log "正在编译 linux/${ARCH}..."

  BIN_TMP="$(mktemp -d)"
  PACK_DIR="${BIN_TMP}/lingcdn-control-${VERSION}"
  mkdir -p "${PACK_DIR}"

  # 编译二进制
  #
  # -ldflags "-X .../buildinfo.appVersion=${VERSION}" 把真正的发版版本号
  # 写入 buildinfo.Version()。运行时代码只从这里读版本，不再读
  # APP_VERSION 环境变量——这是修复"脚本升级后 portal 仍显示旧版本"的
  # 关键：无论 /etc/lingcdn/lingcdn.env 里钉了什么老 APP_VERSION，
  # 新二进制都会如实上报自己的版本。
  # -s -w 去掉符号和 DWARF 调试信息，二进制更小（可选）。
  cd "${PROJECT_ROOT}/services/control"
  GOOS=linux GOARCH="$ARCH" CGO_ENABLED=0 go build \
    -ldflags "-s -w -X github.com/lingcdn/control/internal/buildinfo.appVersion=${VERSION}" \
    -o "${PACK_DIR}/lingcdn-control" \
    ./cmd/lingcdn-control/
  chmod +x "${PACK_DIR}/lingcdn-control"
  log "二进制编译完成: linux/${ARCH}"

  # 复制前端
  if [[ -d "$UI_DIST" && -f "${UI_DIST}/index.html" ]]; then
    cp -a "$UI_DIST" "${PACK_DIR}/ui"
    log "前端已打包"
  else
    log "警告: 未找到前端 dist，包内将不包含前端文件"
  fi

  cd "$PROJECT_ROOT"

  # 打包
  ARTIFACT_NAME="lingcdn-control-${VERSION}-linux-${ARCH}.tar.gz"
  ARTIFACT_PATH="${OUTPUT_DIR}/${ARTIFACT_NAME}"

  tar -czf "$ARTIFACT_PATH" -C "$BIN_TMP" "lingcdn-control-${VERSION}"
  rm -rf "$BIN_TMP"

  # 计算校验值
  if command -v sha256sum >/dev/null 2>&1; then
    CHECKSUM="$(sha256sum "$ARTIFACT_PATH" | awk '{print $1}')"
  elif command -v shasum >/dev/null 2>&1; then
    CHECKSUM="$(shasum -a 256 "$ARTIFACT_PATH" | awk '{print $1}')"
  else
    CHECKSUM="(无法计算)"
  fi

  ARTIFACT_SIZE="$(du -h "$ARTIFACT_PATH" | awk '{print $1}')"

  echo ""
  echo "  ┌─────────────────────────────────────"
  ok "  │ ${ARTIFACT_NAME}"
  echo "  │ 大小:   ${ARTIFACT_SIZE}"
  echo "  │ SHA256: ${CHECKSUM}"
  echo "  │ 路径:   ${ARTIFACT_PATH}"
  echo "  └─────────────────────────────────────"
  echo ""
done

ok "打包完成！请将 ${OUTPUT_DIR}/ 下的文件上传到授权站。"
