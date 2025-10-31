#!/usr/bin/env bash

set -o errexit
set -o nounset
set -o pipefail

SCRIPTS_DIR="$(dirname "$0")"
# Provides $OS,$ARCH,$PLATFORM,$ROOT variables
source "$SCRIPTS_DIR/os-env.sh"

RH_BUNDLE_PATH="$ROOT/bundle-redhat-certified"
YQ="$ROOT/bin/$PLATFORM/yq"

# RH Bundle folder
rm -rf "$RH_BUNDLE_PATH"
cp -R "$ROOT/bundle" "$RH_BUNDLE_PATH"

# Patch annotations.yaml
eval $SED -e \'s/operators.operatorframework.io.bundle.package.v1: datadog-operator/operators.operatorframework.io.bundle.package.v1: datadog-operator-certified/g\' "$RH_BUNDLE_PATH/metadata/annotations.yaml"

# Patch CSV
eval $SED \'s#image: gcr.io/datadoghq/operator:#image: registry.connect.redhat.com/datadog/operator:#g\' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"
eval $SED \'s#containerImage: gcr.io/datadoghq/operator:#containerImage: registry.connect.redhat.com/datadog/operator:#g\' "$RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml"

# set `DD_TOOL_VERSION` to `redhat-certified`
$YQ -i "(.spec.install.spec.deployments[] | select(.name == \"datadog-operator-manager\") | .spec.template.spec.containers[] | select(.name == \"manager\") | .env[] | select(.name == \"DD_TOOL_VERSION\") | .value) = \"redhat-certified\"" $RH_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml

# Pin images
$ROOT/bin/$PLATFORM/operator-manifest-tools pinning pin "$RH_BUNDLE_PATH/manifests" -a ~/.redhat/auths.json -r skopeo

# Remove tests folder as unused
rm -rf "$RH_BUNDLE_PATH/tests"

# Generate the marketplace bundle
RHMP_BUNDLE_PATH="$ROOT/bundle-redhat-marketplace"
rm -rf "$RHMP_BUNDLE_PATH"
cp -R "$RH_BUNDLE_PATH" "$RHMP_BUNDLE_PATH"

mv "$RHMP_BUNDLE_PATH/manifests/datadog-operator.clusterserviceversion.yaml" "$RHMP_BUNDLE_PATH/manifests/datadog-operator-certified-rhmp.clusterserviceversion.yaml"
eval $SED \'s/datadog-operator-certified/datadog-operator-certified-rhmp/g\' "$RHMP_BUNDLE_PATH/metadata/annotations.yaml"

# Add marketplace annotations in CSV
$YQ -i '.metadata.annotations."marketplace.openshift.io/remote-workflow" = "https://marketplace.redhat.com/en-us/operators/datadog-operator-certified-rhmp/pricing?utm_source=openshift_console"' $RHMP_BUNDLE_PATH/manifests/datadog-operator-certified-rhmp.clusterserviceversion.yaml
$YQ -i '.metadata.annotations."marketplace.openshift.io/support-workflow" = "https://marketplace.redhat.com/en-us/operators/datadog-operator-certified-rhmp/support?utm_source=openshift_console"' $RHMP_BUNDLE_PATH/manifests/datadog-operator-certified-rhmp.clusterserviceversion.yaml

# set `DD_TOOL_VERSION` to `redhat-marketplace`
$YQ -i "(.spec.install.spec.deployments[] | select(.name == \"datadog-operator-manager\") | .spec.template.spec.containers[] | select(.name == \"manager\") | .env[] | select(.name == \"DD_TOOL_VERSION\") | .value) = \"redhat-marketplace\"" $RHMP_BUNDLE_PATH/manifests/datadog-operator-certified-rhmp.clusterserviceversion.yaml
