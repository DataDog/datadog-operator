#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR=$(git rev-parse --show-toplevel)
YQ="$ROOT_DIR/bin/$(uname -s)-$(uname -m)/yq"

v1beta1=config/crd/bases/v1beta1
v1=config/crd/bases/v1

# Remove "x-kubernetes-*" as only supported in Kubernetes 1.16+.
# Users of Kubernetes < 1.16 need to use v1beta1, others need to use v1
#
# Cannot use directly yq -d .. 'spec.validation.openAPIV3Schema.properties.**.x-kubernetes-*'
# as for some reason, yq takes several minutes to execute this command
for crd in "$ROOT_DIR/$v1beta1"/*.yaml
do
  for path in $($YQ r "$crd" 'spec.validation.openAPIV3Schema.properties.**.x-kubernetes-*' --printMode p)
  do
    $YQ d -i "$crd" $path
  done
done

# Remove defaultOverride section in DatadogAgent status due to the error: "datadoghq.com_datadogagents.yaml bigger than total allowed limit"
$YQ d -i "$ROOT_DIR/$v1beta1/datadoghq.com_datadogagents.yaml" 'spec.validation.openAPIV3Schema.properties.status.properties.defaultOverride'
$YQ d -i "$ROOT_DIR/$v1/datadoghq.com_datadogagents.yaml" 'spec.versions[*].schema.openAPIV3Schema.properties.status.properties.defaultOverride.properties'
