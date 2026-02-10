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

TEST_DEPS_PATH=${2:-"$ROOT/$INSTALL_PATH"}

if [ -z "$VERSION" ];
then
  echo "usage: bin/install-kubebuilder-tools.sh <version>"
  exit 1
fi


OS=$(go env GOOS)
ARCH=$(go env GOARCH)

curl -L https://github.com/kubernetes-sigs/controller-tools/releases/download/envtest-v${VERSION}/envtest-v${VERSION}-${OS}-${ARCH}.tar.gz | tar -xz -C $WORK_DIR

# move to repo_path/bin/kubebuilder - you'll need to set the KUBEBUILDER_ASSETS env var with
rm -rf ${TEST_DEPS_PATH}/etcd
rm -rf ${TEST_DEPS_PATH}/kube-apiserver
rm -rf ${TEST_DEPS_PATH}/kubectl
mkdir -p ${TEST_DEPS_PATH}
mv $WORK_DIR/controller-tools/envtest/etcd ${TEST_DEPS_PATH}/etcd
mv $WORK_DIR/controller-tools/envtest/kube-apiserver ${TEST_DEPS_PATH}/kube-apiserver
mv $WORK_DIR/controller-tools/envtest/kubectl ${TEST_DEPS_PATH}/kubectl
