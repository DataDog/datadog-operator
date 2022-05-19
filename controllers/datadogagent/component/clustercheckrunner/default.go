// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package clustercheckrunner

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
)

// NewDefaultClusterCheckRunnerDeployment return a new default cluster-check-runner deployment
func NewDefaultClusterCheckRunnerDeployment(dda metav1.Object) *appsv1.Deployment {
	deployment := component.NewDeployment(dda, apicommon.DefaultClusterChecksRunnerResourceSuffix, component.GetClusterCheckRunnerName(dda), component.GetAgentVersion(dda), nil)

	podTemplate := NewDefaultClusterCheckRunnerPodTemplateSpec(dda)
	for key, val := range deployment.GetLabels() {
		podTemplate.Labels[key] = val
	}

	for key, val := range deployment.GetAnnotations() {
		podTemplate.Annotations[key] = val
	}

	deployment.Spec.Template = *podTemplate
	deployment.Spec.Replicas = apiutils.NewInt32Pointer(apicommon.DefaultClusterAgentReplicas)

	return deployment
}

// NewDefaultClusterCheckRunnerPodTemplateSpec return a default cluster-check-runner for the cluster-agent deployment
func NewDefaultClusterCheckRunnerPodTemplateSpec(dda metav1.Object) *corev1.PodTemplateSpec {
	// TODO(operator-ga): implement NewDefaultClusterCheckRunnerPodTemplateSpec function
	template := &corev1.PodTemplateSpec{}

	return template
}
