#!/bin/sh
set -eu

REPO="${REPO:-haidaralif-24/cc-router}"
VERSION="${VERSION:-latest}"
BIN_DIR="${BIN_DIR:-/usr/local/bin}"

os="$(uname -s)"
arch="$(uname -m)"

case "$os" in
  Linux) os="linux" ;;
  Darwin) os="darwin" ;;
  *) echo "Unsupported OS: ${os}" >&2; exit 1 ;;
esac

case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  aarch64|arm64) arch="arm64" ;;
  *) echo "Unsupported architecture: ${arch}" >&2; exit 1 ;;
esac

asset="claude-router_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/${VERSION}/download/${asset}"

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

curl -fsSL "$url" -o "$tmpdir/claude-router.tar.gz"
tar -xzf "$tmpdir/claude-router.tar.gz" -C "$tmpdir"
cp "$tmpdir/claude-router" "${BIN_DIR}/claude-router"
chmod 0755 "${BIN_DIR}/claude-router"

echo "Installed claude-router to ${BIN_DIR}/claude-router"
exec "${BIN_DIR}/claude-router"
