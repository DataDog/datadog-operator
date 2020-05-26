#!/usr/bin/env bash
set -e

RELEASE_VERSION=v0.17.0

ROOT=$(pwd)

# copy binary in current repo
mkdir -p $ROOT/bin

WORK_DIR=`mktemp -d`

# deletes the temp directory
function cleanup {      
  rm -rf "$WORK_DIR"
  echo "Deleted temp working directory $WORK_DIR"
}

# register the cleanup function to be called on the EXIT signal
trap cleanup EXIT

uname_os() {
  os=$(uname -s | tr '[:upper:]' '[:lower:]')
  case "$os" in
    msys_nt) os="windows" ;;
  esac
  echo "$os"
}

OS=$(uname_os)

mkdir -p bin

cd $WORK_DIR
if [ "$OS" == "darwin" ]; then
    echo "darwin"
    curl -OJL https://github.com/operator-framework/operator-sdk/releases/download/${RELEASE_VERSION}/operator-sdk-${RELEASE_VERSION}-x86_64-apple-darwin
    mv operator-sdk-${RELEASE_VERSION}-x86_64-apple-darwin $ROOT/bin/operator-sdk
else
    echo "linux"
    curl -OJL https://github.com/operator-framework/operator-sdk/releases/download/${RELEASE_VERSION}/operator-sdk-${RELEASE_VERSION}-x86_64-linux-gnu
    mv operator-sdk-${RELEASE_VERSION}-x86_64-linux-gnu $ROOT/bin/operator-sdk
fi

chmod +x $ROOT/bin/operator-sdk
