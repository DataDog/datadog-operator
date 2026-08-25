// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package release

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"k8s.io/utils/ptr"
	"oras.land/oras-go/v2"

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

func TestOCIReleaseResolver_Resolve(t *testing.T) {
	tests := []struct {
		name    string
		release datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec
		payload string
		want    *ResolvedRelease
		wantErr string
	}{
		{
			name:    "tag",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			payload: validReleasePayload,
			want: &ResolvedRelease{
				Release: BYOCRelease{
					Images: BYOCReleaseImages{
						Pomsky: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/cloudprem",
							Tag:        "v0.1.32",
							Digest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
						ObservabilityPipelinesWorker: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/observability-pipelines-worker",
							Tag:        "2.10.0",
							Digest:     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						},
					},
				},
			},
		},
		{
			name:    "digest",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Digest: ptr.To(validArtifactDigest)},
			payload: validReleasePayload,
			want: &ResolvedRelease{
				Release: BYOCRelease{
					Images: BYOCReleaseImages{
						Pomsky: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/cloudprem",
							Tag:        "v0.1.32",
							Digest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
						ObservabilityPipelinesWorker: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/observability-pipelines-worker",
							Tag:        "2.10.0",
							Digest:     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						},
					},
				},
			},
		},
		{
			name: "matching tag and digest",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
				Tag:    ptr.To(releaseTag),
				Digest: ptr.To(validArtifactDigest),
			},
			payload: validReleasePayload,
			want: &ResolvedRelease{
				Release: BYOCRelease{
					Images: BYOCReleaseImages{
						Pomsky: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/cloudprem",
							Tag:        "v0.1.32",
							Digest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
						ObservabilityPipelinesWorker: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/observability-pipelines-worker",
							Tag:        "2.10.0",
							Digest:     "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						},
					},
				},
			},
		},
		{
			name:    "Pomsky image with tag",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			payload: tagOnlyReleasePayload,
			want: &ResolvedRelease{
				Release: BYOCRelease{
					Images: BYOCReleaseImages{
						Pomsky: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/cloudprem",
							Tag:        "v0.1.32",
						},
						ObservabilityPipelinesWorker: BYOCReleaseImage{
							Repository: "public.ecr.aws/datadog/observability-pipelines-worker",
							Tag:        "2.10.0",
						},
					},
				},
			},
		},
		{
			name: "tag and digest mismatch",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{
				Tag:    ptr.To(releaseTag),
				Digest: ptr.To("sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"),
			},
			payload: validReleasePayload,
			wantErr: "resolved to digest",
		},
		{
			name:    "missing reference",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{},
			payload: validReleasePayload,
			wantErr: "tag or digest",
		},
		{
			name:    "invalid digest",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Digest: ptr.To("sha256:invalid")},
			payload: validReleasePayload,
			wantErr: "invalid release digest",
		},
		{
			name:    "invalid JSON payload",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
			payload: "{",
			wantErr: "decode release payload",
		},
		{
			name:    "invalid release payload",
			release: datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec{Tag: ptr.To(releaseTag)},
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
			resolver := newOCIReleaseResolver(defaultReleaseRepository, func(context.Context, string) (oras.ReadOnlyTarget, error) {
				return artifact, nil
			})

			got, err := resolver.Resolve(context.Background(), &tt.release)
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
			if diff := cmp.Diff(tt.want, got, cmpopts.IgnoreFields(ResolvedRelease{}, "Digest")); diff != "" {
				t.Errorf("Resolve() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestBYOCReleaseImage_ImageReference(t *testing.T) {
	tests := []struct {
		name  string
		image BYOCReleaseImage
		want  string
	}{
		{
			name: "digest",
			image: BYOCReleaseImage{
				Repository: "public.ecr.aws/datadog/cloudprem",
				Tag:        "v0.1.32",
				Digest:     "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			},
			want: "public.ecr.aws/datadog/cloudprem@sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		},
		{
			name: "tag",
			image: BYOCReleaseImage{
				Repository: "public.ecr.aws/datadog/cloudprem",
				Tag:        "v0.1.32",
			},
			want: "public.ecr.aws/datadog/cloudprem:v0.1.32",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.image.ImageReference(); got != tt.want {
				t.Errorf("ImageReference() = %q, want %q", got, tt.want)
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

func (a *fakeArtifact) Resolve(_ context.Context, _ string) (ocispec.Descriptor, error) {
	return a.manifestDescriptor, nil
}

func (a *fakeArtifact) Fetch(_ context.Context, descriptor ocispec.Descriptor) (io.ReadCloser, error) {
	switch descriptor.Digest {
	case a.manifestDescriptor.Digest:
		return io.NopCloser(bytes.NewReader(a.manifest)), nil
	case a.layerDescriptor.Digest:
		return io.NopCloser(bytes.NewReader(a.payload)), nil
	default:
		return nil, errors.New("content not found")
	}
}

func (a *fakeArtifact) Exists(_ context.Context, descriptor ocispec.Descriptor) (bool, error) {
	return descriptor.Digest == a.manifestDescriptor.Digest || descriptor.Digest == a.layerDescriptor.Digest, nil
}
