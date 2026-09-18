// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package defaults

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestApply(t *testing.T) {
	tests := []struct {
		name   string
		worker *datadoghqv1alpha1.DatadogObservabilityPipelinesWorker
		want   datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec
	}{
		{
			name:   "defaults omitted values",
			worker: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{},
			want: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
					DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						Replicas: ptr.To[int32](1),
						Resources: &corev1.ResourceRequirements{Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						}},
						TerminationGracePeriodSeconds: ptr.To[int64](70),
					},
					Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
			},
		},
		{
			name: "defaults enabled autoscaling",
			worker: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{},
					},
				},
			}},
			want: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
					DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						Replicas: ptr.To[int32](1),
						Resources: &corev1.ResourceRequirements{Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("2"),
							corev1.ResourceMemory: resource.MustParse("4Gi"),
						}},
						TerminationGracePeriodSeconds: ptr.To[int64](70),
					},
					Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
						MinReplicas: ptr.To[int32](1),
						MaxReplicas: ptr.To[int32](10),
						Metrics: []autoscalingv2.MetricSpec{{
							Type: autoscalingv2.ResourceMetricSourceType,
							Resource: &autoscalingv2.ResourceMetricSource{
								Name: corev1.ResourceCPU,
								Target: autoscalingv2.MetricTarget{
									Type:               autoscalingv2.UtilizationMetricType,
									AverageUtilization: ptr.To[int32](80),
								},
							},
						}},
					},
					Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}},
				},
			},
		},
		{
			name: "preserves configured values",
			worker: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorker{Spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							Replicas: ptr.To[int32](4),
							Resources: &corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
							},
							TerminationGracePeriodSeconds: ptr.To[int64](90),
						},
						Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
							MinReplicas: ptr.To[int32](3),
							MaxReplicas: ptr.To[int32](20),
							Metrics:     []autoscalingv2.MetricSpec{{Type: autoscalingv2.ExternalMetricSourceType}},
							Behavior:    &autoscalingv2.HorizontalPodAutoscalerBehavior{},
						},
						Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
							VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
						},
					},
				},
			}},
			want: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
				DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
					DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						Replicas: ptr.To[int32](4),
						Resources: &corev1.ResourceRequirements{
							Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("8Gi")},
						},
						TerminationGracePeriodSeconds: ptr.To[int64](90),
					},
					Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
						MinReplicas: ptr.To[int32](3),
						MaxReplicas: ptr.To[int32](20),
						Metrics:     []autoscalingv2.MetricSpec{{Type: autoscalingv2.ExternalMetricSourceType}},
						Behavior:    &autoscalingv2.HorizontalPodAutoscalerBehavior{},
					},
					Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
						VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Apply(tt.worker)
			if diff := cmp.Diff(tt.want, got.Spec.DatadogBYOCClusterPipelineComponentSpec); diff != "" {
				t.Errorf("Apply() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
