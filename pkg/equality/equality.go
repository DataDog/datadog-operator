// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package equality

import (
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// IsEqualObject return true if the two object are equal.
func IsEqualObject(kind kubernetes.ObjectKind, a, b client.Object) bool {
	if !IsEqualOperatorObjectMeta(a, b) {
		return false
	}

	switch kind {
	case kubernetes.ConfigMapKind:
		return IsEqualConfigMap(a, b)
	case kubernetes.ClusterRolesKind:
		return IsEqualClusterRoles(a, b)
	case kubernetes.ClusterRoleBindingKind:
		return IsEqualClusterRoleBinding(a, b)
	case kubernetes.RolesKind:
		return IsEqualRoles(a, b)
	case kubernetes.RoleBindingKind:
		return IsEqualRoleBinding(a, b)
	case kubernetes.ValidatingWebhookConfigurationsKind:
		return IsEqualValidatingWebhookConfigurations(a, b)
	case kubernetes.MutatingWebhookConfigurationsKind:
		return IsEqualMutatingWebhookConfigurations(a, b)
	case kubernetes.APIServiceKind:
		return IsEqualAPIService(a, b)
	case kubernetes.SecretsKind:
		return IsEqualSecrets(a, b)
	case kubernetes.ServicesKind:
		return IsEqualServices(a, b)
	case kubernetes.ServiceAccountsKind:
		return IsEqualServiceAccounts(a, b)
	case kubernetes.PodDisruptionBudgetsKind:
		return IsEqualPodDisruptionBudgets(a, b)
	case kubernetes.NetworkPoliciesKind:
		return IsEqualNetworkPolicies(a, b)
	case kubernetes.CiliumNetworkPoliciesKind:
		return IsEqualCiliumNetworkPolicies(a, b)
	default:
		return false
	}
}

// IsEqualConfigMap return true if the two ConfigMap are equal
func IsEqualConfigMap(a, b client.Object) bool {
	cmA, okA := a.(*corev1.ConfigMap)
	cmB, okB := b.(*corev1.ConfigMap)
	if okA && okB && cmA != nil && cmB != nil {
		if !apiutils.IsEqualStruct(cmA.Data, cmB.Data) {
			return false
		}
		if !apiutils.IsEqualStruct(cmA.BinaryData, cmA.BinaryData) {
			return false
		}
		return true
	}
	return false
}

// IsEqualClusterRoles return true if the two ClusterRole are equal
func IsEqualClusterRoles(a, b client.Object) bool {
	crA, okA := a.(*rbacv1.ClusterRole)
	crB, okB := b.(*rbacv1.ClusterRole)
	if okA && okB && crA != nil && crB != nil {
		return apiequality.Semantic.DeepEqual(crA.Rules, crB.Rules)
	}
	return false
}

// IsEqualClusterRoleBinding return true if the two ClusterRoleBinding are equal
func IsEqualClusterRoleBinding(objA, objB client.Object) bool {
	a, okA := objA.(*rbacv1.ClusterRoleBinding)
	b, okB := objB.(*rbacv1.ClusterRoleBinding)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.RoleRef, b.RoleRef) && apiequality.Semantic.DeepEqual(a.Subjects, b.Subjects)
	}
	return false
}

// IsEqualRoles return true if the two Roles are equal
func IsEqualRoles(objA, objB client.Object) bool {
	a, okA := objA.(*rbacv1.Role)
	b, okB := objB.(*rbacv1.Role)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.Rules, b.Rules)
	}
	return false
}

// IsEqualRoleBinding return true if the two RoleBinding are equal
func IsEqualRoleBinding(objA, objB client.Object) bool {
	a, okA := objA.(*rbacv1.RoleBinding)
	b, okB := objB.(*rbacv1.RoleBinding)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.RoleRef, b.RoleRef) && apiequality.Semantic.DeepEqual(a.Subjects, b.Subjects)
	}
	return false
}

// IsEqualValidatingWebhookConfigurations return true if the two ValidatingWebhookConfigurations are equal
func IsEqualValidatingWebhookConfigurations(objA, objB client.Object) bool {
	a, okA := objA.(*admissionregistrationv1.ValidatingWebhookConfiguration)
	b, okB := objB.(*admissionregistrationv1.ValidatingWebhookConfiguration)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.Webhooks, b.Webhooks)
	}
	return false
}

