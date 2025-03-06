// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

const (
	DDClusterAgentAuthToken          = "DD_CLUSTER_AGENT_AUTH_TOKEN"
	DDClusterAgentServiceAccountName = "DD_CLUSTER_AGENT_SERVICE_ACCOUNT_NAME"
	DDInstallInfoToolVersion         = "DD_TOOL_VERSION"
	DDFIPSEnabled                    = "DD_FIPS_ENABLED"
	DDFIPSPortRangeStart             = "DD_FIPS_PORT_RANGE_START"
	DDFIPSUseHTTPS                   = "DD_FIPS_HTTPS"
	DDFIPSLocalAddress               = "DD_FIPS_LOCAL_ADDRESS"
	DDSecretBackendCommand           = "DD_SECRET_BACKEND_COMMAND"
	DDSecretBackendArguments         = "DD_SECRET_BACKEND_ARGUMENTS"
	DDSecretBackendTimeout           = "DD_SECRET_BACKEND_TIMEOUT"
	DDCriSocketPath                  = "DD_CRI_SOCKET_PATH"
	DockerHost                       = "DOCKER_HOST"
	DDKubernetesPodResourcesSocket   = "DD_KUBERNETES_KUBELET_PODRESOURCES_SOCKET"
	DDKubeletTLSVerify               = "DD_KUBELET_TLS_VERIFY"
	DDChecksTagCardinality           = "DD_CHECKS_TAG_CARDINALITY"
	DDKubeletCAPath                  = "DD_KUBELET_CLIENT_CA"
	DDNamespaceLabelsAsTags          = "DD_KUBERNETES_NAMESPACE_LABELS_AS_TAGS"
	DDNamespaceAnnotationsAsTags     = "DD_KUBERNETES_NAMESPACE_ANNOTATIONS_AS_TAGS"
	DDNodeLabelsAsTags               = "DD_KUBERNETES_NODE_LABELS_AS_TAGS"
	DDOriginDetectionUnified         = "DD_ORIGIN_DETECTION_UNIFIED"
	DDPodAnnotationsAsTags           = "DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS"
	DDPodLabelsAsTags                = "DD_KUBERNETES_POD_LABELS_AS_TAGS"
	DDTags                           = "DD_TAGS"
)
