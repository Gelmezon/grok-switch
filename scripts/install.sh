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
  sums_url="https://github.com/${REPO}/releases/latest/download/SHA256SUMS"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
  sums_url="https://github.com/${REPO}/releases/download/${VERSION}/SHA256SUMS"
fi

echo "下载 ${url} ..."
if command -v curl >/dev/null 2>&1; then
  curl -fsSL -o "${tmp}/grok-switch" "$url"
  curl -fsSL -o "${tmp}/SHA256SUMS" "$sums_url"
elif command -v wget >/dev/null 2>&1; then
  wget -qO "${tmp}/grok-switch" "$url"
  wget -qO "${tmp}/SHA256SUMS" "$sums_url"
else
  echo "需要 curl 或 wget" >&2
  exit 1
fi

if ! command -v sha256sum >/dev/null 2>&1; then
  echo "需要 sha256sum 校验下载文件" >&2
  exit 1
fi
expected=$(awk -v name="$asset" '$2 == name { print $1; exit }' "${tmp}/SHA256SUMS")
if [[ ! "$expected" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "SHA256SUMS 中找不到 ${asset} 的有效校验值" >&2
  exit 1
fi
actual=$(sha256sum "${tmp}/grok-switch" | awk '{ print $1 }')
if [[ "$actual" != "$expected" ]]; then
  echo "SHA-256 校验失败" >&2
  exit 1
fi
echo "SHA-256 校验通过。"

chmod 755 "${tmp}/grok-switch"
if [[ -w "$INSTALL_DIR" ]]; then
  install -Dm755 "${tmp}/grok-switch" "${INSTALL_DIR}/grok-switch"
else
  sudo install -Dm755 "${tmp}/grok-switch" "${INSTALL_DIR}/grok-switch"
fi

echo "已安装到 ${INSTALL_DIR}/grok-switch"
"${INSTALL_DIR}/grok-switch" version