// IsEqualMutatingWebhookConfigurations return true if the two MutatingWebhookConfigurations are equal
func IsEqualMutatingWebhookConfigurations(objA, objB client.Object) bool {
	a, okA := objA.(*admissionregistrationv1.MutatingWebhookConfiguration)
	b, okB := objB.(*admissionregistrationv1.MutatingWebhookConfiguration)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.Webhooks, b.Webhooks)
	}
	return false
}

// IsEqualAPIService return true if the two APIService are equal
func IsEqualAPIService(objA, objB client.Object) bool {
	a, okA := objA.(*apiregistrationv1.APIService)
	b, okB := objB.(*apiregistrationv1.APIService)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.Spec, b.Spec)
	}
	return false
}

// IsEqualSecrets return true if the two Secrets are equal
func IsEqualSecrets(a, b client.Object) bool {
	sA, okA := a.(*corev1.Secret)
	sB, okB := b.(*corev1.Secret)
	if okA && okB && sA != nil && sB != nil {
		return apiutils.IsEqualStruct(sA.Data, sB.Data)
	}
	return false
}

// IsEqualServices return true if the two Services are equal
func IsEqualServices(objA, objB client.Object) bool {
	a, okA := objA.(*corev1.Service)
	b, okB := objB.(*corev1.Service)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.Spec, b.Spec)
	}
	return false
}

// IsEqualServiceAccounts return true if the two ServiceAccounts are equal
func IsEqualServiceAccounts(objA, objB client.Object) bool {
	a, okA := objA.(*corev1.ServiceAccount)
	b, okB := objB.(*corev1.ServiceAccount)
	if okA && okB && a != nil && b != nil {
		return true
	}
	return false
}

// IsEqualPodDisruptionBudgets return true if the two PodDisruptionBudgets are equal
func IsEqualPodDisruptionBudgets(objA, objB client.Object) bool {
	a, okA := objA.(*policyv1.PodDisruptionBudget)
	b, okB := objB.(*policyv1.PodDisruptionBudget)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.Spec, b.Spec)
	}
	return false
}

// IsEqualNetworkPolicies return true if the two NetworkPolicies are equal
func IsEqualNetworkPolicies(objA, objB client.Object) bool {
	a, okA := objA.(*networkingv1.NetworkPolicy)
	b, okB := objB.(*networkingv1.NetworkPolicy)
	if okA && okB && a != nil && b != nil {
		return apiequality.Semantic.DeepEqual(a.Spec, b.Spec)
	}
	return false
}

// IsEqualCiliumNetworkPolicies return true if the two CiliumNetworkPolicies are equal
func IsEqualCiliumNetworkPolicies(objA, objB client.Object) bool {
	unstructuredA, errA := runtime.DefaultUnstructuredConverter.ToUnstructured(objA)
	if errA != nil {
		return false
	}

	unstructuredB, errB := runtime.DefaultUnstructuredConverter.ToUnstructured(objB)
	if errB != nil {
		return false
	}

	return apiequality.Semantic.DeepEqual(unstructuredA["specs"], unstructuredB["specs"])
}

// IsEqualOperatorObjectMeta return true if the meta information added by the Operator are equal:
// Annotations, Labels, OwnerReference
func IsEqualOperatorObjectMeta(a, b metav1.Object) bool {
	if a.GetName() != b.GetName() || a.GetNamespace() != b.GetNamespace() {
		return false
	}
	return apiequality.Semantic.DeepEqual(a.GetOwnerReferences(), b.GetOwnerReferences()) && IsEqualOperatorAnnotations(a, b) && IsEqualOperatorLabels(a, b)
}

// IsEqualOperatorAnnotations use to check if Operator annotations are equal between 2 Objects
func IsEqualOperatorAnnotations(a, b metav1.Object) bool {
	return apiequality.Semantic.DeepEqual(a.GetAnnotations(), b.GetAnnotations())
}

// IsEqualOperatorLabels use to check if Operator labels are equal between 2 Objects
func IsEqualOperatorLabels(a, b metav1.Object) bool {
	return apiequality.Semantic.DeepEqual(a.GetLabels(), b.GetLabels())
}
