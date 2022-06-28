#!/usr/bin/env bash
set -euo pipefail

PLATFORM="$(uname -s)-$(uname -m)"
ROOT=$(git rev-parse --show-toplevel)

if [[ $# -ne 1 ]]; then
  echo "usage: bin/install-operator-manifest-tools.sh <version>"
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

mkdir -p "$ROOT/bin/$PLATFORM"
curl -Lo "$ROOT/bin/$PLATFORM/operator-manifest-tools" "https://github.com/operator-framework/operator-manifest-tools/releases/download/v${RELEASE_VERSION}/operator-manifest-tools_${RELEASE_VERSION}_${OS}_amd64"
chmod +x "$ROOT/bin/$PLATFORM/operator-manifest-tools"
