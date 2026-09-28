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
	"k8s.io/apimachinery/pkg/util/validation/field"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestValidateClusterSpec(t *testing.T) {
	tests := []struct {
		name string
		spec datadoghqv1alpha1.DatadogBYOCClusterSpec
		want []string
	}{
		{
			name: "release tag",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To("v1"),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: nil,
		},

		{
			name: "release digest",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Digest: ptr.To("sha512:custom"),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: nil,
		},

		{
			name: "no release or overrides",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: []string{
				"spec: release or imageOverrides must be specified",
			},
		},

		{
			name: "release with neither version",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: []string{
				"spec.release: exactly one of tag or digest must be specified",
			},
		},

		{
			name: "release with both versions",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag:    ptr.To("v1"),
					Digest: ptr.To("sha256:custom"),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: []string{
				"spec.release: exactly one of tag or digest must be specified",
			},
		},

		{
			name: "release uses field presence",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To(""),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: nil,
		},

		{
			name: "missing provider",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To("v1"),
				},
			},
			want: []string{
				"spec.provider.aws: at least one provider must be specified",
			},
		},

		{
			name: "empty provider",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To("v1"),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{},
			},
			want: []string{
				"spec.provider.aws: at least one provider must be specified",
			},
		},

		{
			name: "empty overrides without release are rejected",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: []string{
				"spec.imageOverrides.byoc: image must be fully specified when release is omitted",
				"spec.imageOverrides.observabilityPipelinesWorker: image must be fully specified when release is omitted",
			},
		},

		{
			name: "partial override without release is rejected",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
					BYOC: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Repository: ptr.To("registry.example/byoc"),
					},
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
			},
			want: []string{
				"spec.imageOverrides.byoc: tag or digest must be specified when release is omitted",
				"spec.imageOverrides.observabilityPipelinesWorker: image must be fully specified when release is omitted",
			},
		},

		{
			name: "tag and digest in both overrides",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To("v1"),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
				ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
					BYOC: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Tag:    ptr.To("v1"),
						Digest: ptr.To(""),
					},
					ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Tag:    ptr.To("v2"),
						Digest: ptr.To("sha512:custom"),
					},
				},
			},
			want: []string{
				"spec.imageOverrides.byoc: tag and digest are mutually exclusive",
				"spec.imageOverrides.observabilityPipelinesWorker: tag and digest are mutually exclusive",
			},
		},

		{
			name: "partial overrides with release",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To("v1"),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
				ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
					BYOC: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Repository: ptr.To("mirror/byoc"),
					},
					ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Digest: ptr.To("sha512:custom"),
					},
				},
			},
			want: nil,
		},

		{
			name: "fully specified images without release",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
				ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
					BYOC: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Repository: ptr.To("mirror/byoc"),
						Tag:        ptr.To("v1"),
					},
					ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Repository: ptr.To("mirror/worker"),
						Digest:     ptr.To("sha512:custom"),
					},
				},
			},
			want: nil,
		},

		{
			name: "unused release is still validated",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{
				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},
				ImageOverrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
					BYOC: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Repository: ptr.To("mirror/byoc"),
						Tag:        ptr.To("v1"),
					},
					ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCImageSpec{
						Repository: ptr.To("mirror/worker"),
						Tag:        ptr.To("v1"),
					},
				},
			},
			want: []string{
				"spec.release: exactly one of tag or digest must be specified",
			},
		},

		{
			name: "all component paths are validated and errors are aggregated",
			spec: datadoghqv1alpha1.DatadogBYOCClusterSpec{

				Release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
					Tag: ptr.To("v1"),
				},
				Provider: &datadoghqv1alpha1.DatadogBYOCClusterProviderSpec{
					AWS: &datadoghqv1alpha1.DatadogBYOCClusterAWSSpec{},
				},

				Global: datadoghqv1alpha1.DatadogBYOCClusterGlobalSpec{
					PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
						MinAvailable:   ptr.To(intstr.FromInt32(0)),
						MaxUnavailable: ptr.To(intstr.FromInt32(0)),
					},
				},

				Components: &datadoghqv1alpha1.DatadogBYOCClusterComponentsSpec{

					Indexer: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable:   ptr.To(intstr.FromInt32(0)),
								MaxUnavailable: ptr.To(intstr.FromInt32(0)),
							},
							Resources: &corev1.ResourceRequirements{},
						},
						Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{},
					},
					Searcher: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable:   ptr.To(intstr.FromInt32(0)),
								MaxUnavailable: ptr.To(intstr.FromInt32(0)),
							},
							Resources: &corev1.ResourceRequirements{},
						},
						Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{},
					},

					Pipelines: []datadoghqv1alpha1.DatadogBYOCClusterPipelineSpec{

						{
							Name: "logs",
							DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
								DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
									DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
										PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
											MinAvailable:   ptr.To(intstr.FromInt32(0)),
											MaxUnavailable: ptr.To(intstr.FromInt32(0)),
										},
										Resources: &corev1.ResourceRequirements{},
									},
									Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{},
								},
							},
						},

						{
							Name: "audit",
							DatadogBYOCClusterPipelineComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterPipelineComponentSpec{
								DatadogBYOCClusterStatefulComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
									DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
										PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
											MinAvailable:   ptr.To(intstr.FromInt32(0)),
											MaxUnavailable: ptr.To(intstr.FromInt32(0)),
										},
										Resources: &corev1.ResourceRequirements{},
									},
									Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{},
								},
							},
						},
					},

					Metastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable:   ptr.To(intstr.FromInt32(0)),
								MaxUnavailable: ptr.To(intstr.FromInt32(0)),
							},
						},
					},

					ReadOnlyMetastore: &datadoghqv1alpha1.DatadogBYOCClusterMetastoreComponentSpec{
						DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
							PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
								MinAvailable:   ptr.To(intstr.FromInt32(0)),
								MaxUnavailable: ptr.To(intstr.FromInt32(0)),
							},
						},
					},

					ControlPlane: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
							MinAvailable:   ptr.To(intstr.FromInt32(0)),
							MaxUnavailable: ptr.To(intstr.FromInt32(0)),
						},
					},
					Compactor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
							MinAvailable:   ptr.To(intstr.FromInt32(0)),
							MaxUnavailable: ptr.To(intstr.FromInt32(0)),
						},
					},
					Janitor: &datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
						PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
							MinAvailable:   ptr.To(intstr.FromInt32(0)),
							MaxUnavailable: ptr.To(intstr.FromInt32(0)),
						},
					},
				},
			},
			want: []string{
				"spec.global.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.indexer.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.indexer.resources.limits.memory: resources.limits.memory must be specified when resources is set",
				"spec.components.indexer.storage: exactly one storage type must be specified",
				"spec.components.searcher.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.searcher.resources.limits.memory: resources.limits.memory must be specified when resources is set",
				"spec.components.searcher.storage: exactly one storage type must be specified",
				"spec.components.pipelines[0].podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.pipelines[0].resources.limits.memory: resources.limits.memory must be specified when resources is set",
				"spec.components.pipelines[0].storage: exactly one storage type must be specified",
				"spec.components.pipelines[1].podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.pipelines[1].resources.limits.memory: resources.limits.memory must be specified when resources is set",
				"spec.components.pipelines[1].storage: exactly one storage type must be specified",
				"spec.components.metastore.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.readOnlyMetastore.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.controlPlane.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.compactor.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
				"spec.components.janitor.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, validationMessages(ValidateClusterSpec(&tt.spec))); diff != "" {
				t.Errorf("validation mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateImageOverride(t *testing.T) {
	tests := []struct {
		name            string
		image           *datadoghqv1alpha1.DatadogBYOCImageSpec
		requireComplete bool
		want            []string
	}{
		{
			name:            "missing image without release",
			requireComplete: true,
			want:            []string{"spec.imageOverrides.byoc: image must be fully specified when release is omitted"},
		},
		{
			name:            "empty image without release",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{},
			requireComplete: true,
			want: []string{
				"spec.imageOverrides.byoc.repository: repository must be non-empty when release is omitted",
				"spec.imageOverrides.byoc: tag or digest must be specified when release is omitted",
			},
		},
		{
			name: "secrets alone cannot resolve an image",
			image: &datadoghqv1alpha1.DatadogBYOCImageSpec{
				ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
			},
			requireComplete: true,
			want: []string{
				"spec.imageOverrides.byoc.repository: repository must be non-empty when release is omitted",
				"spec.imageOverrides.byoc: tag or digest must be specified when release is omitted",
			},
		},
		{
			name:            "missing repository",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{Tag: ptr.To("v1")},
			requireComplete: true,
			want:            []string{"spec.imageOverrides.byoc.repository: repository must be non-empty when release is omitted"},
		},
		{
			name:            "empty repository",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: ptr.To(""), Tag: ptr.To("v1")},
			requireComplete: true,
			want:            []string{"spec.imageOverrides.byoc.repository: repository must be non-empty when release is omitted"},
		},
		{
			name:            "missing version",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: ptr.To("registry.example/byoc")},
			requireComplete: true,
			want:            []string{"spec.imageOverrides.byoc: tag or digest must be specified when release is omitted"},
		},
		{
			name:            "empty tag",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: ptr.To("registry.example/byoc"), Tag: ptr.To("")},
			requireComplete: true,
			want:            []string{"spec.imageOverrides.byoc.tag: tag must be non-empty when release is omitted"},
		},
		{
			name:            "empty digest",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: ptr.To("registry.example/byoc"), Digest: ptr.To("")},
			requireComplete: true,
			want:            []string{"spec.imageOverrides.byoc.digest: digest must be non-empty when release is omitted"},
		},
		{
			name: "tag and digest together",
			image: &datadoghqv1alpha1.DatadogBYOCImageSpec{
				Repository: ptr.To("registry.example/byoc"), Tag: ptr.To("v1"), Digest: ptr.To("sha256:custom"),
			},
			requireComplete: true,
			want:            []string{"spec.imageOverrides.byoc: tag and digest are mutually exclusive"},
		},
		{
			name:            "complete tag reference",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: ptr.To("registry.example/byoc"), Tag: ptr.To("v1")},
			requireComplete: true,
		},
		{
			name:            "complete digest reference",
			image:           &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: ptr.To("registry.example/byoc"), Digest: ptr.To("sha512:custom")},
			requireComplete: true,
		},
		{
			name: "release supplies both images",
		},
		{
			name:  "release supplies repository for a tag override",
			image: &datadoghqv1alpha1.DatadogBYOCImageSpec{Tag: ptr.To("v1")},
		},
		{
			name:  "release supplies version for a repository override",
			image: &datadoghqv1alpha1.DatadogBYOCImageSpec{Repository: ptr.To("mirror/byoc")},
		},
		{
			name:  "empty override with release",
			image: &datadoghqv1alpha1.DatadogBYOCImageSpec{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateImageOverride(tt.image, tt.requireComplete, field.NewPath("spec", "imageOverrides", "byoc"))
			if diff := cmp.Diff(tt.want, validationMessages(got)); diff != "" {
				t.Errorf("validation mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestValidateStatefulComponent(t *testing.T) {
	tests := []struct {
		name      string
		component *datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec
		want      []string
	}{
		{
			name:      "omitted component",
			component: nil,
			want:      nil,
		},

		{
			name:      "omitted settings",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{},
			want:      nil,
		},

		{
			name: "empty resources",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					Resources: &corev1.ResourceRequirements{},
				},
			},
			want: []string{
				"spec.resources.limits.memory: resources.limits.memory must be specified when resources is set",
			},
		},

		{
			name: "requests alone",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					Resources: &corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			want: []string{
				"spec.resources.limits.memory: resources.limits.memory must be specified when resources is set",
			},
		},

		{
			name: "limits without memory",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
					},
				},
			},
			want: []string{
				"spec.resources.limits.memory: resources.limits.memory must be specified when resources is set",
			},
		},

		{
			name: "memory limit",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
				},
			},
			want: nil,
		},

		{
			name: "zero memory is present",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceMemory: resource.MustParse("0"),
						},
					},
				},
			},
			want: nil,
		},

		{
			name: "empty storage",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{},
			},
			want: []string{
				"spec.storage: exactly one storage type must be specified",
			},
		},

		{
			name: "emptyDir storage",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			want: nil,
		},

		{
			name: "PVC storage",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
				},
			},
			want: nil,
		},

		{
			name: "both storage types",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				Storage: &datadoghqv1alpha1.DatadogBYOCClusterStorageSpec{
					EmptyDir:            &corev1.EmptyDirVolumeSource{},
					VolumeClaimTemplate: &datadoghqv1alpha1.DatadogBYOCClusterEmbeddedPersistentVolumeClaim{},
				},
			},
			want: []string{
				"spec.storage: exactly one storage type must be specified",
			},
		},

		{
			name: "empty PDB disables budget",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{},
				},
			},
			want: nil,
		},

		{
			name: "PDB minimum only",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
						MinAvailable: ptr.To(intstr.FromInt32(0)),
					},
				},
			},
			want: nil,
		},

		{
			name: "PDB maximum only",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
						MaxUnavailable: ptr.To(intstr.FromString("50%")),
					},
				},
			},
			want: nil,
		},

		{
			name: "conflicting PDB",
			component: &datadoghqv1alpha1.DatadogBYOCClusterStatefulComponentSpec{
				DatadogBYOCClusterComponentSpec: datadoghqv1alpha1.DatadogBYOCClusterComponentSpec{
					PodDisruptionBudget: &datadoghqv1alpha1.DatadogBYOCClusterPodDisruptionBudgetSpec{
						MinAvailable:   ptr.To(intstr.FromInt32(0)),
						MaxUnavailable: ptr.To(intstr.FromInt32(0)),
					},
				},
			},
			want: []string{
				"spec.podDisruptionBudget: minAvailable and maxUnavailable are mutually exclusive",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, validationMessages(ValidateStatefulComponent(tt.component, field.NewPath("spec")))); diff != "" {
				t.Errorf("validation mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func validationMessages(errs field.ErrorList) []string {
	var messages []string
	for _, err := range errs {
		messages = append(messages, err.Field+": "+err.Detail)
	}
	return messages
}
