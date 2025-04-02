#!/usr/bin/env bash
set -euo pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLATFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"


if [[ $# -ne 1 ]]; then
  echo "usage: bin/install-openshift-preflight.sh <version>"
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

if [ "$OS" != "linux" ];
then
  echo "this tool is not available on OS: $OS"
  exit 1
fi

mkdir -p "$ROOT/bin/$PLATFORM"

if [ "$RELEASE_VERSION" == "latest" ];
then
  echo "Downloading latest preflight version"
  curl -Lo "$ROOT/bin/$PLATFORM/preflight" "https://github.com/redhat-openshift-ecosystem/openshift-preflight/releases/latest/download/preflight-${OS}-amd64"
else
  curl -Lo "$ROOT/bin/$PLATFORM/preflight" "https://github.com/redhat-openshift-ecosystem/openshift-preflight/releases/download/${RELEASE_VERSION}/preflight-${OS}-amd64"
fi

chmod +x "$ROOT/bin/$PLATFORM/preflight"
