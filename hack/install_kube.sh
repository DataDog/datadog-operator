#!/bin/bash
set -o errexit
set -o nounset
set -o pipefail

mkdir -p /usr/local/bin/

curl -Lo kubectl https://storage.googleapis.com/kubernetes-release/release/v1.14.1/bin/linux/amd64/kubectl && chmod +x kubectl && mv kubectl /usr/local/bin/
curl -Lo ./kind https://github.com/kubernetes-sigs/kind/releases/download/v0.5.1/kind-$(uname)-amd64  && chmod +x ./kind && mv ./kind /usr/local/bin/

kind create cluster --config test/cluster-kind-gitlabci.yaml

