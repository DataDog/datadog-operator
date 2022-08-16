// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clusteragent

import (
	"fmt"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// GetClusterAgentServiceName return the Cluster-Agent service name based on the DatadogAgent name
func GetClusterAgentServiceName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentName return the Cluster-Agent name based on the DatadogAgent name
func GetClusterAgentName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentVersion return the Cluster-Agent version based on the DatadogAgent info
func GetClusterAgentVersion(dda metav1.Object) string {
	// Todo implement this function
	return ""
}

// GetClusterAgentRbacResourcesName return the Cluster-Agent RBAC resource name
func GetClusterAgentRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), apicommon.DefaultClusterAgentResourceSuffix)
}

// GetClusterAgentService returns the Cluster-Agent service
func GetClusterAgentService(dda metav1.Object) *corev1.Service {
	labels := object.GetDefaultLabels(dda, apicommon.DefaultClusterAgentResourceSuffix, GetClusterAgentVersion(dda))
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
				apicommon.AgentDeploymentComponentLabelKey: apicommon.DefaultClusterAgentResourceSuffix,
			},
			Ports: []corev1.ServicePort{
				{
					Protocol:   corev1.ProtocolTCP,
					TargetPort: intstr.FromInt(apicommon.DefaultClusterAgentServicePort),
					Port:       apicommon.DefaultClusterAgentServicePort,
				},
			},
			SessionAffinity: corev1.ServiceAffinityNone,
		},
	}
	_, _ = comparison.SetMD5DatadogAgentGenerationAnnotation(&service.ObjectMeta, &service.Spec)

	return service
}
