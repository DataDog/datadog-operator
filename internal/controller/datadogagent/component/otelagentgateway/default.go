// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package otelagentgateway

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
	"github.com/DataDog/datadog-operator/pkg/images"
)

// GetOtelAgentGatewayName return the OtelAgentGateway name based on the DatadogAgent name
func GetOtelAgentGatewayName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultOtelAgentGatewayResourceSuffix)
}

// GetOtelAgentGatewayRbacResourcesName returns the OtelAgentGateway RBAC resource name
func GetOtelAgentGatewayRbacResourcesName(dda metav1.Object) string {
	return fmt.Sprintf("%s-%s", dda.GetName(), constants.DefaultOtelAgentGatewayResourceSuffix)
}

// NewDefaultOtelAgentGatewayDeployment return a new default otel-collector-gateway deployment
func NewDefaultOtelAgentGatewayDeployment(dda metav1.Object) *appsv1.Deployment {
	deployment := common.NewDeployment(dda, constants.DefaultOtelAgentGatewayResourceSuffix, GetOtelAgentGatewayName(dda), common.GetAgentVersion(dda), nil)

	podTemplate := NewDefaultOtelAgentGatewayPodTemplateSpec(dda)
	maps.Copy(podTemplate.Labels, deployment.GetLabels())
	maps.Copy(podTemplate.Annotations, deployment.GetAnnotations())

	deployment.Spec.Template = *podTemplate
	deployment.Spec.Replicas = apiutils.NewInt32Pointer(defaultOtelAgentGatewayReplicas)

	return deployment
}

// NewDefaultOtelAgentGatewayPodTemplateSpec returns a default otel-collector-gateway pod template spec
func NewDefaultOtelAgentGatewayPodTemplateSpec(dda metav1.Object) *corev1.PodTemplateSpec {
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
		ServiceAccountName: GetOtelAgentGatewayRbacResourcesName(dda),
		Containers: []corev1.Container{
			{
				Name:    string(apicommon.OtelAgent),
				Image:   images.GetLatestDdotCollectorImage(),
				Command: []string{"otel-agent", "--sync-delay=30s"},
				VolumeMounts: []corev1.VolumeMount{
					common.GetVolumeMountForLogs(),
				},
				Ports: []corev1.ContainerPort{
					{
						Name:          "otel-grpc",
						ContainerPort: 4317,
						Protocol:      corev1.ProtocolTCP,
					},
					{
						Name:          "otel-http",
						ContainerPort: 4318,
						Protocol:      corev1.ProtocolTCP,
					},
				},
			},
		},
		Volumes: []corev1.Volume{
			common.GetVolumeForLogs(),
		},
	}
}
