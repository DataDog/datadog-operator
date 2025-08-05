#!/bin/bash

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Missing version to publish"
    exit 1
fi

REGISTRY=gcr.io/datadog-public/datadog
FULL_TAG=$1
SHORT_TAG=$(echo "$FULL_TAG" | cut -d '.' -f 1-2)

echo "### Copying operator '$FULL_TAG/$SHORT_TAG' from DockerHub to '$REGISTRY/operator'"

gcrane cp "datadog/operator:$FULL_TAG" "$REGISTRY/datadog-operator:$FULL_TAG"
gcrane cp "datadog/operator:$FULL_TAG" "$REGISTRY/datadog-operator:$SHORT_TAG"

echo "### Publishing Deployer"

APP_VERSION=$(yq eval '.spec.descriptor.version' chart/datadog-mp/templates/application.yaml)
if [ "$APP_VERSION" != "$FULL_TAG" ];
then
  echo "### Input version: $FULL_TAG does not match '.spec.descriptor.version' from chart/datadog-mp/templates/application.yaml ($APP_VERSION). Please update this file"
  exit 1
fi

docker build --pull --platform linux/amd64 --no-cache --build-arg TAG="$FULL_TAG" --tag "$REGISTRY/deployer:$FULL_TAG" . && docker push "$REGISTRY/deployer:$FULL_TAG"
gcrane mutate --annotation "com.googleapis.cloudmarketplace.product.service.name=services/datadog-datadog-saas.cloudpartnerservices.goog" "$REGISTRY/deployer:$FULL_TAG"
# We use gcloud to add the tag to the existing manifest, as docker push creates a new manifest
gcloud container images add-tag "$REGISTRY/deployer:$FULL_TAG" "$REGISTRY/deployer:$SHORT_TAG" --quiet

# Get the each platform manifest digest for the operator image
readarray -t manifest_digests < <(
  gcrane manifest "$REGISTRY/datadog-operator:$FULL_TAG" | jq -r '.manifests[].digest'
)

append_manifests = ""

# Mutate each image to add the annotation
echo "📦 Found ${#manifest_digests[@]} manifest digests:"
for digest in "${manifest_digests[@]}"; do
  echo "- $digest"
  gcrane mutate --annotation "com.googleapis.cloudmarketplace.product.service.name=services/datadog-datadog-saas.cloudpartnerservices.goog" "$REGISTRY/operator@sha256:$digest"

  new_digest=$(gcrane mutate gcr.io/datadog-public/datadog/datadog-operator@$digest \
    -a com.googleapis.cloudmarketplace.product.service.name=services/datadog-datadog-saas.cloudpartnerservices.goog \
    | tee /dev/tty | tail -n 1)

  append_cmd+=("-m $new_digest")

done

append_cmd = "gcrane index append" + append_manifests + " -t $REGISTRY/datadog-operator:$FULL_TAG -t $REGISTRY/datadog-operator:$SHORT_TAG"

echo append_cmd
