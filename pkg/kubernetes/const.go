// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

// This file tracks string constants that are native to Kubernetes

const (
	// AppKubernetesComponentLabelKey is the key for the component within the architecture
	AppKubernetesComponentLabelKey = "app.kubernetes.io/component"
	// AppKubernetesInstanceLabelKey is the key for a unique name identifying the instance of an application
	AppKubernetesInstanceLabelKey = "app.kubernetes.io/instance"
	// AppKubernetesManageByLabelKey is the key for the tool being used to manage the operation of an application
	AppKubernetesManageByLabelKey = "app.kubernetes.io/managed-by"
	// AppKubernetesNameLabelKey is the key for the name of the application
	AppKubernetesNameLabelKey = "app.kubernetes.io/name"
	// AppKubernetesPartOfLabelKey is the key for the name of a higher level application this one is part of
	AppKubernetesPartOfLabelKey = "app.kubernetes.io/part-of"
	// AppKubernetesVersionLabelKey is the key for the current version of the application
	AppKubernetesVersionLabelKey = "app.kubernetes.io/version"
)

// ObjectKind type for kubernetes resource kind. These strings are plural because
// their list kind is used to query the Kubernetes API when cleaning up resources.

// They are also used in the store for DatadogAgent dependencies.
type ObjectKind string

const (
	// APIServiceKind is the APIService resource kind
	APIServiceKind = "apiservices"
	// CiliumNetworkPoliciesKind is the CiliumNetworkPolicies resource kind
	CiliumNetworkPoliciesKind = "ciliumnetworkpolicies"
	// ClusterRolesKind is the ClusterRoles resource kind
	ClusterRolesKind = "clusterroles"
	// ClusterRoleBindingKind is the ClusterRoleBindings resource kind
	ClusterRoleBindingKind = "clusterrolebindings"
	// ConfigMapKind is the ConfigMaps resource kind
	ConfigMapKind ObjectKind = "configmaps"
	// MutatingWebhookConfigurationsKind is the MutatingWebhookConfigurations resource kind
	MutatingWebhookConfigurationsKind = "mutatingwebhookconfigurations"
	// NetworkPoliciesKind is the NetworkPolicies resource kind
	NetworkPoliciesKind = "networkpolicies"
	// NodeKind is the Nodes resource kind
	NodeKind = "nodes"
	// PodDisruptionBudgetsKind is the PodDisruptionBudgets resource kind
	PodDisruptionBudgetsKind = "poddisruptionbudgets"
	// RoleBindingKind is the RoleBinding resource kind
	RoleBindingKind = "rolebindings"
	// RolesKind is the Roles resource kind
	RolesKind = "roles"
	// SecretsKind is the Secrets resource kind
	SecretsKind = "secrets"
	// ServiceAccountsKind is the ServiceAccounts resource kind
	ServiceAccountsKind = "serviceaccounts"
	// ServicesKind is the Services resource kind
	ServicesKind = "services"
	// ValidatingWebhookConfigurationsKind is the ValidatingWebhookConfigurations resource kind
	ValidatingWebhookConfigurationsKind = "validatingwebhookconfigurations"
)

// getResourcesKind return the list of all possible ObjectKind supported as DatadogAgent dependencies
func getResourcesKind(withCiliumResources bool) []ObjectKind {
	resources := []ObjectKind{
		APIServiceKind,
		ClusterRolesKind,
		ClusterRoleBindingKind,
		ConfigMapKind,
		MutatingWebhookConfigurationsKind,
		NetworkPoliciesKind,
		PodDisruptionBudgetsKind,
		RolesKind,
		RoleBindingKind,
		SecretsKind,
		ServiceAccountsKind,
		ServicesKind,
		ValidatingWebhookConfigurationsKind,
	}

	if withCiliumResources {
		resources = append(resources, CiliumNetworkPoliciesKind)
	}

	return resources
}

// These constants are used in Datadog event submission
const (
	// ExtendedDaemonSetKind is the ExtendedDaemonset resource kind
	ExtendedDaemonSetKind = "extendeddaemonset"
	// DaemonSetKind is the Daemonset resource kind
	DaemonSetKind = "daemonset"
	// DeploymentKind is the Deployment resource kind
	DeploymentKind = "deployment"
)
