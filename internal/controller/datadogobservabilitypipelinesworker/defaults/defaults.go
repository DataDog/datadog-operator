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

// Apply returns a deep copy of the worker with reconciliation-time defaults applied.
func Apply(worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker) *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker {
	defaulted := worker.DeepCopy()
	component := &defaulted.Spec.DatadogBYOCClusterPipelineComponentSpec

	if component.Replicas == nil {
		component.Replicas = ptr.To[int32](1)
	}
	if component.Resources == nil {
		component.Resources = &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("4Gi"),
			},
		}
	}
	if component.TerminationGracePeriodSeconds == nil {
		component.TerminationGracePeriodSeconds = ptr.To[int64](70)
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
	if autoscaling.MinReplicas == nil {
		autoscaling.MinReplicas = ptr.To[int32](1)
	}
	if autoscaling.MaxReplicas == nil {
		autoscaling.MaxReplicas = ptr.To[int32](10)
	}
	if len(autoscaling.Metrics) == 0 {
		autoscaling.Metrics = []autoscalingv2.MetricSpec{{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: ptr.To[int32](80),
				},
			},
		}}
	}
}
