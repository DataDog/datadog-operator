// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package image

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry/remote"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	defaultReleaseRepository = "public.ecr.aws/datadog/byoc-release"
)

// ResolvedImages contains the effective workload images after applying overrides.
// These values do not imply that a workload is running the image.
type ResolvedImages struct {
	Pomsky                       ResolvedImage
	ObservabilityPipelinesWorker ResolvedImage
}

// ResolvedImage contains a workload image and its Pod pull secrets.
// Pull secrets are local configuration, not part of the release artifact.
type ResolvedImage struct {
	Repository       string
	Tag              string
	Digest           string
	ImagePullSecrets []corev1.LocalObjectReference
}

// ImageReference returns the immutable image reference when a digest is available.
func (i ResolvedImage) ImageReference() string {
	if i.Digest != "" {
		return i.Repository + "@" + i.Digest
	}
	return i.Repository + ":" + i.Tag
}

// ImageResolver resolves the effective images for a DatadogBYOCCluster.
type ImageResolver interface {
	Resolve(context.Context, *datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec, *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides) (*ResolvedImages, error)
}

// byocRelease is the JSON payload published in a BYOC release artifact.
type byocRelease struct {
	Images byocReleaseImages `json:"images"`
}

// byocReleaseImages lists the compatible images in a BYOC release.
type byocReleaseImages struct {
	Pomsky                       releaseImage `json:"pomsky"`
	ObservabilityPipelinesWorker releaseImage `json:"observabilityPipelinesWorker"`
}

// releaseImage identifies an image by repository and tag or digest.
type releaseImage struct {
	Repository string `json:"repository"`
	Tag        string `json:"tag,omitempty"`
	Digest     string `json:"digest,omitempty"`
}

type targetFactory func(context.Context, string) (oras.ReadOnlyTarget, error)

type ociImageResolver struct {
	repository    string
	targetFactory targetFactory
}

// NewOCIImageResolver returns an image resolver backed by the Datadog public OCI repository.
func NewOCIImageResolver() ImageResolver {
	return newOCIImageResolver(defaultReleaseRepository, func(_ context.Context, repository string) (oras.ReadOnlyTarget, error) {
		return remote.NewRepository(repository)
	})
}

func newOCIImageResolver(repository string, factory targetFactory) *ociImageResolver {
	return &ociImageResolver{
		repository:    repository,
		targetFactory: factory,
	}
}

// Resolve returns the effective workload images, fetching a release artifact only
// when the local overrides do not fully specify both images.
func (r *ociImageResolver) Resolve(ctx context.Context, spec *datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec, overrides *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides) (*ResolvedImages, error) {
	var release *byocRelease
	if requiresResolution(overrides) {
		var err error
		release, err = r.resolveRelease(ctx, spec)
		if err != nil {
			return nil, err
		}
	}
	return overrideImages(release, overrides)
}

// resolveRelease fetches and validates the release artifact selected by tag or digest.
func (r *ociImageResolver) resolveRelease(ctx context.Context, spec *datadoghqv1alpha1.DatadogBYOCClusterReleaseSpec) (*byocRelease, error) {
	if spec == nil {
		return nil, errors.New("release must be specified")
	}

	tag := ptr.Deref(spec.Tag, "")
	requestedDigest := ptr.Deref(spec.Digest, "")
	if tag == "" && requestedDigest == "" {
		return nil, errors.New("release tag or digest must be specified")
	}
	if tag != "" && requestedDigest != "" {
		return nil, errors.New("release tag and digest are mutually exclusive")
	}
	if requestedDigest != "" {
		if err := validateDigest(requestedDigest); err != nil {
			return nil, fmt.Errorf("invalid release digest: %w", err)
		}
	}
	if r.targetFactory == nil {
		return nil, errors.New("OCI target factory is not configured")
	}

	repository := ptr.Deref(spec.Repository, r.repository)
	target, err := r.targetFactory(ctx, repository)
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

	var release byocRelease
	if err := json.Unmarshal(layerBytes, &release); err != nil {
		return nil, fmt.Errorf("decode release payload: %w", err)
	}
	if err := validateRelease(release); err != nil {
		return nil, fmt.Errorf("validate release payload: %w", err)
	}

	return &release, nil
}

func validateRelease(release byocRelease) error {
	if err := validateReleaseImage(release.Images.Pomsky, "images.pomsky"); err != nil {
		return err
	}
	if err := validateReleaseImage(release.Images.ObservabilityPipelinesWorker, "images.observabilityPipelinesWorker"); err != nil {
		return err
	}
	return nil
}

func validateReleaseImage(image releaseImage, field string) error {
	if strings.TrimSpace(image.Repository) == "" {
		return fmt.Errorf("%s.repository must be specified", field)
	}
	if image.Tag == "" && image.Digest == "" {
		return fmt.Errorf("%s.tag or %s.digest must be specified", field, field)
	}
	if image.Digest != "" {
		if err := validateDigest(image.Digest); err != nil {
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

// requiresResolution reports whether either image needs values from the release artifact.
// Both logical images must be fully specified, regardless of which workloads are enabled.
func requiresResolution(overrides *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides) bool {
	return overrides == nil || !completeImageOverride(overrides.BYOC) || !completeImageOverride(overrides.ObservabilityPipelinesWorker)
}

func completeImageOverride(override *datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec) bool {
	return override != nil && ptr.Deref(override.Repository, "") != "" &&
		((ptr.Deref(override.Tag, "") != "") != (ptr.Deref(override.Digest, "") != ""))
}

// overrideImages applies local overrides to release images without mutating either input.
// A nil release is allowed only when both images are fully specified by overrides.
func overrideImages(release *byocRelease, overrides *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides) (*ResolvedImages, error) {
	var byoc, worker *datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec
	if overrides != nil {
		byoc, worker = overrides.BYOC, overrides.ObservabilityPipelinesWorker
	}
	if release == nil && requiresResolution(overrides) {
		return nil, errors.New("resolved release is required unless both image overrides specify repository and tag or digest")
	}

	var base byocReleaseImages
	if release != nil {
		base = release.Images
	}
	images := &ResolvedImages{
		Pomsky:                       overrideImage(base.Pomsky, byoc),
		ObservabilityPipelinesWorker: overrideImage(base.ObservabilityPipelinesWorker, worker),
	}
	return images, nil
}

func overrideImage(base releaseImage, override *datadoghqv1alpha1.DatadogBYOCClusterImageOverrideSpec) ResolvedImage {
	image := ResolvedImage{Repository: base.Repository, Tag: base.Tag, Digest: base.Digest}
	if override == nil {
		return image
	}
	if override.Repository != nil {
		image.Repository = *override.Repository
	}
	// Version overrides replace, rather than merge with, the release version.
	if override.Tag != nil {
		image.Tag, image.Digest = *override.Tag, ""
	} else if override.Digest != nil {
		image.Tag, image.Digest = "", *override.Digest
	}
	image.ImagePullSecrets = slices.Clone(override.ImagePullSecrets)
	return image
}
