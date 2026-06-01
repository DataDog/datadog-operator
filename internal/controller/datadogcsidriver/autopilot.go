// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
)

const (
	// GKEMatchingAllowlistLabelKey is the label key used by GKE Autopilot to match
	// workloads with WorkloadAllowlists.
	GKEMatchingAllowlistLabelKey = "cloud.google.com/matching-allowlist"

	// WorkloadAllowlist is the CRD Kind for modern GKE Autopilot (>= 1.32.1-gke.1729000)
	workloadAllowlistKind = "WorkloadAllowlist"

	// AllowlistedV2Workload is the CRD Kind for legacy GKE Autopilot
	allowlistedV2WorkloadKind = "AllowlistedV2Workload"
)

// GKEAutopilotMode represents the detected GKE Autopilot mode
type GKEAutopilotMode int

const (
	// GKEAutopilotModeNone means not running on GKE Autopilot
	GKEAutopilotModeNone GKEAutopilotMode = iota
	// GKEAutopilotModeModern means running on GKE Autopilot with WorkloadAllowlist support (>= 1.32.1-gke.1729000)
	GKEAutopilotModeModern
	// GKEAutopilotModeLegacy means running on legacy GKE Autopilot (only AllowlistedV2Workload)
	GKEAutopilotModeLegacy
)

// DetectGKEAutopilotMode detects the GKE Autopilot mode based on available CRDs
func DetectGKEAutopilotMode(platformInfo PlatformInfo) GKEAutopilotMode {
	if platformInfo == nil {
		return GKEAutopilotModeNone
	}

	// Modern Autopilot has WorkloadAllowlist CRD
	if platformInfo.IsResourceSupported(workloadAllowlistKind) {
		return GKEAutopilotModeModern
	}

	// Legacy Autopilot only has AllowlistedV2Workload CRD
	if platformInfo.IsResourceSupported(allowlistedV2WorkloadKind) {
		return GKEAutopilotModeLegacy
	}

	return GKEAutopilotModeNone
}

// IsGKEAutopilot returns true if running on any version of GKE Autopilot
func IsGKEAutopilot(platformInfo PlatformInfo) bool {
	return DetectGKEAutopilotMode(platformInfo) != GKEAutopilotModeNone
}

// IsGKEAutopilotModern returns true if running on modern GKE Autopilot with WorkloadAllowlist support
func IsGKEAutopilotModern(platformInfo PlatformInfo) bool {
	return DetectGKEAutopilotMode(platformInfo) == GKEAutopilotModeModern
}

// IsGKEAutopilotLegacy returns true if running on legacy GKE Autopilot
func IsGKEAutopilotLegacy(platformInfo PlatformInfo) bool {
	return DetectGKEAutopilotMode(platformInfo) == GKEAutopilotModeLegacy
}

// CreateCSIAllowlistSynchronizerIfNeeded creates the AllowlistSynchronizer for the
// CSI driver if running on modern GKE Autopilot (with WorkloadAllowlist support).
func CreateCSIAllowlistSynchronizerIfNeeded(instance *datadoghqv1alpha1.DatadogCSIDriver, platformInfo PlatformInfo) {
	if !IsGKEAutopilotModern(platformInfo) {
		return
	}

	partOfLabel := object.NewPartOfLabelValue(instance).String()
	// Use empty string to get the default version (v1.1.0)
	allowlistsynchronizer.CreateCSIAllowlistSynchronizer("", partOfLabel)
}

// GetGKEAutopilotLabels returns the labels needed for the CSI driver DaemonSet
// on GKE Autopilot. Returns nil if not on modern Autopilot.
func GetGKEAutopilotLabels(platformInfo PlatformInfo) map[string]string {
	if !IsGKEAutopilotModern(platformInfo) {
		return nil
	}

	return map[string]string{
		GKEMatchingAllowlistLabelKey: allowlistsynchronizer.CSIMatchingAllowlistLabel,
	}
}
