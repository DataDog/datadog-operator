#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

ROOT=$(git rev-parse --show-toplevel)
PLATFORM="$(uname -s)-$(uname -m)"
RH_BUNDLE_PATH="$ROOT/bundle-redhat"
YQ="$ROOT/bin/$PLATFORM/yq"

# RH Bundle folder
rm -rf "$RH_BUNDLE_PATH"
cp -R "$ROOT/bundle" "$RH_BUNDLE_PATH"

# Patch annotations.yaml
sed -i 's/operators.operatorframework.io.bundle.package.v1: datadog-operator/operators.operatorframework.io.bundle.package.v1: datadog-operator-certified/g' "$RH_BUNDLE_PATH/metadata/annotations.yaml"

# Patch CSV
sed -i 's#image: gcr.io/datadoghq/operator:#image: registry.connect.redhat.com/datadog/operator:#g' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"
sed -i 's#containerImage: gcr.io/datadoghq/operator:#containerImage: registry.connect.redhat.com/datadog/operator:#g' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"

# Pin images
$ROOT/bin/$PLATFORM/operator-manifest-tools pinning pin "$RH_BUNDLE_PATH/manifests" -a ~/.redhat/auths.json

# Remove tests folder as unused
rm -rf "$RH_BUNDLE_PATH/tests"

# Generate the marketplace bundle
RHMP_BUNDLE_PATH="$RH_BUNDLE_PATH-mp"
rm -rf "$RHMP_BUNDLE_PATH"
cp -R "$RH_BUNDLE_PATH" "$RHMP_BUNDLE_PATH"

mv "$RHMP_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml" "$RHMP_BUNDLE_PATH/manifests/datadog-operator-certified-rhmp.clusterserviceversion.yaml"
sed -i 's/datadog-operator-certified/datadog-operator-certified-rhmp/g' "$RHMP_BUNDLE_PATH/metadata/annotations.yaml"

# Add marketplace annotations in CSV
$YQ w -i "$RHMP_BUNDLE_PATH/manifests/datadog-operator-certified-rhmp.clusterserviceversion.yaml" 'metadata.annotations."marketplace.openshift.io/remote-workflow"' "https://marketplace.redhat.com/en-us/operators/datadog-operator-certified-rhmp/pricing?utm_source=openshift_console"
$YQ w -i "$RHMP_BUNDLE_PATH/manifests/datadog-operator-certified-rhmp.clusterserviceversion.yaml" 'metadata.annotations."marketplace.openshift.io/support-workflow"' "https://marketplace.redhat.com/en-us/operators/datadog-operator-certified-rhmp/support?utm_source=openshift_console"
