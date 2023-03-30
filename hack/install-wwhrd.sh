#!/usr/bin/env bash
set -euo pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLATFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"

if [[ $# -ne 1 ]]; then
  echo "usage: bin/install-wwhrd.sh <version>"
  exit 1
fi
VERSION=$1
TARBALL="wwhrd_${VERSION}_$(uname)_amd64.tar.gz"

mkdir -p "$ROOT/bin/$PLATFORM"
curl -L "https://github.com/frapposelli/wwhrd/releases/download/v${VERSION}/${TARBALL}" | tar -xmz -C "$ROOT/bin/$PLATFORM" wwhrd
