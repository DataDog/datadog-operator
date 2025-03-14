// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

const (
	DDClusterAgentAuthToken          = "DD_CLUSTER_AGENT_AUTH_TOKEN"
	DDClusterAgentServiceAccountName = "DD_CLUSTER_AGENT_SERVICE_ACCOUNT_NAME"

	// InstallInfoToolVersion is used by the Operator to override the tool
	// version value in the Agent's install info
	InstallInfoToolVersion = "DD_TOOL_VERSION"

	DDKubernetesResourcesLabelsAsTags      = "DD_KUBERNETES_RESOURCES_LABELS_AS_TAGS"
	DDKubernetesResourcesAnnotationsAsTags = "DD_KUBERNETES_RESOURCES_ANNOTATIONS_AS_TAGS"
)
