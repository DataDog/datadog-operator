// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed by Datadog (https://www.datadoghq.com/).
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
	cluster := testCluster()
	original := cluster.DeepCopy()
	want := &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
		Metastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
			DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas: ptr.To[int32](2),
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				},
			},
		},
		ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
			DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas: ptr.To[int32](2),
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("2"),
						corev1.ResourceMemory: resource.MustParse("4Gi"),
					},
				},
			},
		},
		Indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
			DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas: ptr.To[int32](2),
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
				TerminationGracePeriodSeconds: ptr.To[int64](300),
			},
			Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
				VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("250Gi")},
						},
					},
				},
			},
		},
		Searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
			DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas: ptr.To[int32](2),
				Resources: &corev1.ResourceRequirements{
					Limits: corev1.ResourceList{
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: resource.MustParse("16Gi"),
					},
				},
			},
			Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}},
		},
		ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
			Replicas: ptr.To[int32](1),
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
		Compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
			Replicas:                      ptr.To[int32](1),
			TerminationGracePeriodSeconds: ptr.To[int64](60),
		},
		Janitor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
			Replicas: ptr.To[int32](1),
			Resources: &corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
				Requests: corev1.ResourceList{
					corev1.ResourceCPU:    resource.MustParse("2"),
					corev1.ResourceMemory: resource.MustParse("4Gi"),
				},
			},
		},
	}

	got := Apply(cluster)

	if diff := cmp.Diff(want, got.Spec.Components); diff != "" {
		t.Errorf("Apply() components mismatch (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(original, cluster); diff != "" {
		t.Errorf("Apply() modified its input (-want +got):\n%s", diff)
	}
	if diff := cmp.Diff(got, Apply(got)); diff != "" {
		t.Errorf("Apply() is not idempotent (-want +got):\n%s", diff)
	}
}

