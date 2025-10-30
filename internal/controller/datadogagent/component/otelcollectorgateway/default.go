// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelcollectorgateway

import (
	"fmt"
	"maps"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

// GetOtelCollectorGatewayName return the OtelCollectorGateway name based on the DatadogAgent name
func GetOtelCollectorGatewayName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultOtelCollectorGatewayResourceSuffix)
}

// GetOtelCollectorGatewayRbacResourcesName returns the OtelCollectorGateway RBAC resource name
func GetOtelCollectorGatewayRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultOtelCollectorGatewayResourceSuffix)
}

// NewDefaultOtelCollectorGatewayDeployment return a new default otel-collector-gateway deployment
func NewDefaultOtelCollectorGatewayDeployment(dda metav1.Object) *appsv1.Deployment {
	deployment := common.NewDeployment(dda, constants.DefaultOtelCollectorGatewayResourceSuffix, GetOtelCollectorGatewayName(dda), common.GetAgentVersion(dda), nil)

	podTemplate := NewDefaultOtelCollectorGatewayPodTemplateSpec(dda)
	maps.Copy(podTemplate.Labels, deployment.GetLabels())
	maps.Copy(podTemplate.Annotations, deployment.GetAnnotations())

	deployment.Spec.Template = *podTemplate
	deployment.Spec.Replicas = apiutils.NewInt32Pointer(defaultOtelCollectorGatewayReplicas)

	return deployment
}

// NewDefaultOtelCollectorGatewayPodTemplateSpec returns a default otel-collector-gateway pod template spec
func NewDefaultOtelCollectorGatewayPodTemplateSpec(dda metav1.Object) *corev1.PodTemplateSpec {
	template := &corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      make(map[string]string),
			Annotations: make(map[string]string),
		},
		Spec: defaultPodSpec(dda),
	}

	return template
}

func defaultPodSpec(dda metav1.Object) corev1.PodSpec {
	return corev1.PodSpec{
		ServiceAccountName: GetOtelCollectorGatewayRbacResourcesName(dda),
		Containers: []corev1.Container{
			{
				Name:    string(apicommon.OtelCollectorGatewayContainerName),
				Image:   "docker.io/library/busybox:latest",
				Command: []string{"/bin/sh"},
				Args:    []string{"-c", "while true; do date; sleep 10; done"},
			},
		},
	}
}
