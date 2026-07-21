#!/usr/bin/env bash
# Install grok-switch on Linux (amd64/arm64).
set -euo pipefail

REPO="${GROK_SWITCH_REPO:-Gelmezon/grok-switch}"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"

arch=$(uname -m)
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "不支持的架构: $arch" >&2; exit 1 ;;
esac

os=$(uname -s | tr '[:upper:]' '[:lower:]')
if [[ "$os" != "linux" ]]; then
  echo "此安装脚本仅支持 Linux。" >&2
  exit 1
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

asset="grok-switch-linux-${arch}"
if [[ "$VERSION" == "latest" ]]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
fi

echo "下载 ${url} ..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "${tmp}/grok-switch" "$url"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmp}/grok-switch" "$url"
else
  echo "需要 curl 或 wget" >&2
  exit 1
fi

chmod 755 "${tmp}/grok-switch"
if [[ -w "$INSTALL_DIR" ]]; then
  install -Dm755 "${tmp}/grok-switch" "${INSTALL_DIR}/grok-switch"
else
  sudo install -Dm755 "${tmp}/grok-switch" "${INSTALL_DIR}/grok-switch"
fi

echo "已安装到 ${INSTALL_DIR}/grok-switch"
"${INSTALL_DIR}/grok-switch" version
