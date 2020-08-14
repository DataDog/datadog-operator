#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

source "$(dirname $0)/os-env.sh"
ROOT=$(git rev-parse --show-toplevel)

pushd $ROOT/deploy/olm-catalog/datadog-operator/
PREVIOUS_VERSION=$(ls -dUt */ | grep -v \'-rc\' | head -1 | sed 's/.$//')
echo "PREVIOUS_VERSION=$PREVIOUS_VERSION"
popd

VERSION=""
RELEASE_CANDIDATE=""
if [ $# -gt 0 ]; then
    VERSION=$1
    echo "VERSION=$VERSION"
else
    echo "First parameter should be the new VERSION"
    exit 1
fi

if [ $# -gt 1 ]; then
    RELEASE_CANDIDATE=$2
    echo "RELEASE_CANDIDATE=$RELEASE_CANDIDATE"
    VERSION=$VERSION"-rc."$RELEASE_CANDIDATE
fi

VVERSION=v$VERSION

pushd $ROOT
# Update dockerfile
$SED "s/ARG TAG=.*/ARG TAG=$VERSION/g" $ROOT/Dockerfile

# Update chart version, and image.tag
$ROOT/bin/yq w -i $ROOT/chart/datadog-operator/Chart.yaml "version" $VERSION
$ROOT/bin/yq w -i $ROOT/chart/datadog-operator/values.yaml "image.tag" $VERSION
# Version in deploy folder
$ROOT/bin/yq w -i $ROOT/deploy/operator.yaml "spec.template.spec.containers[0].image" "datadog/operator:$VERSION"

# Run OLM generation
make VERSION=$VERSION generate-olm 

# Patch OLM Generation
OLM_FILE=$ROOT/deploy/olm-catalog/datadog-operator/$VERSION/datadog-operator.clusterserviceversion.yaml
$ROOT/bin/yq m -i --overwrite --autocreate=true $OLM_FILE $ROOT/hack/release/cluster-service-version-patch.yaml
$ROOT/bin/yq w -i $OLM_FILE "spec.customresourcedefinitions.owned[0].displayName" "Datadog Agent"
$ROOT/bin/yq w -i $OLM_FILE "spec.replaces" "datadog-operator.v$PREVIOUS_VERSION"
$ROOT/bin/yq w -i $OLM_FILE "metadata.annotations.createdAt" "$(date '+%Y-0%m-%d %T')"

# update datadog-operator.package.yaml
if [ -z "$RELEASE_CANDIDATE" ]; then
    # Update official channel
    $ROOT/bin/yq w -i $ROOT/deploy/olm-catalog/datadog-operator/datadog-operator.package.yaml "channels[0].currentCSV" "datadog-operator.$VVERSION"
fi
# always update the test channel
$ROOT/bin/yq w -i $ROOT/deploy/olm-catalog/datadog-operator/datadog-operator.package.yaml "channels[1].currentCSV" "datadog-operator.$VVERSION"


# cleanup tmp files
find . -name "*.bak" -type f -delete

# leave the ROOT folder
popd
