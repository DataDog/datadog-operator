// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

const (
	// AppKubernetesNameLabelKey The name of the application
	AppKubernetesNameLabelKey = "app.kubernetes.io/name"
	// AppKubernetesInstanceLabelKey A unique name identifying the instance of an application
	AppKubernetesInstanceLabelKey = "app.kubernetes.io/instance"
	// AppKubernetesVersionLabelKey The current version of the application
	AppKubernetesVersionLabelKey = "app.kubernetes.io/version"
	// AppKubernetesComponentLabelKey The component within the architecture
	AppKubernetesComponentLabelKey = "app.kubernetes.io/component"
	// AppKubernetesPartOfLabelKey The name of a higher level application this one is part of
	AppKubernetesPartOfLabelKey = "app.kubernetes.io/part-of"
	// AppKubernetesManageByLabelKey The tool being used to manage the operation of an application
	AppKubernetesManageByLabelKey = "app.kubernetes.io/managed-by"
)

// ObjectKind type for kubernetes resource kind.
type ObjectKind string

const (
	// ConfigMapKind ConfigMaps resource kind
	ConfigMapKind ObjectKind = "configmaps"
	// ClusterRolesKind ClusterRoles resource kind
	ClusterRolesKind = "clusterroles"
	// ClusterRoleBindingKind ClusterRoleBindings resource kind
	ClusterRoleBindingKind = "clusterrolebindings"
	// RolesKind Roles resource kind
	RolesKind = "roles"
	// RoleBindingKind RoleBinding resource kind
	RoleBindingKind = "rolebindings"
	// MutatingWebhookConfigurationsKind MutatingWebhookConfigurations resource kind
	MutatingWebhookConfigurationsKind = "mutatingwebhookconfigurations"
	// APIServiceKind APIService resource kind
	APIServiceKind = "apiservices"
	// SecretsKind Secrets resource kind
	SecretsKind = "secrets"
	// ServicesKind Services resource kind
	ServicesKind = "services"
	// ServiceAccountsKind ServiceAccounts resource kind
	ServiceAccountsKind = "serviceaccounts"
	// PodDisruptionBudgetsKind PodDisruptionBudgets resource kind
	PodDisruptionBudgetsKind = "poddisruptionbudgets"
	// NetworkPoliciesKind NetworkPolicies resource kind
	NetworkPoliciesKind = "networkpolicies"
	// PodSecurityPoliciesKind PodSecurityPolicies resource kind
	PodSecurityPoliciesKind = "podsecuritypolicies"
	// CiliumNetworkPoliciesKind CiliumNetworkPolicies resource kind
	CiliumNetworkPoliciesKind = "ciliumnetworkpolicies"
)

// GetResourcesKind return the list of all possible ObjectKind supported as DatadogAgent dependencies
func GetResourcesKind(withCiliumResources bool) []ObjectKind {
	resources := []ObjectKind{
		ConfigMapKind,
		ClusterRolesKind,
		ClusterRoleBindingKind,
		RolesKind,
		RoleBindingKind,
		MutatingWebhookConfigurationsKind,
		APIServiceKind,
		SecretsKind,
		ServicesKind,
		ServiceAccountsKind,
		PodDisruptionBudgetsKind,
		NetworkPoliciesKind,
		PodSecurityPoliciesKind,
	}

	if withCiliumResources {
		resources = append(resources, CiliumNetworkPoliciesKind)
	}

	return resources
}
