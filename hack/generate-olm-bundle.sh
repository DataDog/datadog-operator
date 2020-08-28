#!/usr/bin/env bash

TAG=""
if [ $# -gt 0 ]; then
    TAG=$1
    echo "TAG=$TAG"
else
    echo "First parameter should be the new TAG"
    exit 1
fi
VERSION=${TAG:1}

source "$(dirname $0)/os-env.sh"

ROOT=$(git rev-parse --show-toplevel)
OLM_FOLDER=$ROOT/deploy/olm-catalog/datadog-operator
IMAGE_NAME='datadog/operator'
REDHAT_REGISTRY='registry.connect.redhat.com/'
REDHAT_IMAGE_NAME="${REDHAT_REGISTRY}${IMAGE_NAME}"
ZIP_FILE_NAME=$ROOT/dist/olm-redhat-bundle.zip

WORK_DIR=$(mktemp -d)

# deletes the temp directory
function cleanup {      
  rm -rf "$WORK_DIR"
  echo "Deleted temp working directory $WORK_DIR"
}

# register the cleanup function to be called on the EXIT signal
trap cleanup EXIT

# move all zip file if exit
mv $ZIP_FILE_NAME $ZIP_FILE_NAME.old

for i in $OLM_FOLDER/$VERSION/*.yaml $OLM_FOLDER/*.yaml
do  
    cp $i $WORK_DIR/${i##*/}
    $SED -e "s|${IMAGE_NAME}|${REDHAT_IMAGE_NAME}|g" $WORK_DIR/${i##*/}
    echo "$i $filename" 
done

pushd $WORK_DIR
$SED -e 's/packageName\: datadog-operator/packageName\: datadog-operator-certified/g' datadog-operator.package.yaml
rm *.bak
zip $ZIP_FILE_NAME ./*.yaml
popd
