// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"fmt"
	"strings"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/constants"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	policyv1beta1 "k8s.io/api/policy/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/version"
)

// GetClusterAgentService returns the Cluster-Agent service
func GetClusterAgentService(dda metav1.Object) *corev1.Service {
	labels := object.GetDefaultLabels(dda, constants.DefaultClusterAgentResourceSuffix, GetClusterAgentVersion(dda))
	annotations := object.GetDefaultAnnotations(dda)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:        GetClusterAgentServiceName(dda),
			Namespace:   dda.GetNamespace(),
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				apicommon.AgentDeploymentNameLabelKey:      dda.GetName(),
				apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(common.DefaultClusterAgentServicePort),
					Port:       common.DefaultClusterAgentServicePort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	_, _ = comparison.SetMD5DatadogAgentGenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service
}

func GetClusterAgentPodDisruptionBudget(dda metav1.Object, useV1BetaPDB bool) client.Object {
	// labels and annotations
	minAvailableStr := intstr.FromInt(pdbMinAvailableInstances)
	matchLabels := map[string]string{
		apicommon.AgentDeploymentNameLabelKey:      dda.GetName(),
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix}
	if useV1BetaPDB {
		return &policyv1beta1.PodDisruptionBudget{
			ObjectMeta: metav1.ObjectMeta{
				Name:      GetClusterAgentPodDisruptionBudgetName(dda),
				Namespace: dda.GetNamespace(),
			},
			Spec: policyv1beta1.PodDisruptionBudgetSpec{
				MinAvailable: &minAvailableStr,
				Selector: &metav1.LabelSelector{
					MatchLabels: matchLabels,
				},
			},
		}
	}
	return &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:      GetClusterAgentPodDisruptionBudgetName(dda),
			Namespace: dda.GetNamespace(),
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable: &minAvailableStr,
			Selector: &metav1.LabelSelector{
				MatchLabels: matchLabels,
			},
		},
	}
}

// GetMetricsServerServiceName returns the external metrics provider service name
func GetMetricsServerServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), defaultMetricsServerResourceSuffix)
}

// GetMetricsServerAPIServiceName returns the external metrics provider apiservice name
func GetMetricsServerAPIServiceName() string {
	return v2alpha1.ExternalMetricsAPIServiceName
}

// GetDefaultExternalMetricSecretName returns the external metrics provider secret name
func GetDefaultExternalMetricSecretName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), "metrics-server")
}

// GetHPAClusterRoleBindingName returns a external metrics provider clusterrolebinding for auth-delegator
func GetHPAClusterRoleBindingName(dda metav1.Object) string {
	return fmt.Sprintf("%s-auth-delegator", GetClusterAgentRbacResourcesName(dda))
}

// GetExternalMetricsReaderClusterRoleName returns the name for the external metrics reader cluster role
func GetExternalMetricsReaderClusterRoleName(dda metav1.Object, versionInfo *version.Info) string {
	if versionInfo != nil && strings.Contains(versionInfo.GitVersion, "-gke.") {
		// For GKE clusters the name of the role is hardcoded and cannot be changed - HPA controller expects this name
		return "external-metrics-reader"
	}
	return fmt.Sprintf("%s-metrics-reader", GetClusterAgentRbacResourcesName(dda))
}

// GetResourceMetadataAsTagsClusterRoleName returns the name for the cluster role name used for kubernetes resource labels and annotations as tags
func GetResourceMetadataAsTagsClusterRoleName(dda metav1.Object) string {
	return fmt.Sprintf("%s-annotations-and-labels-as-tags", GetClusterAgentRbacResourcesName(dda))
}

// GetApiserverAuthReaderRoleBindingName returns the name for the role binding to access the extension-apiserver-authentication cm
func GetApiserverAuthReaderRoleBindingName(dda metav1.Object) string {
	return fmt.Sprintf("%s-apiserver", GetClusterAgentRbacResourcesName(dda))
}
