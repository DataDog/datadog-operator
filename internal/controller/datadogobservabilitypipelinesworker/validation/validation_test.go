// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package validation

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestValidateWorkerSpec(t *testing.T) {
	tests := []struct {
		name string
		spec datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec
		want []string
	}{
		{
			name: "missing image",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{},
			want: []string{"spec.image: image must be specified"},
		},
		{
			name: "empty image",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{},
			},
			want: []string{
				"spec.image.repository: repository must be specified",
				"spec.image: exactly one of tag or digest must be specified",
			},
		},
		{
			name: "missing repository",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Tag: ptr.To("v1"),
				},
			},
			want: []string{"spec.image.repository: repository must be specified"},
		},
		{
			name: "missing version",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
				},
			},
			want: []string{"spec.image: exactly one of tag or digest must be specified"},
		},
		{
			name: "both tag and digest",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Tag:        ptr.To("v1"),
					Digest:     ptr.To("sha256:custom"),
				},
			},
			want: []string{"spec.image: exactly one of tag or digest must be specified"},
		},
		{
			name: "empty digest without tag",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Digest:     ptr.To(""),
				},
			},
			want: []string{"spec.image.digest: digest must be non-empty"},
		},
		{
			name: "empty digest still conflicts with tag",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Tag:        ptr.To("v1"),
					Digest:     ptr.To(""),
				},
			},
			want: []string{"spec.image: exactly one of tag or digest must be specified"},
		},
		{
			name: "image with tag",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Tag:        ptr.To("v1"),
				},
			},
			want: nil,
		},
		{
			name: "image with digest",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Digest:     ptr.To("sha512:custom"),
				},
			},
			want: nil,
		},
		{
			name: "valid component settings",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							Resources: &corev1.ResourceRequirements{
								Limits: corev1.ResourceList{corev1.ResourceMemory: resource.MustParse("1Gi")},
							},
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable: ptr.To(intstr.FromInt32(1)),
							},
						},
						Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
							EmptyDir: &corev1.EmptyDirVolumeSource{},
						},
					},
				},
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Tag:        ptr.To("v1"),
				},
			},
			want: nil,
		},
		{
			name: "component and image errors are combined",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							Resources: &corev1.ResourceRequirements{},
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable:   ptr.To(intstr.FromInt32(1)),
								MaxUnavailable: ptr.To(intstr.FromInt32(1)),
							},
						},
						Autoscaling: &datadoghqv1alpha1.DatadogBYOCClusterAutoscalingSpec{
							MinReplicas: ptr.To[int32](5),
							MaxReplicas: ptr.To[int32](2),
						},
						Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{},
					},
				},
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{},
			},
			want: []string{
				"spec.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.autoscaling.maxReplicas: must be greater than or equal to minReplicas (5)",
				"spec.storage: exactly one storage type must be specified",
				"spec.image.repository: repository must be specified",
				"spec.image: exactly one of tag or digest must be specified",
			},
		},
		{
			name: "reserved names",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					Ports: []datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerPort{
						{Name: "api", Port: 9000},
						{Name: "otlp-http", Port: 8686},
					},
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							InitContainers: []corev1.Container{{Name: "worker"}},
							Volumes:        []corev1.Volume{{Name: "data"}},
							VolumeMounts:   []corev1.VolumeMount{{Name: "certificates", MountPath: "/var/lib/observability-pipelines-worker/"}},
						},
					},
				},
				Image: &datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerImageSpec{
					Repository: ptr.To("registry.example.com/worker"),
					Tag:        ptr.To("v1"),
				},
			},
			want: []string{
				"spec.ports[0].name: is reserved for the Worker API port",
				"spec.ports[1].port: is reserved for the Worker API port",
				"spec.initContainers[0].name: is reserved for the Worker container",
				"spec.volumes[0].name: is reserved for a built-in volume",
				"spec.volumeMounts[0].mountPath: is reserved for a built-in volume mount",
			},
		},
		{
			name: "missing image preserves component errors",
			spec: datadoghqv1alpha1.DatadogObservabilityPipelinesWorkerSpec{
				DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
					DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{},
					},
				},
			},
			want: []string{
				"spec.storage: exactly one storage type must be specified",
				"spec.image: image must be specified",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []string
			for _, err := range ValidateWorkerSpec(&tt.spec) {
				got = append(got, err.Field+": "+err.Detail)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("validation errors (-want +got):\n%s", diff)
			}
		})
	}
}
