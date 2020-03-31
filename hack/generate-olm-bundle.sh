#!/usr/bin/env sh

ROOT=$(git rev-parse --show-toplevel)
OLM_FOLDER=$ROOT/deploy/olm-catalog/datadog-operator
IMAGE_NAME='datadog/operator'
REDHAT_REGISTRY='registry.connect.redhat.com/'
REDHAT_IMAGE_NAME="${REDHAT_REGISTRY}${IMAGE_NAME}"
ZIP_FILE_NAME=$ROOT/tmp/olm-redhat-bundle.zip

WORK_DIR=$(mktemp -d -p $ROOT/tmp)

# deletes the temp directory
function cleanup {      
  rm -rf "$WORK_DIR"
  echo "Deleted temp working directory $WORK_DIR"
}

# register the cleanup function to be called on the EXIT signal
trap cleanup EXIT

# move all zip file if exit
mv $ZIP_FILE_NAME $ZIP_FILE_NAME.old

for i in $OLM_FOLDER/*/*.yaml $OLM_FOLDER/*.yaml
do  
    cp $i $WORK_DIR/${i##*/}
    sed -i'.bak' -e "s|${IMAGE_NAME}|${REDHAT_IMAGE_NAME}|g" $WORK_DIR/${i##*/}
    echo "$i $filename" 
done

pushd $WORK_DIR
sed -i'.bak' -e 's/packageName\: datadog-operator/packageName\: datadog-operator-certified/g' datadog-operator.package.yaml
rm *.bak
zip $ZIP_FILE_NAME ./*.yaml
popd

