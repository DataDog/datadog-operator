// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

const (
	// FieldPathSpecNodeName used as FieldPath for selecting the NodeName
	FieldPathSpecNodeName = "spec.nodeName"

	// FieldPathStatusHostIP used as FieldPath to retrieve the host ip
	FieldPathStatusHostIP = "status.hostIP"

	// FieldPathStatusPodIP used as FieldPath to retrieve the pod ip
	FieldPathStatusPodIP = "status.podIP"

	// FieldPathMetaName used as FieldPath to retrieve the pod name
	FieldPathMetaName = "metadata.name"

	// kind names definition
	extendedDaemonSetKind   = "ExtendedDaemonSet"
	daemonSetKind           = "DaemonSet"
	deploymentKind          = "Deployment"
	clusterRoleKind         = "ClusterRole"
	clusterRoleBindingKind  = "ClusterRoleBinding"
	roleKind                = "Role"
	roleBindingKind         = "RoleBinding"
	configMapKind           = "ConfigMap"
	serviceAccountKind      = "ServiceAccount"
	podDisruptionBudgetKind = "PodDisruptionBudget"
	secretKind              = "Secret"
	serviceKind             = "Service"
	apiServiceKind          = "APIService"
	networkPolicyKind       = "NetworkPolicy"
	ciliumNetworkPolicyKind = "CiliumNetworkPolicy"

	// Datadog tags prefix
	datadogTagPrefix = "tags.datadoghq.com"
)
