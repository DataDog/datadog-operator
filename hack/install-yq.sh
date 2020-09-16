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
BINARY="yq_$(uname)_amd64"

if [ -z "$VERSION" ];
then
  echo "usage: bin/install-yq.sh <version>"
  exit 1
fi

cd $WORK_DIR
curl -Lo ${BINARY} https://github.com/mikefarah/yq/releases/download/$VERSION/$BINARY

chmod +x $BINARY
mkdir -p $ROOT/bin
mv $BINARY $ROOT/bin/yq
