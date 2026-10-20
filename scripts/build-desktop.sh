#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
if [[ "$(uname -s)" != Linux ]]; then
  echo 'Build Linux packages on Linux. Use scripts/build-desktop.ps1 for Windows.' >&2
  exit 1
fi
command -v go >/dev/null
command -v npm >/dev/null
pkg-config --exists gtk+-3.0 webkit2gtk-4.1
if [[ ! -d node_modules ]]; then npm ci; fi
npm run build
mkdir -p cmd/opsbuddy-desktop/frontend release/linux
# Only replace the generated bundle, never application data.
rm -rf -- cmd/opsbuddy-desktop/frontend/assets
cp -R dist/. cmd/opsbuddy-desktop/frontend/
CGO_ENABLED=1 go build -trimpath -tags 'production,webkit2_41' -ldflags '-s -w' -o release/linux/opsbuddy ./cmd/opsbuddy-desktop
cp README.md README.en.md release/linux/
cp scripts/opsbuddy.desktop release/linux/
tar -czf release/OpsBuddy-Linux-amd64.tar.gz -C release/linux opsbuddy opsbuddy.desktop README.md README.en.md
echo 'Desktop: release/linux/opsbuddy'
echo 'Package: release/OpsBuddy-Linux-amd64.tar.gz'
