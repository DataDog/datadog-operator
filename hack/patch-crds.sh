#!/usr/bin/env bash

set -e

SCRIPT_DIR=$(dirname "${BASH_SOURCE:-0}")
YQ="$SCRIPT_DIR/../bin/yq"

# Remove "x-kubernetes-*" as only supported in Kubernetes 1.16+.
# Users of Kubernetes < 1.16 need to use v1beta1, others need to use v1
#
# Cannot use directly yq -d .. 'spec.validation.openAPIV3Schema.properties.**.x-kubernetes-*'
# as for some reason, yq takes several minutes to execute this command
for crd in $(ls "$SCRIPT_DIR/../deploy/crds/v1beta1")
do
  for path in $($YQ r "$SCRIPT_DIR/../deploy/crds/v1beta1/$crd" 'spec.validation.openAPIV3Schema.properties.**.x-kubernetes-*' --printMode p)
  do
    $YQ d -i "$SCRIPT_DIR/../deploy/crds/v1beta1/$crd" $path
  done
done

# Last step
# operator-sdk generate crds does not like symlinks
for crd in $(ls "$SCRIPT_DIR/../deploy/crds/v1")
do
  cp "$SCRIPT_DIR/../deploy/crds/v1beta1/$crd" "$SCRIPT_DIR/../deploy/crds/$crd"
done
