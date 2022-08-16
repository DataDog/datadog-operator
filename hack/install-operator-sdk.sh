#!/usr/bin/env bash
set -euo pipefail

PLATFORM="$(uname -s)-$(uname -m)"
ROOT=$(git rev-parse --show-toplevel)

if [[ $# -ne 1 ]]; then
  echo "usage: bin/install-operator-sdk.sh <version>"
  exit 1
fi
RELEASE_VERSION=$1

# copy binary in current repo
uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    msys_nt) os="windows" ;;
  esac
  echo "$os"
}
OS=$(uname_os)

ARCH=$(go env GOARCH)

mkdir -p "$ROOT/bin/$PLATFORM"
curl -Lo "$ROOT/bin/$PLATFORM/operator-sdk" "https://github.com/operator-framework/operator-sdk/releases/download/${RELEASE_VERSION}/operator-sdk_${OS}_${ARCH}"
chmod +x "$ROOT/bin/$PLATFORM/operator-sdk"
