#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(git rev-parse --show-toplevel)
WORK_DIR=`mktemp -d`
cleanup() {
  rm -rf "$WORK_DIR"
}
trap "cleanup" EXIT SIGINT

VERSION=$1
BINARY="crd-to-markdown_$(uname)_x86_64"

if [ -z "$VERSION" ];
then
  echo "usage: bin/install-crd-to-markdown.sh <version>"
  exit 1
fi

cd $WORK_DIR
echo "https://github.com/clamoriniere/crd-to-markdown/releases/download/$VERSION/$BINARY"
curl -Lo ${BINARY} https://github.com/clamoriniere/crd-to-markdown/releases/download/$VERSION/$BINARY

chmod +x $BINARY
mkdir -p $ROOT/bin
mv $BINARY $ROOT/bin/crd-to-markdown
