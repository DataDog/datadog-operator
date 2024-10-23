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

// ObjectFromKind returns the corresponding object list from a kind
func ObjectFromKind(kind ObjectKind, platformInfo PlatformInfo) client.Object {
	switch kind {
	case ConfigMapKind:
		return &corev1.ConfigMap{}
	case ClusterRolesKind:
		return &rbacv1.ClusterRole{}
	case ClusterRoleBindingKind:
		return &rbacv1.ClusterRoleBinding{}
	case RolesKind:
		return &rbacv1.Role{}
	case RoleBindingKind:
		return &rbacv1.RoleBinding{}
	case ValidatingWebhookConfigurationsKind:
		return &admissionregistrationv1.ValidatingWebhookConfiguration{}
	case MutatingWebhookConfigurationsKind:
		return &admissionregistrationv1.MutatingWebhookConfiguration{}
	case APIServiceKind:
		return &apiregistrationv1.APIService{}
	case SecretsKind:
		return &corev1.Secret{}
	case ServicesKind:
		return &corev1.Service{}
	case ServiceAccountsKind:
		return &corev1.ServiceAccount{}
	case PodDisruptionBudgetsKind:
		return &policyv1.PodDisruptionBudget{}
	case NetworkPoliciesKind:
		return &networkingv1.NetworkPolicy{}
	case CiliumNetworkPoliciesKind:
		return ciliumv1.EmptyCiliumUnstructuredPolicy()
	case NodeKind:
		return &corev1.Node{}
	}

	return nil
}
