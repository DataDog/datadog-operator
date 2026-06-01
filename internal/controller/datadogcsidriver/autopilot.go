// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

import (
	"strings"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
)

const (
	// GKEAutopilotAnnotation enables GKE Autopilot mode for the CSI driver.
	// When set to "true", the controller creates an AllowlistSynchronizer and
	// adds the cloud.google.com/matching-allowlist label to the DaemonSet.
	GKEAutopilotAnnotation = "csi.datadoghq.com/gke-autopilot"

	// GKEAutopilotAllowlistVersionAnnotation allows overriding the CSI driver
	// WorkloadAllowlist version. If not set, defaults to v1.1.0.
	GKEAutopilotAllowlistVersionAnnotation = "csi.datadoghq.com/gke-autopilot-allowlist-version"

	// GKEAutopilotLegacyAnnotation indicates the cluster is running legacy Autopilot
	// (only AllowlistedV2Workload CRD, not WorkloadAllowlist). When set to "true",
	// the storage-dir volume and DD_APM_ENABLED env var are removed from the DaemonSet
	// because no updated exemption exists for legacy Autopilot.
	GKEAutopilotLegacyAnnotation = "csi.datadoghq.com/gke-autopilot-legacy"

	// GKEMatchingAllowlistLabelKey is the label key used by GKE Autopilot to match
	// workloads with WorkloadAllowlists.
	GKEMatchingAllowlistLabelKey = "cloud.google.com/matching-allowlist"
)

// IsGKEAutopilotEnabled returns true if the CSI driver is configured for GKE Autopilot.
func IsGKEAutopilotEnabled(instance *datadoghqv1alpha1.DatadogCSIDriver) bool {
	if instance == nil {
		return false
	}
	ann := instance.GetAnnotations()
	if ann == nil {
		return false
	}
	return strings.EqualFold(ann[GKEAutopilotAnnotation], "true")
}

// IsGKEAutopilotLegacy returns true if the cluster is running legacy Autopilot
// (only AllowlistedV2Workload CRD, not the newer WorkloadAllowlist).
func IsGKEAutopilotLegacy(instance *datadoghqv1alpha1.DatadogCSIDriver) bool {
	if instance == nil {
		return false
	}
	ann := instance.GetAnnotations()
	if ann == nil {
		return false
	}
	return strings.EqualFold(ann[GKEAutopilotLegacyAnnotation], "true")
}

// GetGKEAutopilotAllowlistVersion returns the configured WorkloadAllowlist version
// from annotations, or empty string to use the default.
func GetGKEAutopilotAllowlistVersion(instance *datadoghqv1alpha1.DatadogCSIDriver) string {
	if instance == nil {
		return ""
	}
	ann := instance.GetAnnotations()
	if ann == nil {
		return ""
	}
	return ann[GKEAutopilotAllowlistVersionAnnotation]
}

// CreateCSIAllowlistSynchronizerIfNeeded creates the AllowlistSynchronizer for the
// CSI driver if GKE Autopilot is enabled and not in legacy mode.
func CreateCSIAllowlistSynchronizerIfNeeded(instance *datadoghqv1alpha1.DatadogCSIDriver) {
	if !IsGKEAutopilotEnabled(instance) {
		return
	}
	if IsGKEAutopilotLegacy(instance) {
		return
	}

	version := GetGKEAutopilotAllowlistVersion(instance)
	partOfLabel := object.NewPartOfLabelValue(instance).String()
	allowlistsynchronizer.CreateCSIAllowlistSynchronizer(version, partOfLabel)
}

// GetGKEAutopilotLabels returns the labels needed for the CSI driver DaemonSet
// on GKE Autopilot. Returns nil if Autopilot is not enabled or is in legacy mode.
func GetGKEAutopilotLabels(instance *datadoghqv1alpha1.DatadogCSIDriver) map[string]string {
	if !IsGKEAutopilotEnabled(instance) {
		return nil
	}
	if IsGKEAutopilotLegacy(instance) {
		return nil
	}

	version := GetGKEAutopilotAllowlistVersion(instance)
	return map[string]string{
		GKEMatchingAllowlistLabelKey: allowlistsynchronizer.GetCSIMatchingAllowlistLabelValue(version),
	}
}
