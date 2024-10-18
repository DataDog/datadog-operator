#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLATFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"

WORK_DIR=`mktemp -d`
cleanup() {
  rm -rf "$WORK_DIR"
}
trap "cleanup" EXIT SIGINT

uname_arch() {
  arch=$(uname -m)
  case $arch in
    x86_64) arch="amd64" ;;
    x86) arch="386" ;;
    i686) arch="386" ;;
    i386) arch="386" ;;
    aarch64) arch="arm64" ;;
    armv5*) arch="armv5" ;;
    armv6*) arch="armv6" ;;
    armv7*) arch="armv7" ;;
  esac
  echo ${arch}
}

VERSION=$1

BIN_ARCH=$(uname_arch)
OS=$(uname| tr [:upper:] [:lower:])
if [ "$OS" == "darwin" ]; then
    OS="macos"
fi
BINARY="jq-$OS-$BIN_ARCH"

if [ -z "$VERSION" ];
then
  echo "usage: bin/install-jq.sh <version>"
  exit 1
fi

cd $WORK_DIR
# https://github.com/jqlang/jq/releases/download/jq-1.7.1/jq-linux-arm64
curl -Lo ${BINARY} https://github.com/jqlang/jq/releases/download/jq-$VERSION/$BINARY


chmod +x $BINARY
mkdir -p $ROOT/bin/$PLATFORM/
mv $BINARY $ROOT/bin/$PLATFORM/jq
