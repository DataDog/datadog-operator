// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package providers overrides pod template content per node provider (e.g. GKE COS).
package providers

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// Provider overrides pod template content by name. ok is false when the
// provider has no opinion (use the caller's value). If ok is true, nil means
// skip it, non-nil means use this instead.
type Provider interface {
	// Volume overrides the pod-level volume named name.
	Volume(name string) (*corev1.Volume, bool)
	// VolumeMount overrides the volume mount named name.
	VolumeMount(name string) (*corev1.VolumeMount, bool)
}

// defaultProvider has no overrides. Concrete providers embed it and
// implement only the methods where they differ.
type defaultProvider struct{}

func (defaultProvider) Volume(name string) (*corev1.Volume, bool) {
	return nil, false
}

func (defaultProvider) VolumeMount(name string) (*corev1.VolumeMount, bool) {
	return nil, false
}

// Default is the no-op Provider.
func Default() Provider {
	return defaultProvider{}
}

// Get returns the Provider for a node provider name (from
// kubernetes.NodeProviderAnnotationKey). Unknown or empty returns Default().
func Get(name string) Provider {
	switch name {
	case kubernetes.GKECosProvider:
		return gkeCosProvider{}
	default:
		return defaultProvider{}
	}
}
