// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package image

import (
	"slices"

	corev1 "k8s.io/api/core/v1"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

const (
	defaultBYOCRepository                         = "public.ecr.aws/datadog/cloudprem"
	defaultBYOCTag                                = "v0.1.36"
	defaultObservabilityPipelinesWorkerRepository = "public.ecr.aws/datadog/observability-pipelines-worker"
	defaultObservabilityPipelinesWorkerTag        = "2.25.1"
)

// ResolvedImages contains the effective workload images after applying overrides.
// These values do not imply that a workload is running the image.
type ResolvedImages struct {
	Pomsky                       ResolvedImage
	ObservabilityPipelinesWorker ResolvedImage
}

// ResolvedImage contains a workload image and its Pod pull secrets.
type ResolvedImage struct {
	Repository       string
	Tag              string
	Digest           string
	ImagePullPolicy  corev1.PullPolicy
	ImagePullSecrets []corev1.LocalObjectReference
}

// GetImageReference returns the immutable image reference when a digest is available.
func (i ResolvedImage) GetImageReference() string {
	if i.Digest != "" {
		return i.Repository + "@" + i.Digest
	}
	return i.Repository + ":" + i.Tag
}

// GetImagePullPolicy returns the configured image pull policy or the BYOC default.
func (i ResolvedImage) GetImagePullPolicy() corev1.PullPolicy {
	if i.ImagePullPolicy == "" {
		return corev1.PullIfNotPresent
	}
	return i.ImagePullPolicy
}

// Resolve applies the image overrides to the default images without mutating the overrides.
func Resolve(overrides *datadoghqv1alpha1.DatadogBYOCClusterImageOverrides) *ResolvedImages {
	var byoc, worker *datadoghqv1alpha1.DatadogBYOCImageSpec
	if overrides != nil {
		byoc, worker = overrides.BYOC, overrides.ObservabilityPipelinesWorker
	}
	return &ResolvedImages{
		Pomsky:                       overrideImage(defaultBYOCRepository, defaultBYOCTag, byoc),
		ObservabilityPipelinesWorker: overrideImage(defaultObservabilityPipelinesWorkerRepository, defaultObservabilityPipelinesWorkerTag, worker),
	}
}

func overrideImage(repository, tag string, override *datadoghqv1alpha1.DatadogBYOCImageSpec) ResolvedImage {
	image := ResolvedImage{Repository: repository, Tag: tag, ImagePullPolicy: corev1.PullIfNotPresent}
	if override == nil {
		return image
	}
	if override.Repository != nil {
		image.Repository = *override.Repository
	}
	// Version overrides replace, rather than merge with, the default version.
	if override.Tag != nil {
		image.Tag = *override.Tag
	} else if override.Digest != nil {
		image.Tag, image.Digest = "", *override.Digest
	}
	if override.PullPolicy != nil {
		image.ImagePullPolicy = *override.PullPolicy
	}
	image.ImagePullSecrets = slices.Clone(override.ImagePullSecrets)
	return image
}
