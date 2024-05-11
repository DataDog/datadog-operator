#!/usr/bin/env bash

set -e

ROOT_DIR=$(git rev-parse --show-toplevel)
YQ="$ROOT_DIR/bin/yq"

v1beta1=config/crd/bases/v1beta1
v1=config/crd/bases/v1

# Remove "x-kubernetes-*" as only supported in Kubernetes 1.16+.
# Users of Kubernetes < 1.16 need to use v1beta1, others need to use v1
#
# Cannot use directly yq -d .. 'spec.validation.openAPIV3Schema.properties.**.x-kubernetes-*'
# as for some reason, yq takes several minutes to execute this command
for crd in $(ls "$ROOT_DIR/$v1beta1")
do
  for path in $($YQ r "$ROOT_DIR/$v1beta1/$crd" 'spec.validation.openAPIV3Schema.properties.**.x-kubernetes-*' --printMode p)
  do
    $YQ d -i "$ROOT_DIR/$v1beta1/$crd" $path
  done
done

# Update the `protocol` attribute of v1.ContainerPort to required as default is not yet supported
# See: https://github.com/kubernetes/api/blob/master/core/v1/types.go#L2165
# Until issue is fixed: https://github.com/kubernetes-sigs/controller-tools/issues/438 and integrated in operator-sdk
$YQ m -i "$ROOT_DIR/$v1beta1/datadoghq.com_extendeddaemonsetreplicasets.yaml" "$ROOT_DIR/hack/patch-crd-v1beta1-protocol-kube1.18.yaml"
$YQ m -i "$ROOT_DIR/$v1beta1/datadoghq.com_extendeddaemonsets.yaml" "$ROOT_DIR/hack/patch-crd-v1beta1-protocol-kube1.18.yaml"
$YQ m -i "$ROOT_DIR/$v1/datadoghq.com_extendeddaemonsetreplicasets.yaml" "$ROOT_DIR/hack/patch-crd-v1-protocol-kube1.18.yaml"
$YQ m -i "$ROOT_DIR/$v1/datadoghq.com_extendeddaemonsets.yaml" "$ROOT_DIR/hack/patch-crd-v1-protocol-kube1.18.yaml"


# Update `metadata` attribute of v1.PodTemplateSpec to properly validate the
# resource's metadata, since the automatically generated validation is
# insufficient.
patch_CRD_metadata="$ROOT_DIR/hack/patch-crd-metadata.yaml"

# Different object path in v1beta1 and v1. Notice that v1 assumes that there's just 1 version
openAPIV3Schema_path_v1beta1="spec.validation.openAPIV3Schema"
openAPIV3Schema_path="spec.versions[0].schema.openAPIV3Schema"
metadata_subpath="properties.spec.properties.template.properties.metadata"

$YQ w -i "$ROOT_DIR/$v1beta1/datadoghq.com_extendeddaemonsetreplicasets.yaml" "$openAPIV3Schema_path_v1beta1"."$metadata_subpath" -f "$patch_CRD_metadata"
$YQ w -i "$ROOT_DIR/$v1beta1/datadoghq.com_extendeddaemonsets.yaml" "$openAPIV3Schema_path_v1beta1"."$metadata_subpath" -f "$patch_CRD_metadata"
$YQ w -i "$ROOT_DIR/$v1/datadoghq.com_extendeddaemonsetreplicasets.yaml" "$openAPIV3Schema_path"."$metadata_subpath" -f "$patch_CRD_metadata"
$YQ w -i "$ROOT_DIR/$v1/datadoghq.com_extendeddaemonsets.yaml" "$openAPIV3Schema_path"."$metadata_subpath" -f "$patch_CRD_metadata"
