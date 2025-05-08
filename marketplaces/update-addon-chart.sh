#!/bin/bash

# Usage: ./pull_deps.sh [version]
# If version is not provided, defaults to 2.9.2

set -euo pipefail

# CHART_CRD="datadog/datadog-crds"
# CHART_CRD_VERSION="2.6.0"
CHART_OPERATOR="datadog/datadog-operator"
CHART_OPERATOR_VERSION="${1:-2.9.2}"
SUBCHART_DIR="./charts/operator-eks-addon/charts"

mkdir -p "$SUBCHART_DIR"

echo "Pulling $CHART_OPERATOR@$CHART_OPERATOR_VERSION..."

helm repo update
rm -fr "./charts/operator-eks-addon/charts/datadog-operator"
helm pull "$CHART_OPERATOR" --version "$CHART_OPERATOR_VERSION" --untar --untardir "$SUBCHART_DIR"

# clean-up sub-charts to pass add-on validation

# delete v1beta1 CRDs
rm ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/datadoghq.com_datadogagents_v1beta1.yaml
rm ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/datadoghq.com_datadogmonitors_v1beta1.yaml
rm ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/datadoghq.com_datadogslos_v1beta1.yaml
rm ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/datadoghq.com_datadogagentprofiles_v1beta1.yaml
rm ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/datadoghq.com_datadogmetrics_v1beta1.yaml

# delete semverCompare not allowed by add-on validation
find ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/ -type f -name "*.yaml" -exec sed -i '' 's#(semverCompare ">1.21-0" .Capabilities.KubeVersion.GitVersion ) ##g' {} \;
find ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/ -type f -name "*.yaml" -exec sed -i '' 's#and ##g' {} \;

# replace '{{ .Release.Service }}' with eks-addon in CRD files
find ./charts/operator-eks-addon/charts/datadog-operator/charts/datadog-crds/templates/ -type f -name "*.yaml" -exec sed -i '' "s#'{{ .Release.Service }}'#eks-addon#g" {} \;
# do the same for datadog-operator _helpers.tpl
sed -i '' "s#{{ .Release.Service }}#eks-addon#g" ./charts/operator-eks-addon/charts/datadog-operator/templates/_helpers.tpl

# replace PDB policy version check with just v1 assignment, and clean up any extra end block
sed -i '' '/{{- define "policy.poddisruptionbudget.apiVersion" -}}/,/{{- end -}}/c\
{{- define "policy.poddisruptionbudget.apiVersion" -}}\
"policy/v1"\
{{- end -}}' ./charts/operator-eks-addon/charts/datadog-operator/templates/_helpers.tpl
sed -i '' "s#{{- end -}}{{- end -}}#{{- end -}}#g" ./charts/operator-eks-addon/charts/datadog-operator/templates/_helpers.tpl

# presence of gcr.io/datadoghq/operator in values.yaml may cause issues with add-on validation
sed -i '' 's#gcr.io/datadoghq/operator#709825985650.dkr.ecr.us-east-1.amazonaws.com/datadog/operator#g' ./charts/operator-eks-addon/charts/datadog-operator/values.yaml

# template the chart with default values
helm template operator-eks-addon ./charts/operator-eks-addon -n datadog-agent > addon_manifest.yaml
echo "Chart updated and templated to addon_manifest.yaml"
