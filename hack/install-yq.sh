#!/usr/bin/env bash
set -euo pipefail

PLATFORM="$(uname -s)-$(uname -m)"
ROOT=$(git rev-parse --show-toplevel)
ARCH=$(go env GOARCH)
BINARY="yq_$(uname)_$ARCH"

if [[ $# -ne 1 ]]; then
  echo "usage: bin/install-yq.sh <version>"
  exit 1
fi
VERSION=$1

mkdir -p "$ROOT/bin/$PLATFORM"
curl -Lo "$ROOT/bin/$PLATFORM/yq" "https://github.com/mikefarah/yq/releases/download/$VERSION/$BINARY"
chmod +x "$ROOT/bin/$PLATFORM/yq"
