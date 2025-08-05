#!/bin/bash

set -euo pipefail

if [ $# -eq 0 ]; then
    echo "Missing version to publish"
    exit 1
fi

REGISTRY=gcr.io/datadog-public/datadog
FULL_TAG=$1
SHORT_TAG=$(echo "$FULL_TAG" | cut -d '.' -f 1-2)

# Determine the script directory
SCRIPTS_DIR="../../../hack"
# Source common installation variables and functions
source "$SCRIPTS_DIR/os-env.sh"

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

# Get each platform manifest digest for the operator image
digest_array=()
while IFS= read -r line; do
  digest_array+=("$line")
done < <(gcrane manifest "$REGISTRY/datadog-operator:$FULL_TAG" | jq -r '.manifests[].digest')

# Build gcrane index append manifests args
append_manifests=""

# Mutate each image to add the annotation
echo "### Adding image manifest annotations..."
for digest in "${digest_array[@]}"; do
  echo "### Annotating $digest"

  new_digest=$(gcrane mutate --annotation "com.googleapis.cloudmarketplace.product.service.name=services/datadog-datadog-saas.cloudpartnerservices.goog" "$REGISTRY/datadog-operator@$digest" | tee /dev/tty | tail -n 1)

  echo "### New image digest: $new_digest"
  append_manifests+=" -m $new_digest"

done

# Create new index manifest with the annotated images
append_cmd="gcrane index append $append_manifests -t $REGISTRY/datadog-operator:$FULL_TAG"
sh -c "$append_cmd"
gcloud container images add-tag "$REGISTRY/datadog-operator:$FULL_TAG" "$REGISTRY/datadog-operator:$SHORT_TAG" --quiet

# We must use go-containerregistry to annotate the new manifest, as gcrane does not support annotating index manifests
cd "../../.."
make annotate-gcp-manifest
FULL_TAG=$FULL_TAG bin/"$PLATFORM"/annotate-manifest

# We use gcloud to add the tag to the existing manifest, as docker push creates a new manifest
gcloud container images add-tag "$REGISTRY/datadog-operator:$FULL_TAG" "$REGISTRY/datadog-operator:$SHORT_TAG" --quiet