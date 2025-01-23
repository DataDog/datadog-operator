// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

// This file tracks constants used in features, component default code

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

// Annotations
const (
	AppArmorAnnotationKey = "container.apparmor.security.beta.kubernetes.io"

	SystemProbeAppArmorAnnotationKey   = "container.apparmor.security.beta.kubernetes.io/system-probe"
	SystemProbeAppArmorAnnotationValue = "unconfined"
)

// Condition types
const (
	// ClusterAgentReconcileConditionType ReconcileConditionType for Cluster Agent component
	ClusterAgentReconcileConditionType = "ClusterAgentReconcile"
	// AgentReconcileConditionType ReconcileConditionType for Agent component
	AgentReconcileConditionType = "AgentReconcile"
	// ClusterChecksRunnerReconcileConditionType ReconcileConditionType for Cluster Checks Runner component
	ClusterChecksRunnerReconcileConditionType = "ClusterChecksRunnerReconcile"
	// OverrideReconcileConflictConditionType ReconcileConditionType for override conflict
	OverrideReconcileConflictConditionType = "OverrideReconcileConflict"
	// DatadogAgentReconcileErrorConditionType ReconcileConditionType for DatadogAgent reconcile error
	DatadogAgentReconcileErrorConditionType = "DatadogAgentReconcileError"
)
