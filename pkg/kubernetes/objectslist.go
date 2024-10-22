// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetes

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	ciliumv1 "github.com/DataDog/datadog-operator/pkg/cilium/v1"
)

// ObjectListFromKind returns the corresponding object list from a kind
func ObjectListFromKind(kind ObjectKind, platformInfo PlatformInfo) client.ObjectList {
	switch kind {
	case ConfigMapKind:
		return &corev1.ConfigMapList{}
	case ClusterRolesKind:
		return &rbacv1.ClusterRoleList{}
	case ClusterRoleBindingKind:
		return &rbacv1.ClusterRoleBindingList{}
	case RolesKind:
		return &rbacv1.RoleList{}
	case RoleBindingKind:
		return &rbacv1.RoleBindingList{}
	case ValidatingWebhookConfigurationsKind:
		return &admissionregistrationv1.ValidatingWebhookConfigurationList{}
	case MutatingWebhookConfigurationsKind:
		return &admissionregistrationv1.MutatingWebhookConfigurationList{}
	case APIServiceKind:
		return &apiregistrationv1.APIServiceList{}
	case SecretsKind:
		return &corev1.SecretList{}
	case ServicesKind:
		return &corev1.ServiceList{}
	case ServiceAccountsKind:
		return &corev1.ServiceAccountList{}
	case PodDisruptionBudgetsKind:
		return &policyv1.PodDisruptionBudgetList{}
	case NetworkPoliciesKind:
		return &networkingv1.NetworkPolicyList{}
	case CiliumNetworkPoliciesKind:
		return ciliumv1.EmptyCiliumUnstructuredListPolicy()
	}

	return nil
}
