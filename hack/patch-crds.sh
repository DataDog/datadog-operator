#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLAFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"
YQ="$ROOT/bin/$PLATFORM/yq"

v1beta1=config/crd/bases/v1beta1
v1=config/crd/bases/v1

# Remove "x-kubernetes-*" as only supported in Kubernetes 1.16+.
# Users of Kubernetes < 1.16 need to use v1beta1, others need to use v1
for crd in "$ROOT/$v1beta1"/*.yaml
do
  $YQ -i 'del(.spec.validation.openAPIV3Schema.properties.**.x-kubernetes-*)' "$crd"
done

# Remove defaultOverride section in DatadogAgent status due to the error: "datadoghq.com_datadogagents.yaml bigger than total allowed limit"
$YQ -i 'del(.spec.validation.openAPIV3Schema.properties.status.properties.defaultOverride)' "$ROOT/$v1beta1/datadoghq.com_datadogagents.yaml"
$YQ -i 'del(.spec.versions[].schema.openAPIV3Schema.properties.status.properties.defaultOverride)' "$ROOT/$v1/datadoghq.com_datadogagents.yaml"

# Pretty print CRD files so they they all have same formatting
for crd in "$ROOT/$v1beta1"/*.yaml
do
  $YQ -i -P "$crd"
done

for crd in "$ROOT/$v1"/*.yaml
do
  $YQ -i -P "$crd"
  go run $SCRIPTS_DIR/jsonschema/openapi2jsonschema.go "$crd"
done

# Special run for the DatadogPodAutoscaler CRD
OPT_PATCH_RESOURCE_LIST=true go run $SCRIPTS_DIR/jsonschema/openapi2jsonschema.go "$ROOT/$v1/datadoghq.com_datadogpodautoscalers.yaml"
