#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(git rev-parse --show-toplevel)
WORK_DIR=$(mktemp -d)
cleanup() {
  rm -rf "$WORK_DIR"
}
trap "cleanup" EXIT SIGINT

VERSION=$1

TEST_DEPS_PATH=${2:-"$ROOT/bin/kubebuilder-tools"}

if [ -z "$VERSION" ];
then
  echo "usage: bin/install-kubebuilder-tools.sh <version>"
  exit 1
fi


OS=$(go env GOOS)
ARCH=$(go env GOARCH)

curl -L https://storage.googleapis.com/kubebuilder-tools/kubebuilder-tools-${VERSION}-${OS}-${ARCH}.tar.gz | tar -xz -C $WORK_DIR 

# move to repo_path/bin/kubebuilder - you'll need to set the KUBEBUILDER_ASSETS env var with
rm -rf ${TEST_DEPS_PATH}
mkdir -p ${TEST_DEPS_PATH}
mv $WORK_DIR/kubebuilder/bin/ ${TEST_DEPS_PATH}
