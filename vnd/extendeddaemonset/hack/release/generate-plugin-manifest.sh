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
PLUGIN_NAME=kubectl-eds
OUTPUT_FOLDER=$GIT_ROOT/dist
TARBALL_NAME="$PLUGIN_NAME_$VERSION.tar.gz"

DARWIN_AMD64=$(grep $PLUGIN_NAME $OUTPUT_FOLDER/checksums.txt  | grep "darwin_amd64" | awk '{print $1}')
WINDOWS_AMD64=$(grep $PLUGIN_NAME $OUTPUT_FOLDER/checksums.txt  | grep "windows_amd64" | awk '{print $1}')
LINUX_AMD64=$(grep $PLUGIN_NAME $OUTPUT_FOLDER/checksums.txt  | grep "linux_amd64" | awk '{print $1}')
DARWIN_ARM64=$(grep $PLUGIN_NAME $OUTPUT_FOLDER/checksums.txt  | grep "darwin_arm64" | awk '{print $1}')
WINDOWS_ARM64=$(grep $PLUGIN_NAME $OUTPUT_FOLDER/checksums.txt  | grep "windows_arm64" | awk '{print $1}')
LINUX_ARM64=$(grep $PLUGIN_NAME $OUTPUT_FOLDER/checksums.txt  | grep "linux_arm64" | awk '{print $1}')

echo "DARWIN_AMD64=$DARWIN_AMD64"
echo "WINDOWS_AMD64=$WINDOWS_AMD64"
echo "LINUX_AMD64=$LINUX_AMD64"
echo "DARWIN_ARM64=$DARWIN_ARM64"
echo "WINDOWS_ARM64=$WINDOWS_ARM64"
echo "LINUX_ARM64=$LINUX_ARM64"

cp $GIT_ROOT/hack/release/eds-plugin-tmpl.yaml $OUTPUT_FOLDER/eds-plugin.yaml

$SED "s/PLACEHOLDER_TAG/$TAG/g" $OUTPUT_FOLDER/eds-plugin.yaml
$SED "s/PLACEHOLDER_VERSION/$VERSION/g" $OUTPUT_FOLDER/eds-plugin.yaml
$SED "s/PLACEHOLDER_SHA_AMD_DARWIN/$DARWIN_AMD64/g" $OUTPUT_FOLDER/eds-plugin.yaml
$SED "s/PLACEHOLDER_SHA_AMD_LINUX/$LINUX_AMD64/g" $OUTPUT_FOLDER/eds-plugin.yaml
$SED "s/PLACEHOLDER_SHA_AMD_WINDOWS/$WINDOWS_AMD64/g" $OUTPUT_FOLDER/eds-plugin.yaml
$SED "s/PLACEHOLDER_SHA_ARM_DARWIN/$DARWIN_ARM64/g" $OUTPUT_FOLDER/eds-plugin.yaml
$SED "s/PLACEHOLDER_SHA_ARM_LINUX/$LINUX_ARMD64/g" $OUTPUT_FOLDER/eds-plugin.yaml
$SED "s/PLACEHOLDER_SHA_ARM_WINDOWS/$WINDOWS_ARM64/g" $OUTPUT_FOLDER/eds-plugin.yaml
