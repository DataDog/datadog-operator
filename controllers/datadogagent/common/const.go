// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// Resource names
const (
	DatadogTokenOldResourceName          = "datadogtoken"            // Kept for backward compatibility with agent <7.37.0
	DatadogLeaderElectionOldResourceName = "datadog-leader-election" // Kept for backward compatibility with agent <7.37.0
	DatadogCustomMetricsResourceName     = "datadog-custom-metrics"
	DatadogClusterIDResourceName         = "datadog-cluster-id"
	ExtensionAPIServerAuthResourceName   = "extension-apiserver-authentication"
	KubeSystemResourceName               = "kube-system"

	NodeAgentSuffix    = "node"
	ChecksRunnerSuffix = "ccr"
	ClusterAgentSuffix = "dca"

	CustomResourceDefinitionsName = "customresourcedefinitions"

	DefaultAgentInstallType = "k8s_manual"
)
