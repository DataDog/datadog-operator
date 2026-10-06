// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ProfileSharedConfigOverlayFunc validates and applies one profile's shared settings.
// validationSpec tracks raw DDA and accepted profile values, without defaults.
// defaultDDAISpec receives accepted settings on the generated default DDAI.
// originalDefaultDDAISpec is read-only context for prerequisites and inheritance.
// Discard both candidates on error.
type ProfileSharedConfigOverlayFunc func(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, profile *v2alpha1.DatadogAgentSpec) error

// DDASharedDependenciesFunc adds DDA-owned resources shared by all DDAIs.
// ddai and ddaiSpec are read-only; sharedSpec contains the final shared settings.
type DDASharedDependenciesFunc func(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddai metav1.Object, ddaiSpec, sharedSpec *v2alpha1.DatadogAgentSpec, managers ResourceManagers) error

// profileSharedConfigOverlays is populated by feature package init functions
// through RegisterProfileSharedConfigOverlay.
var profileSharedConfigOverlays = map[IDType]ProfileSharedConfigOverlayFunc{}

// ddaSharedDependencies is populated by feature package init functions through
// RegisterDDASharedDependencies.
var ddaSharedDependencies = map[IDType]DDASharedDependenciesFunc{}

// RegisterProfileSharedConfigOverlay registers a feature's shared profile overlay.
func RegisterProfileSharedConfigOverlay(id IDType, overlay ProfileSharedConfigOverlayFunc) error {
	if _, found := profileSharedConfigOverlays[id]; found {
		return fmt.Errorf("the profile shared config overlay %s is registered already", id)
	}
	profileSharedConfigOverlays[id] = overlay
	return nil
}

// RegisterDDASharedDependencies registers DDA shared dependency logic for a feature.
func RegisterDDASharedDependencies(id IDType, dependency DDASharedDependenciesFunc) error {
	if _, found := ddaSharedDependencies[id]; found {
		return fmt.Errorf("DDA shared dependencies %s is registered already", id)
	}
	ddaSharedDependencies[id] = dependency
	return nil
}

// ApplyProfileSharedConfigOverlays runs all feature overlays for one profile.
// Commit both candidates only if every overlay succeeds.
func ApplyProfileSharedConfigOverlays(validationSpec, defaultDDAISpec, originalDefaultDDAISpec, profile *v2alpha1.DatadogAgentSpec) error {
	if validationSpec == nil || defaultDDAISpec == nil {
		return fmt.Errorf("profile shared config overlay target spec is nil")
	}

	sortedKeys := sortedProfileSharedConfigOverlayIDs()

	for _, id := range sortedKeys {
		if err := profileSharedConfigOverlays[id](validationSpec, defaultDDAISpec, originalDefaultDDAISpec, profile); err != nil {
			return fmt.Errorf("%s profile shared config overlay failed: %w", id, err)
		}
	}

	return nil
}

// sortedProfileSharedConfigOverlayIDs keeps feature execution order consistent.
func sortedProfileSharedConfigOverlayIDs() []IDType {
	ids := make([]IDType, 0, len(profileSharedConfigOverlays))
	for id := range profileSharedConfigOverlays {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
}

// ApplyDDASharedDependencies applies all registered DDA shared dependency hooks
// for one rendered DDAI spec.
func ApplyDDASharedDependencies(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddai metav1.Object, ddaiSpec, sharedSpec *v2alpha1.DatadogAgentSpec, managers ResourceManagers) error {
	// Sort feature IDs because map iteration order is unspecified.
	sortedKeys := make([]IDType, 0, len(ddaSharedDependencies))
	for key := range ddaSharedDependencies {
		sortedKeys = append(sortedKeys, key)
	}
	slices.Sort(sortedKeys)

	for _, id := range sortedKeys {
		if err := ddaSharedDependencies[id](dda, ddaSpec, ddai, ddaiSpec, sharedSpec, managers); err != nil {
			return fmt.Errorf("%s DDA shared dependencies failed: %w", id, err)
		}
	}

	return nil
}
