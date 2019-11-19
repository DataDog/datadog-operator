#!/usr/bin/env bash
set -e

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(git rev-parse --show-toplevel)
WORK_DIR=`mktemp -d`
cleanup() {
  rm -rf "$WORK_DIR"
}
trap "cleanup" EXIT SIGINT

export GOPATH=$WORK_DIR
export GO111MODULE="off"
go get github.com/frapposelli/wwhrd

mkdir -p $ROOT/bin
mv $GOPATH/bin/wwhrd $ROOT/bin/wwhrd
