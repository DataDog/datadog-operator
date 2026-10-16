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

const (
	// Replicas
	defaultIndexerReplicas      int32 = 2
	defaultSearcherReplicas     int32 = 2
	defaultPipelineReplicas     int32 = 2
	defaultMetastoreReplicas    int32 = 2
	defaultControlPlaneReplicas int32 = 1
	defaultJanitorReplicas      int32 = 1
	defaultCompactorReplicas    int32 = 1

	// Termination grace periods
	defaultIndexerTerminationGracePeriodSeconds   int64 = 300
	defaultPipelineTerminationGracePeriodSeconds  int64 = 70
	defaultCompactorTerminationGracePeriodSeconds int64 = 60

	// Autoscaling
	defaultAutoscalingMinReplicas         int32 = 2
	defaultAutoscalingMaxReplicas         int32 = 10
	defaultIndexerTargetCPUUtilization    int32 = 70
	defaultIndexerScaleUpWindowSeconds    int32 = 0
	defaultIndexerScaleDownWindowSeconds  int32 = 300
	defaultPipelineTargetCPUUtilization   int32 = 70
	defaultPipelineScaleUpWindowSeconds   int32 = 0
	defaultPipelineScaleDownWindowSeconds int32 = 300
	defaultSearcherTargetCPUUtilization   int32 = 50
	defaultSearcherScaleUpWindowSeconds   int32 = 60
	defaultSearcherScaleDownWindowSeconds int32 = 300

	// Storage
	defaultStorageSize = "30Gi"

	// Resources
	defaultStatefulCPURequest   = "4"
	defaultStatefulMemory       = "16Gi"
	defaultDeploymentCPURequest = "2"
	defaultDeploymentMemory     = "4Gi"
	defaultPipelineCPURequest   = "2"
	defaultPipelineMemory       = "4Gi"
)

// Apply returns a deep copy of the cluster with reconciliation-time defaults applied.
// DatadogBYOCCluster does not use a mutating admission webhook, so defaults that cannot be
// expressed in the shared CRD schema are applied here instead.
func Apply(cluster *datadoghqv1alpha1.DatadogBYOCCluster) *datadoghqv1alpha1.DatadogBYOCCluster {
	defaulted := cluster.DeepCopy()
	components := defaulted.Spec.Components
	applyIndexerDefaults(components.Indexer)
	applySearcherDefaults(components.Searcher)
	for i := range components.Pipelines {
		applyPipelineDefaults(&components.Pipelines[i].DatadogBYOCClusterPipelineComponentSpec)
	}
	applyMetastoreDefaults(components.Metastore)
	applyComponentDefaults(components.ControlPlane, defaultControlPlaneReplicas, deploymentResources())
	applyComponentDefaults(components.Janitor, defaultJanitorReplicas, deploymentResources())
	if components.ReadOnlyMetastore != nil {
		applyMetastoreDefaults(components.ReadOnlyMetastore)
	}
	if components.Compactor != nil {
		applyCompactorDefaults(components.Compactor)
	}
	return defaulted
}

func applyPipelineDefaults(pipeline *datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec) {
	applyComponentDefaults(&pipeline.DatadogBYOCClusterComponentSpec, defaultPipelineReplicas, pipelineResources())
	if pipeline.TerminationGracePeriodSeconds == nil {
		pipeline.TerminationGracePeriodSeconds = new(defaultPipelineTerminationGracePeriodSeconds)
	}
	applyAutoscalingDefaults(pipeline.Autoscaling, defaultPipelineTargetCPUUtilization, defaultPipelineScaleUpWindowSeconds, defaultPipelineScaleDownWindowSeconds)

	if pipeline.Storage == nil {
		pipeline.Storage = &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
			VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
		}
	}
	if pipeline.Storage.VolumeClaimTemplate == nil {
		return
	}

	claimSpec := &pipeline.Storage.VolumeClaimTemplate.Spec
	if claimSpec.AccessModes == nil {
		claimSpec.AccessModes = []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce}
	}
	if claimSpec.Resources.Requests == nil {
		claimSpec.Resources.Requests = corev1.ResourceList{}
	}
	if _, found := claimSpec.Resources.Requests[corev1.ResourceStorage]; !found {
		claimSpec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(defaultStorageSize)
	}
}

func applyIndexerDefaults(indexer *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec) {
	applyComponentDefaults(&indexer.DatadogBYOCClusterComponentSpec, defaultIndexerReplicas, statefulResources())
	if indexer.TerminationGracePeriodSeconds == nil {
		indexer.TerminationGracePeriodSeconds = new(defaultIndexerTerminationGracePeriodSeconds)
	}
	applyAutoscalingDefaults(indexer.Autoscaling, defaultIndexerTargetCPUUtilization, defaultIndexerScaleUpWindowSeconds, defaultIndexerScaleDownWindowSeconds)

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
		claimSpec.Resources.Requests[corev1.ResourceStorage] = resource.MustParse(defaultStorageSize)
	}
}

func applySearcherDefaults(searcher *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec) {
	applyComponentDefaults(&searcher.DatadogBYOCClusterComponentSpec, defaultSearcherReplicas, statefulResources())
	applyAutoscalingDefaults(searcher.Autoscaling, defaultSearcherTargetCPUUtilization, defaultSearcherScaleUpWindowSeconds, defaultSearcherScaleDownWindowSeconds)
	if searcher.Storage == nil {
		searcher.Storage = &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		}
	}
}

func applyMetastoreDefaults(metastore *datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec) {
	applyComponentDefaults(&metastore.DatadogBYOCClusterComponentSpec, defaultMetastoreReplicas, deploymentResources())
}

func applyCompactorDefaults(compactor *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec) {
	applyComponentDefaults(compactor, defaultCompactorReplicas, nil)
	if compactor.TerminationGracePeriodSeconds == nil {
		compactor.TerminationGracePeriodSeconds = new(defaultCompactorTerminationGracePeriodSeconds)
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
			corev1.ResourceMemory: resource.MustParse(defaultStatefulMemory),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(defaultStatefulCPURequest),
			corev1.ResourceMemory: resource.MustParse(defaultStatefulMemory),
		},
	}
}

func deploymentResources() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(defaultDeploymentMemory),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(defaultDeploymentCPURequest),
			corev1.ResourceMemory: resource.MustParse(defaultDeploymentMemory),
		},
	}
}

func pipelineResources() *corev1.ResourceRequirements {
	return &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(defaultPipelineMemory),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(defaultPipelineCPURequest),
			corev1.ResourceMemory: resource.MustParse(defaultPipelineMemory),
		},
	}
}
