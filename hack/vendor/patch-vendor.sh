#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(git rev-parse --show-toplevel)

# Patching due to https://github.com/kubernetes-sigs/kustomize/issues/3262
# It's not possible to use a go replace directive to solve this issue
cp "$ROOT/hack/vendor/factorycrd.gopatch" "$ROOT/vendor/sigs.k8s.io/kustomize/pkg/transformers/config/factorycrd.go"
