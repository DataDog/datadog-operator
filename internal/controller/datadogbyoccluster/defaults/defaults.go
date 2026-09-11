// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaults

import (
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

// Apply returns a deep copy of the cluster with reconciliation-time defaults applied.
// DatadogBYOCCluster does not use a mutating admission webhook, so defaults that cannot be
// expressed in the shared CRD schema are applied here instead.
func Apply(cluster *datadoghqv1alpha1.DatadogBYOCCluster) *datadoghqv1alpha1.DatadogBYOCCluster {
	defaulted := cluster.DeepCopy()
	components := defaulted.Spec.Components
	applyIndexerDefaults(components.Indexer)
	applySearcherDefaults(components.Searcher)
	applyMetastoreDefaults(components.Metastore)
	applyComponentDefaults(components.ControlPlane, 1, deploymentResources())
	applyComponentDefaults(components.Janitor, 1, deploymentResources())
	if components.ReadOnlyMetastore != nil {
		applyMetastoreDefaults(components.ReadOnlyMetastore)
	}
	if components.Compactor != nil {
		applyCompactorDefaults(components.Compactor)
	}
	return defaulted
}

func applyIndexerDefaults(indexer *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec) {
	applyComponentDefaults(&indexer.DatadogBYOCClusterComponentSpec, 2, statefulResources())
	if indexer.TerminationGracePeriodSeconds == nil {
		indexer.TerminationGracePeriodSeconds = ptr.To[int64](300)
	}
	applyAutoscalingDefaults(indexer.Autoscaling, 70, 0, 300)

	if indexer.Storage == nil {
		indexer.Storage = &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
			VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
		}
	}
	if indexer.Storage.VolumeClaimTemplate == nil {
		return
	}

	claimSpec := &indexer.Storage.VolumeClaimTemplate.Spec
	if claimSpec.AccessModes == nil {
		claimSpec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}
	if claimSpec.Resources.Requests == nil {
		claimSpec.Resources.Requests = corev1.ResourceList{}
	}
	if _, found := claimSpec.Resources.Requests[corev1.ResourceStorage]; !found {
		claimSpec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse("250Gi")
	}
}

func applySearcherDefaults(searcher *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec) {
	applyComponentDefaults(&searcher.DatadogBYOCClusterComponentSpec, 2, statefulResources())
	applyAutoscalingDefaults(searcher.Autoscaling, 50, 60, 300)
	if searcher.Storage == nil {
		searcher.Storage = &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}
}

func applyMetastoreDefaults(metastore *datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec) {
	applyComponentDefaults(&metastore.DatadogBYOCClusterComponentSpec, 2, deploymentResources())
}

func applyCompactorDefaults(compactor *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec) {
	applyComponentDefaults(compactor, 1, nil)
	if compactor.TerminationGracePeriodSeconds == nil {
		compactor.TerminationGracePeriodSeconds = ptr.To[int64](60)
	}
}

func applyComponentDefaults(component *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec, replicas int32, resources *corev1.ResourceRequirements) {
	if component.Replicas == nil {
		component.Replicas = new(replicas)
	}
	if component.Resources == nil && resources != nil {
		component.Resources = resources.DeepCopy()
	}
}

func applyAutoscalingDefaults(autoscaling *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec, averageUtilization, scaleUpWindow, scaleDownWindow int32) {
	if autoscaling == nil {
		return
	}
	if autoscaling.MinReplicas == nil {
		autoscaling.MinReplicas = ptr.To[int32](2)
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
					AverageUtilization: new(averageUtilization),
				},
			},
		}}
	}
	if autoscaling.Behavior == nil {
		autoscaling.Behavior = &autoscalingv2.HorizontalPodAutoscalerBehavior{
			ScaleUp:   &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: new(scaleUpWindow)},
			ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: new(scaleDownWindow)},
		}
	}
}

func statefulResources() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("16Gi"),
		},
	}
}

func deploymentResources() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("2"),
			corev1.ResourceMemory: resource.MustParse("4Gi"),
		},
	}
}
