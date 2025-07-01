// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1beta1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// TestScheme return a runtime.Scheme for testing purposes
func TestScheme() *runtime.Scheme {
	s := scheme.Scheme
	s.AddKnownTypes(edsdatadoghqv1alpha1.GroupVersion, &edsdatadoghqv1alpha1.ExtendedDaemonSet{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentProfile{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentProfileList{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.DaemonSet{})
	s.AddKnownTypes(appsv1.SchemeGroupVersion, &appsv1.Deployment{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Secret{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ServiceAccount{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.ConfigMap{})
	s.AddKnownTypes(corev1.SchemeGroupVersion, &corev1.Node{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRoleBinding{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.ClusterRole{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.Role{})
	s.AddKnownTypes(rbacv1.SchemeGroupVersion, &rbacv1.RoleBinding{})
	s.AddKnownTypes(policyv1.SchemeGroupVersion, &policyv1.PodDisruptionBudget{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIServiceList{})
	s.AddKnownTypes(apiregistrationv1.SchemeGroupVersion, &apiregistrationv1.APIService{})
	s.AddKnownTypes(networkingv1.SchemeGroupVersion, &networkingv1.NetworkPolicy{})
	s.AddKnownTypes(v2alpha1.GroupVersion, &v2alpha1.DatadogAgent{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternal{})
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogAgentInternalList{})
	return s
}
