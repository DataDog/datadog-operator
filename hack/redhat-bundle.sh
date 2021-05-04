#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname $0)/os-env.sh"

ROOT=$(git rev-parse --show-toplevel)
RH_BUNDLE_PATH="$ROOT/bundle-redhat"
RH_BUNDLE_DOCKERFILE="$ROOT/bundle.redhat.Dockerfile"

# RH Bundle folder
rm -rf "$RH_BUNDLE_PATH"
cp -R "$ROOT/bundle" "$RH_BUNDLE_PATH"

# RH Bundle Dockerfile
cp "$ROOT/bundle.Dockerfile" "$RH_BUNDLE_DOCKERFILE"

# Patch Dockerfile
cat <<EOF >> $RH_BUNDLE_DOCKERFILE
# RedHat OpenShift specific labels
# Specify which OpenShift version we support
LABEL com.redhat.openshift.versions="v4.5,v4.6,v4.7"
LABEL com.redhat.delivery.operator.bundle=true
# Specify that we are compatible with OpenShift <= 4.4
LABEL com.redhat.delivery.backport=true
EOF

$SED 's/operators.operatorframework.io.bundle.package.v1=datadog-operator/operators.operatorframework.io.bundle.package.v1=datadog-operator-certified/g' "$RH_BUNDLE_DOCKERFILE"
$SED 's#COPY bundle/#COPY bundle-redhat/#g' "$RH_BUNDLE_DOCKERFILE"

# Patch annotations.yaml
$SED 's/operators.operatorframework.io.bundle.package.v1: datadog-operator/operators.operatorframework.io.bundle.package.v1: datadog-operator-certified/g' "$RH_BUNDLE_PATH/metadata/annotations.yaml"

# Patch CSV
$SED 's#image: datadog/operator:#image: registry.connect.redhat.com/datadog/operator:#g' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"
# Patch images in DatadogAgent examples for bundle validation
$SED 's#gcr.io/datadoghq/#datadog/#g' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"

# Cleanup .bak files
find $RH_BUNDLE_PATH -name "*.bak" -type f -delete