func TestApplyIndexerDefaults(t *testing.T) {
	defaultComponent := datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
		Replicas:                      ptr.To[int32](2),
		Resources:                     statefulResources(),
		TerminationGracePeriodSeconds: ptr.To[int64](300),
	}
	defaultStorage := &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
		VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("250Gi")},
				},
			},
		},
	}
	customStorage := &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
		VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{
			DatadogBYOCClusterEmbeddedObjectMetadata: datadoghqv1alpha1.DatadogBYOCClusterEmbeddedObjectMetadata{
				Annotations: map[string]string{"example.com/storage": "custom"},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteMany},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{corev1.ResourceStorage: resource.MustParse("500Gi")},
				},
			},
		},
	}
	tests := []struct {
		name    string
		indexer *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
		want    *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
	}{
		{
			name:    "defaults",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
			want: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: defaultComponent,
				Storage:                         defaultStorage,
			},
		},
		{
			name: "preserves custom values",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					Replicas:                      ptr.To[int32](4),
					Resources:                     &corev1.ResourceRequirements{},
					TerminationGracePeriodSeconds: ptr.To[int64](600),
				},
				Storage: customStorage.DeepCopy(),
			},
			want: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					Replicas:                      ptr.To[int32](4),
					Resources:                     &corev1.ResourceRequirements{},
					TerminationGracePeriodSeconds: ptr.To[int64](600),
				},
				Storage: customStorage.DeepCopy(),
			},
		},
		{
			name: "empty dir",
			indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
			want: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: defaultComponent,
				Storage:                         &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{EmptyDir: &corev1.EmptyDirVolumeSource{}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyIndexerDefaults(tt.indexer)
			if diff := cmp.Diff(tt.want, tt.indexer); diff != "" {
				t.Errorf("applyIndexerDefaults() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplySearcherDefaults(t *testing.T) {
	defaultComponent := datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
		Replicas:  ptr.To[int32](2),
		Resources: statefulResources(),
	}
	tests := []struct {
		name     string
		searcher *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
		want     *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
	}{
		{
			name:     "defaults",
			searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
			want: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: defaultComponent,
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
		},
		{
			name: "preserves volume claim template",
			searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
				},
			},
			want: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: defaultComponent,
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applySearcherDefaults(tt.searcher)
			if diff := cmp.Diff(tt.want, tt.searcher); diff != "" {
				t.Errorf("applySearcherDefaults() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyComponentDefaults(t *testing.T) {
	customResources := &corev1.ResourceRequirements{
		Requests: corev1.ResourceList{corev1.ResourceCPU: resource.MustParse("8")},
	}
	tests := []struct {
		name             string
		component        *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
		defaultReplicas  int32
		defaultResources *corev1.ResourceRequirements
		want             *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
	}{
		{
			name:             "defaults",
			component:        &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			defaultReplicas:  2,
			defaultResources: deploymentResources(),
			want: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas:  ptr.To[int32](2),
				Resources: deploymentResources(),
			},
		},
		{
			name: "preserves custom values",
			component: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas:  ptr.To[int32](3),
				Resources: customResources.DeepCopy(),
			},
			defaultReplicas:  2,
			defaultResources: deploymentResources(),
			want: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas:  ptr.To[int32](3),
				Resources: customResources.DeepCopy(),
			},
		},
		{
			name:            "no resource default",
			component:       &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			defaultReplicas: 1,
			want:            &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{Replicas: ptr.To[int32](1)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyComponentDefaults(tt.component, tt.defaultReplicas, tt.defaultResources)
			if diff := cmp.Diff(tt.want, tt.component); diff != "" {
				t.Errorf("applyComponentDefaults() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyCompactorDefaults(t *testing.T) {
	tests := []struct {
		name      string
		compactor *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
		want      *datadoghqv1alpha1.DatadogBYOCClusterComponentSpec
	}{
		{
			name:      "defaults",
			compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			want: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas:                      ptr.To[int32](1),
				TerminationGracePeriodSeconds: ptr.To[int64](60),
			},
		},
		{
			name: "preserves custom values",
			compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas:                      ptr.To[int32](3),
				TerminationGracePeriodSeconds: ptr.To[int64](120),
			},
			want: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
				Replicas:                      ptr.To[int32](3),
				TerminationGracePeriodSeconds: ptr.To[int64](120),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyCompactorDefaults(tt.compactor)
			if diff := cmp.Diff(tt.want, tt.compactor); diff != "" {
				t.Errorf("applyCompactorDefaults() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestApplyAutoscalingDefaults(t *testing.T) {
	custom := &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
		MinReplicas: ptr.To[int32](3),
		MaxReplicas: ptr.To[int32](20),
		Metrics:     []autoscalingv2.MetricSpec{{Type: autoscalingv2.ExternalMetricSourceType}},
		Behavior:    &autoscalingv2.HorizontalPodAutoscalerBehavior{},
	}
	tests := []struct {
		name               string
		autoscaling        *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec
		averageUtilization int32
		scaleUpWindow      int32
		scaleDownWindow    int32
		want               *datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec
	}{
		{
			name:               "defaults",
			autoscaling:        &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{},
			averageUtilization: 70,
			scaleUpWindow:      0,
			scaleDownWindow:    300,
			want: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
				MinReplicas: ptr.To[int32](2),
				MaxReplicas: ptr.To[int32](10),
				Metrics: []autoscalingv2.MetricSpec{{
					Type: autoscalingv2.ResourceMetricSourceType,
					Resource: &autoscalingv2.ResourceMetricSource{
						Name: corev1.ResourceCPU,
						Target: autoscalingv2.MetricTarget{
							Type:               autoscalingv2.UtilizationMetricType,
							AverageUtilization: ptr.To[int32](70),
						},
					},
				}},
				Behavior: &autoscalingv2.HorizontalPodAutoscalerBehavior{
					ScaleUp:   &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To[int32](0)},
					ScaleDown: &autoscalingv2.HPAScalingRules{StabilizationWindowSeconds: ptr.To[int32](300)},
				},
			},
		},
		{
			name:               "preserves custom values",
			autoscaling:        custom.DeepCopy(),
			averageUtilization: 70,
			scaleUpWindow:      0,
			scaleDownWindow:    300,
			want:               custom.DeepCopy(),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applyAutoscalingDefaults(tt.autoscaling, tt.averageUtilization, tt.scaleUpWindow, tt.scaleDownWindow)
			if diff := cmp.Diff(tt.want, tt.autoscaling); diff != "" {
				t.Errorf("applyAutoscalingDefaults() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func testCluster() *datadoghqv1alpha1.DatadogBYOCCluster {
	return &datadoghqv1alpha1.DatadogBYOCCluster{
		Spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
			Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{
				Metastore:         &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
				ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{},
				Indexer:           &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
				Searcher:          &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
				ControlPlane:      &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
				Compactor:         &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
				Janitor:           &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{},
			},
		},
	}
}
