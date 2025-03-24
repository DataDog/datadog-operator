// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package global

// This file tracks environment variables used in global
const (
	DDChecksTagCardinality         = "DD_CHECKS_TAG_CARDINALITY"
	DDClusterAgentAuthToken        = "DD_CLUSTER_AGENT_AUTH_TOKEN"
	DDCriSocketPath                = "DD_CRI_SOCKET_PATH"
	DDFIPSEnabled                  = "DD_FIPS_ENABLED"
	DDFIPSPortRangeStart           = "DD_FIPS_PORT_RANGE_START"
	DDFIPSUseHTTPS                 = "DD_FIPS_HTTPS"
	DDFIPSLocalAddress             = "DD_FIPS_LOCAL_ADDRESS"
	DDKubeletCAPath                = "DD_KUBELET_CLIENT_CA"
	DDKubeletTLSVerify             = "DD_KUBELET_TLS_VERIFY"
	DDKubernetesPodResourcesSocket = "DD_KUBERNETES_KUBELET_PODRESOURCES_SOCKET"
	DDNamespaceLabelsAsTags        = "DD_KUBERNETES_NAMESPACE_LABELS_AS_TAGS"
	DDNamespaceAnnotationsAsTags   = "DD_KUBERNETES_NAMESPACE_ANNOTATIONS_AS_TAGS"
	DDNodeLabelsAsTags             = "DD_KUBERNETES_NODE_LABELS_AS_TAGS"
	DDOriginDetectionUnified       = "DD_ORIGIN_DETECTION_UNIFIED"
	DDPodAnnotationsAsTags         = "DD_KUBERNETES_POD_ANNOTATIONS_AS_TAGS"
	DDPodLabelsAsTags              = "DD_KUBERNETES_POD_LABELS_AS_TAGS"
	DDSecretBackendCommand         = "DD_SECRET_BACKEND_COMMAND"
	DDSecretBackendArguments       = "DD_SECRET_BACKEND_ARGUMENTS"
	DDSecretBackendTimeout         = "DD_SECRET_BACKEND_TIMEOUT"
	DDTags                         = "DD_TAGS"
	DockerHost                     = "DOCKER_HOST"
)
