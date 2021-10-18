#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(git rev-parse --show-toplevel)
CRD_VERSION=${1:-v1}
RH_BUNDLE_PATH="$ROOT/bundle-redhat-$CRD_VERSION"
RH_BUNDLE_DOCKERFILE="$ROOT/bundle.redhat.$CRD_VERSION.Dockerfile"

PATCH_v1beta1=$(cat << EOF
# Specify which OpenShift version we support
LABEL com.redhat.openshift.versions="v4.5,v4.6,v4.7,v4.8"
# Specify that we are compatible with OpenShift <= 4.4
LABEL com.redhat.delivery.backport=true
EOF
)

PATCH_v1=$(cat << EOF
# Specify which OpenShift version we support
LABEL com.redhat.openshift.versions="=v4.9"
EOF
)

# RH Bundle folder
rm -rf "$RH_BUNDLE_PATH"
cp -R "$ROOT/bundle-$CRD_VERSION" "$RH_BUNDLE_PATH"

# RH Bundle Dockerfile
cp "$ROOT/bundle.Dockerfile" "$RH_BUNDLE_DOCKERFILE"

# Patch Dockerfile
patch_name="PATCH_$CRD_VERSION"
cat <<EOF >> $RH_BUNDLE_DOCKERFILE
${!patch_name}
EOF

sed -i "s/operators.operatorframework.io.bundle.package.v1=datadog-operator/operators.operatorframework.io.bundle.package.v1=datadog-operator-certified/g" "$RH_BUNDLE_DOCKERFILE"
sed -i "s#COPY bundle/#COPY bundle-redhat-$CRD_VERSION/#g" "$RH_BUNDLE_DOCKERFILE"

# Patch annotations.yaml
sed -i "s/operators.operatorframework.io.bundle.package.v1: datadog-operator/operators.operatorframework.io.bundle.package.v1: datadog-operator-certified/g" "$RH_BUNDLE_PATH/metadata/annotations.yaml"

# Patch CSV
sed -i "s#image: datadog/operator:#image: registry.connect.redhat.com/datadog/operator:#g" "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"
# Patch images in DatadogAgent examples for bundle validation
sed -i "s#gcr.io/datadoghq/#datadog/#g" "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"
