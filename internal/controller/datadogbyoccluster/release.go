// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogbyoccluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"k8s.io/utils/ptr"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	// DefaultReleaseRepository is the OCI repository containing BYOC release artifacts.
	DefaultReleaseRepository = "public.ecr.aws/datadog/byoc-release"
)

// ReleaseResolver resolves a DatadogBYOCCluster release reference.
type ReleaseResolver interface {
	Resolve(context.Context, *datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec) (*ResolvedRelease, error)
}

// ResolvedRelease contains the immutable artifact digest and its validated payload.
type ResolvedRelease struct {
	Digest  string
	Release BYOCRelease
}

// BYOCRelease is the JSON payload published in a BYOC release artifact.
type BYOCRelease struct {
	Images BYOCReleaseImages `json:"images"`
}

// BYOCReleaseImages lists the compatible images in a BYOC release.
type BYOCReleaseImages struct {
	Pomsky                       BYOCReleaseImage `json:"pomsky"`
	ObservabilityPipelinesWorker BYOCReleaseImage `json:"observabilityPipelinesWorker"`
}

// BYOCReleaseImage identifies an image by repository and tag or digest.
type BYOCReleaseImage struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

// ImageReference returns the immutable image reference when a digest is available.
func (i BYOCReleaseImage) ImageReference() string {
	if i.Digest != "" {
		return i.Repository + "@" + i.Digest
	}
	return i.Repository + ":" + i.Tag
}

type targetFactory func(context.Context, string) (oras.ReadOnlyTarget, error)

// OCIReleaseResolver resolves BYOC release artifacts from an OCI repository.
type OCIReleaseResolver struct {
	repository    string
	targetFactory targetFactory
}

// NewOCIReleaseResolver returns a resolver backed by the Datadog public OCI repository.
func NewOCIReleaseResolver() *OCIReleaseResolver {
	return newOCIReleaseResolver(DefaultReleaseRepository, func(_ context.Context, repository string) (oras.ReadOnlyTarget, error) {
		return remote.NewRepository(repository)
	})
}

func newOCIReleaseResolver(repository string, factory targetFactory) *OCIReleaseResolver {
	return &OCIReleaseResolver{
		repository:    repository,
		targetFactory: factory,
	}
}

// Resolve fetches and validates the release artifact selected by tag or digest.
func (r *OCIReleaseResolver) Resolve(ctx context.Context, spec *datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec) (*ResolvedRelease, error) {
	if spec == nil {
		return nil, errors.New("release must be specified")
	}

	tag := ptr.Deref(spec.Tag, "")
	requestedDigest := ptr.Deref(spec.Digest, "")
	if tag == "" && requestedDigest == "" {
		return nil, errors.New("release tag or digest must be specified")
	}
	if requestedDigest != "" {
		if err := validateDigest(requestedDigest); err != nil {
			return nil, fmt.Errorf("invalid release digest: %w", err)
		}
	}
	if r.targetFactory == nil {
		return nil, errors.New("OCI target factory is not configured")
	}

	target, err := r.targetFactory(ctx, r.repository)
	if err != nil {
		return nil, fmt.Errorf("create OCI repository client: %w", err)
	}

	reference := requestedDigest
	if tag != "" {
		reference = tag
	}
	descriptor, err := target.Resolve(ctx, reference)
	if err != nil {
		return nil, fmt.Errorf("resolve release %q: %w", reference, err)
	}
	if requestedDigest != "" && descriptor.Digest.String() != requestedDigest {
		return nil, fmt.Errorf("release tag %q resolved to digest %q, expected %q", tag, descriptor.Digest, requestedDigest)
	}
	manifestBytes, err := content.FetchAll(ctx, target, descriptor)
	if err != nil {
		return nil, fmt.Errorf("fetch release manifest %q: %w", descriptor.Digest, err)
	}
	var manifest ocispec.Manifest
	if decodeErr := json.Unmarshal(manifestBytes, &manifest); decodeErr != nil {
		return nil, fmt.Errorf("decode release manifest: %w", decodeErr)
	}
	if len(manifest.Layers) != 1 {
		return nil, fmt.Errorf("release manifest must contain exactly one layer, found %d", len(manifest.Layers))
	}

	layer := manifest.Layers[0]
	layerBytes, err := content.FetchAll(ctx, target, layer)
	if err != nil {
		return nil, fmt.Errorf("fetch release layer %q: %w", layer.Digest, err)
	}

	var release BYOCRelease
	if err := json.Unmarshal(layerBytes, &release); err != nil {
		return nil, fmt.Errorf("decode release payload: %w", err)
	}
	if err := release.validate(); err != nil {
		return nil, fmt.Errorf("validate release payload: %w", err)
	}

	return &ResolvedRelease{
		Digest:  descriptor.Digest.String(),
		Release: release,
	}, nil
}

func (r BYOCRelease) validate() error {
	if err := r.Images.Pomsky.validate("images.pomsky"); err != nil {
		return err
	}
	if err := r.Images.ObservabilityPipelinesWorker.validate("images.observabilityPipelinesWorker"); err != nil {
		return err
	}
	return nil
}

func (i BYOCReleaseImage) validate(field string) error {
	if strings.TrimSpace(i.Repository) == "" {
		return fmt.Errorf("%s.repository must be specified", field)
	}
	if i.Tag == "" && i.Digest == "" {
		return fmt.Errorf("%s.tag or %s.digest must be specified", field, field)
	}
	if i.Digest != "" {
		if err := validateDigest(i.Digest); err != nil {
			return fmt.Errorf("%s.digest is invalid: %w", field, err)
		}
	}
	return nil
}

func validateDigest(value string) error {
	parsed, err := digest.Parse(value)
	if err != nil {
		return err
	}
	if parsed.Algorithm() != digest.SHA256 {
		return fmt.Errorf("algorithm must be %s", digest.SHA256)
	}
	return nil
}
