#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

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
LABEL com.redhat.openshift.versions="v4.4-v4.9"
LABEL com.redhat.delivery.operator.bundle=true
EOF

sed -i 's/operators.operatorframework.io.bundle.package.v1=datadog-operator/operators.operatorframework.io.bundle.package.v1=datadog-operator-certified/g' "$RH_BUNDLE_DOCKERFILE"
sed -i 's#COPY bundle/#COPY bundle-redhat/#g' "$RH_BUNDLE_DOCKERFILE"

# Patch annotations.yaml
sed -i 's/operators.operatorframework.io.bundle.package.v1: datadog-operator/operators.operatorframework.io.bundle.package.v1: datadog-operator-certified/g' "$RH_BUNDLE_PATH/metadata/annotations.yaml"

# Patch CSV
sed -i 's#image: gcr.io/datadoghq/operator:#image: registry.connect.redhat.com/datadog/operator:#g' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"
sed -i 's#containerImage: gcr.io/datadoghq/operator:#containerImage: registry.connect.redhat.com/datadog/operator:#g' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"
