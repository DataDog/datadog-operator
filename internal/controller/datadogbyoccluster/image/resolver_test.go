// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package image

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	releaseTag          = "0.1.32"
	validArtifactDigest = "sha256:8acf21b04282a82303d2c3177085dd4163750129bb703dc3b42a316c0788236a"
	validReleasePayload = `{
		"images": {
			"pomsky": {
				"repository": "public.ecr.aws/datadog/cloudprem",
				"tag": "v0.1.32",
				"digest": "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			},
			"observabilityPipelinesWorker": {
				"repository": "public.ecr.aws/datadog/observability-pipelines-worker",
				"tag": "2.10.0",
				"digest": "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
			}
		}
	}`
	tagOnlyReleasePayload = `{
		"images": {
			"pomsky": {
				"repository": "public.ecr.aws/datadog/cloudprem",
				"tag": "v0.1.32"
			},
			"observabilityPipelinesWorker": {
				"repository": "public.ecr.aws/datadog/observability-pipelines-worker",
				"tag": "2.10.0"
			}
		}
	}`
)

func TestOCIImageResolver_Resolve(t *testing.T) {
	tests := []struct {
		name      string
		release   *datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec
		overrides *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides
		payload   string
		want      *ResolvedImages
		wantErr   string
	}{
		{
			name:    "tag",
			release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			payload: validReleasePayload,
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/cloudprem",
					Tag:             "v0.1.32",
					Digest:          "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/observability-pipelines-worker",
					Tag:             "2.10.0",
					Digest:          "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		},
		{
			name:    "digest",
			release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Digest: ptr.To(validArtifactDigest)},
			payload: validReleasePayload,
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/cloudprem",
					Tag:             "v0.1.32",
					Digest:          "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/observability-pipelines-worker",
					Tag:             "2.10.0",
					Digest:          "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		},
		{
			name:    "images with tags",
			release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			payload: tagOnlyReleasePayload,
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/cloudprem",
					Tag:             "v0.1.32",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/observability-pipelines-worker",
					Tag:             "2.10.0",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		},
		{
			name:    "release with image overrides",
			release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			overrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
				BYOC: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec{
					Repository:       ptr.To("private.example.com/pomsky"),
					Tag:              ptr.To("hotfix"),
					PullPolicy:       ptr.To(corev1.PullAlways),
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
				},
				ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec{
					Digest:     ptr.To("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
					PullPolicy: ptr.To(corev1.PullNever),
				},
			},
			payload: validReleasePayload,
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:       "private.example.com/pomsky",
					Tag:              "hotfix",
					ImagePullPolicy:  corev1.PullAlways,
					ImagePullSecrets: []corev1.LocalObjectReference{{Name: "registry-credentials"}},
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "public.ecr.aws/datadog/observability-pipelines-worker",
					Digest:          "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					ImagePullPolicy: corev1.PullNever,
				},
			},
		},
		{
			name: "complete image overrides without release",
			overrides: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrides{
				BYOC: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec{
					Repository: ptr.To("private.example.com/pomsky"),
					Tag:        ptr.To("hotfix"),
				},
				ObservabilityPipelinesWorker: &datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec{
					Repository: ptr.To("private.example.com/worker"),
					Digest:     ptr.To("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
				},
			},
			want: &ResolvedImages{
				Pomsky: ResolvedImage{
					Repository:      "private.example.com/pomsky",
					Tag:             "hotfix",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
				ObservabilityPipelinesWorker: ResolvedImage{
					Repository:      "private.example.com/worker",
					Digest:          "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc",
					ImagePullPolicy: corev1.PullIfNotPresent,
				},
			},
		},
		{
			name:    "missing release",
			payload: validReleasePayload,
			wantErr: "release must be specified",
		},
		{
			name:    "invalid JSON payload",
			release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			payload: "{",
			wantErr: "decode release payload",
		},
		{
			name:    "invalid release payload",
			release: &datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			payload: `{
				"images": {
					"pomsky": {
						"repository": "public.ecr.aws/datadog/cloudprem",
						"tag": "v0.1.32"
					}
				}
			}`,
			wantErr: "images.observabilityPipelinesWorker.repository",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			artifact := newFakeArtifact(t, tt.payload)
			resolver := newOCIImageResolver(defaultReleaseRepository, func(_ context.Context, _ string) (releaseTarget, error) {
				return artifact, nil
			})

			got, err := resolver.Resolve(context.Background(), tt.release, tt.overrides)
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("Resolve() error = nil, want an error containing %q", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("Resolve() error = %q, want an error containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Resolve() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
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

type fakeArtifact struct {
	manifestDescriptor ocispec.Descriptor
	manifest           []byte
	layerDescriptor    ocispec.Descriptor
	payload            []byte
}

func newFakeArtifact(t *testing.T, payload string) *fakeArtifact {
	t.Helper()

	payloadBytes := []byte(payload)
	layerDescriptor := ocispec.Descriptor{
		MediaType: "application/json",
		Digest:    digest.FromBytes(payloadBytes),
		Size:      int64(len(payloadBytes)),
	}
	manifest, err := json.Marshal(ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Layers:    []ocispec.Descriptor{layerDescriptor},
	})
	if err != nil {
		t.Fatalf("failed to marshal manifest: %v", err)
	}
	return &fakeArtifact{
		manifestDescriptor: ocispec.Descriptor{
			MediaType: ocispec.MediaTypeImageManifest,
			Digest:    digest.FromBytes(manifest),
			Size:      int64(len(manifest)),
		},
		manifest:        manifest,
		layerDescriptor: layerDescriptor,
		payload:         payloadBytes,
	}
}

func (a *fakeArtifact) Resolve(_ context.Context, _ string) (ocispec.Descriptor, []byte, error) {
	return a.manifestDescriptor, a.manifest, nil
}

func (a *fakeArtifact) Fetch(_ context.Context, _ ocispec.Descriptor) ([]byte, error) {
	return a.payload, nil
}
