// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package feature

import (
	"fmt"
	"slices"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// ProfileSharedConfigOverlayFunc lets a feature lift profile config into shared
// components owned by the default DDAI, such as the Cluster Agent or Cluster
// Checks Runner.
//
// dst is the accumulated default DDAI spec for the current reconcile and may be
// mutated by the overlay. base is the original default DDAI spec before any
// profile overlays were applied, so overlays can distinguish base config from
// profile-contributed config when needed. profile is the current
// DatadogAgentProfile config being considered.
//
// Example: a profile can enable APM SSI for one node group, but SSI is
// configured on the Cluster Agent. The APM overlay merges that profile SSI
// config into dst while leaving the profile-specific node Agent config on the
// profile DDAI.
type ProfileSharedConfigOverlayFunc func(dst, base, profile *v2alpha1.DatadogAgentSpec) error

// profileSharedConfigOverlays is populated by feature package init functions
// through RegisterProfileSharedConfigOverlay.
var profileSharedConfigOverlays = map[IDType]ProfileSharedConfigOverlayFunc{}

// RegisterProfileSharedConfigOverlay registers profile shared-component merge
// logic for a feature.
func RegisterProfileSharedConfigOverlay(id IDType, overlay ProfileSharedConfigOverlayFunc) error {
	if _, found := profileSharedConfigOverlays[id]; found {
		return fmt.Errorf("the profile shared config overlay %s is registered already", id)
	}
	profileSharedConfigOverlays[id] = overlay
	return nil
}

// ApplyProfileSharedConfigOverlays applies all registered profile shared-component
// overlays to dst. The base spec is the original default DDAI spec before any
// profile overlays were applied; profile is the DatadogAgentProfile config.
func ApplyProfileSharedConfigOverlays(dst, base, profile *v2alpha1.DatadogAgentSpec) error {
	// Registration happens from feature package init functions, so sort IDs to
	// keep profile overlay behavior deterministic regardless of init order.
	sortedKeys := make([]IDType, 0, len(profileSharedConfigOverlays))
	for key := range profileSharedConfigOverlays {
		sortedKeys = append(sortedKeys, key)
	}
	slices.Sort(sortedKeys)

	for _, id := range sortedKeys {
		if err := profileSharedConfigOverlays[id](dst, base, profile); err != nil {
			return fmt.Errorf("%s profile shared config overlay failed: %w", id, err)
		}
	}

	return nil
}
