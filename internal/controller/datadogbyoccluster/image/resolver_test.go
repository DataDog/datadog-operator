// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package image

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name      string
		overrides *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides
		want      *ResolvedImages
	}{
		{
			name: "default images",
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/cloudprem",
					Tag:             "v0.1.36",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/observability-pipelines-worker",
					Tag:             "2.25.1",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		},
		{
			name: "repository overrides keep the default tags",
			overrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
				BYOC: &datadoghqv1alpha1.DatadogBYOCImageSpec{
					Repository:       ptr.To("private.example.com/cloudprem"),
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
				},
				ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCImageSpec{
					Repository: ptr.To("private.example.com/worker"),
				},
			},
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:       "private.example.com/cloudprem",
					Tag:              "v0.1.36",
					ImagePullPolicy:  corev1.PullIfNotPresent,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "private.example.com/worker",
					Tag:             "2.25.1",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		},
		{
			name: "version and pull policy overrides",
			overrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
				BYOC: &datadoghqv1alpha1.DatadogBYOCImageSpec{
					Tag:        ptr.To("hotfix"),
					PullPolicy: ptr.To(corev1.PullAlways),
				},
				ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCImageSpec{
					Digest:     ptr.To("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
					PullPolicy: ptr.To(corev1.PullNever),
				},
			},
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/cloudprem",
					Tag:             "hotfix",
					ImagePullPolicy: corev1.PullAlways,
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/observability-pipelines-worker",
					Digest:          "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					ImagePullPolicy: corev1.PullNever,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if diff := cmp.Diff(tt.want, Resolve(tt.overrides)); diff != "" {
				t.Errorf("Resolve() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestResolvedImage_GetImageReference(t *testing.T) {
	tests := []struct {
		name  string
		image ResolvedImage
		want  string
	}{
		{
			name:  "tag",
			image: ResolvedImage{Repository: "registry.example.com/image", Tag: "latest"},
			want:  "registry.example.com/image:latest",
		},
		{
			name:  "digest",
			image: ResolvedImage{Repository: "registry.example.com/image", Digest: "sha256:abcdef"},
			want:  "registry.example.com/image@sha256:abcdef",
		},
		{
			name:  "digest takes precedence over tag",
			image: ResolvedImage{Repository: "registry.example.com/image", Tag: "latest", Digest: "sha256:abcdef"},
			want:  "registry.example.com/image@sha256:abcdef",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.image.GetImageReference(); got != tt.want {
				t.Errorf("GetImageReference() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestResolvedImage_GetImagePullPolicy(t *testing.T) {
	tests := []struct {
		name  string
		image ResolvedImage
		want  corev1.PullPolicy
	}{
		{
			name: "default",
			want: corev1.PullIfNotPresent,
		},
		{
			name:  "configured",
			image: ResolvedImage{ImagePullPolicy: corev1.PullAlways},
			want:  corev1.PullAlways,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.image.GetImagePullPolicy(); got != tt.want {
				t.Errorf("GetImagePullPolicy() = %q, want %q", got, tt.want)
			}
		})
	}
}
