#!/bin/bash
set -e

source "$(dirname $0)/../os-env.sh"

TAG=""
if [ $# -gt 0 ]; then
    TAG=$1
    echo "TAG=$TAG"
else
    echo "First parameter should be the new TAG"
    exit 1
fi
VERSION=${TAG:1}

GIT_ROOT=$(git rev-parse --show-toplevel)
PLUGIN_NAME=kubectl-datadog
OUTPUT_FOLDER=$GIT_ROOT/dist
TARBALL_NAME="$PLUGIN_NAME_$VERSION.tar.gz"

cp -Lr $GIT_ROOT/chart/* $OUTPUT_FOLDER/

for CHART in datadog-operator datadog-agent-with-operator
do
    $SED "s/PLACEHOLDER_VERSION/$VERSION/g" $OUTPUT_FOLDER/$CHART/Chart.yaml
    $SED "s/PLACEHOLDER_VERSION/$VERSION/g" $OUTPUT_FOLDER/$CHART/values.yaml
    tar -zcvf $OUTPUT_FOLDER/$CHART.tar.gz -C $OUTPUT_FOLDER $CHART
done
