#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLAFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"
YQ="$ROOT/bin/$PLATFORM/yq"

v1=config/crd/bases/v1

# Remove defaultOverride section in DatadogAgent status due to the error: "datadoghq.com_datadogagents.yaml bigger than total allowed limit"
$YQ -i 'del(.spec.versions[].schema.openAPIV3Schema.properties.status.properties.defaultOverride)' "$ROOT/$v1/datadoghq.com_datadogagents.yaml"

for crd in "$ROOT/$v1"/*.yaml
do
  $YQ -i -P "$crd"
  go run $SCRIPTS_DIR/jsonschema/openapi2jsonschema.go "$crd"
done

# Special run for the DatadogPodAutoscaler CRD
OPT_PATCH_RESOURCE_LIST=true go run $SCRIPTS_DIR/jsonschema/openapi2jsonschema.go "$ROOT/$v1/datadoghq.com_datadogpodautoscalers.yaml"
