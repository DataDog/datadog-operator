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
INSTALL_PATH=$2

if [ -z "$VERSION" ];
then
  echo "usage: bin/install-kubebuilder.sh <version>"
  exit 1
fi

os=$(go env GOOS)
arch=$(go env GOARCH)

# download kubebuilder and extract it to tmp
rm -rf "$ROOT/$INSTALL_PATH/kubebuilder"
curl -L https://github.com/kubernetes-sigs/kubebuilder/releases/download/v${VERSION}/kubebuilder_${os}_${arch} --output $ROOT/$INSTALL_PATH/kubebuilder

chmod +x $ROOT/$INSTALL_PATH/kubebuilder
