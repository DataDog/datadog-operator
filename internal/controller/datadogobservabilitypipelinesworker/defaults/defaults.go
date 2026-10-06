// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package defaults applies reconciliation-time defaults to Observability Pipelines Workers.
package defaults

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	defaultReplicas                      int32 = 1
	defaultTerminationGracePeriodSeconds int64 = 70

	// Autoscaling
	defaultAutoscalingMinReplicas int32 = 1
	defaultAutoscalingMaxReplicas int32 = 10
	defaultTargetCPUUtilization   int32 = 80

	// Resources
	defaultCPURequest    = "2"
	defaultMemoryRequest = "4Gi"
)

// Apply returns a deep copy of the worker with reconciliation-time defaults applied.
func Apply(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker {
	defaulted := worker.DeepCopy()
	component := &defaulted.Spec.DatadogBYOCClusterPipelineComponentSpec

	if component.Replicas == nil {
		component.Replicas = new(defaultReplicas)
	}
	if component.Resources == nil {
		component.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(defaultCPURequest),
				corev1.ResourceMemory: resource.MustParse(defaultMemoryRequest),
			},
		}
	}
	if component.TerminationGracePeriodSeconds == nil {
		component.TerminationGracePeriodSeconds = new(defaultTerminationGracePeriodSeconds)
	}
	applyAutoscalingDefaults(component.Autoscaling)

	if component.Storage == nil {
		component.Storage = &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}
	return defaulted
}

func applyAutoscalingDefaults(autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec) {
	if autoscaling == nil {
		return
	}
	// Keep an omitted bound consistent with the configured bound.
	if autoscaling.MinReplicas == nil {
		autoscaling.MinReplicas = new(min(defaultAutoscalingMinReplicas, ptr.Deref(autoscaling.MaxReplicas, defaultAutoscalingMinReplicas)))
	}
	if autoscaling.MaxReplicas == nil {
		autoscaling.MaxReplicas = new(max(defaultAutoscalingMaxReplicas, *autoscaling.MinReplicas))
	}
	if len(autoscaling.Metrics) == 0 {
		autoscaling.Metrics = []autoscalingv2.MetricSpec{{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: new(defaultTargetCPUUtilization),
				},
			},
		}}
	}
}
