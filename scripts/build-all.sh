#!/bin/sh
# Builds static binaries (UI embedded) for macOS and Linux, arm64 and amd64, into dist/.
# Needs only Go (see go.mod). Run from anywhere: ./scripts/build-all.sh
set -eu
cd "$(dirname "$0")/.."
rm -rf dist && mkdir dist
for target in darwin/arm64 darwin/amd64 linux/arm64 linux/amd64; do
  os=${target%/*}; arch=${target#*/}
  echo "building dist/devbuddy-$os-$arch"
  CGO_ENABLED=0 GOOS=$os GOARCH=$arch go build -trimpath -ldflags="-s -w" -o "dist/devbuddy-$os-$arch" ./cmd/devbuddy
done
(cd dist && shasum -a 256 devbuddy-* > SHA256SUMS)
ls -lh dist
