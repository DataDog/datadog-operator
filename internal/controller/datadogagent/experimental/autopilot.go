// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package experimental

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/allowlistsynchronizer"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// IsAutopilotEnabled reports whether GKE Autopilot handling should apply, via
// either the provider annotation (datadoghq.com/provider: gke-autopilot — the
// value the DDA controller stamps onto the DDAI) or the experimental opt-in
// annotation on the DDA.
func IsAutopilotEnabled(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	ann := obj.GetAnnotations()
	if ann == nil {
		return false
	}

	if ann[kubernetes.ProviderAnnotationKey] == kubernetes.GKEAutopilotProvider {
		return true
	}

	return strings.EqualFold(ann[getExperimentalAnnotationKey(ExperimentalAutopilotSubkey)], "true")
}

// applyExperimentalAutopilotOverrides creates the GKE Autopilot WorkloadAllowlist
// synchronizer when Autopilot is enabled. Pod-template mutations are handled by
// the provider-capabilities framework (see IsAutopilotEnabled doc).
func applyExperimentalAutopilotOverrides(dda metav1.Object, _ feature.PodTemplateManagers) {
	if IsAutopilotEnabled(dda) {
		allowlistsynchronizer.CreateAllowlistSynchronizer(
			getExperimentalAnnotation(dda, ExperimentalAutopilotAllowlistVersionSubkey),
			object.NewPartOfLabelValue(dda).String(),
			commonLabelsFromObject(dda),
		)
	}
}

// CreateDatadogCSIAllowlistSynchronizer creates the Datadog CSI driver
// WorkloadAllowlist synchronizer when Autopilot support is enabled.
func CreateDatadogCSIAllowlistSynchronizer(dda metav1.Object) {
	if !IsAutopilotEnabled(dda) {
		return
	}

	allowlistsynchronizer.CreateCSIAllowlistSynchronizer(
		getExperimentalAnnotation(dda, ExperimentalAutopilotCSIAllowlistVersionSubkey),
		object.NewPartOfLabelValue(dda).String(),
		commonLabelsFromObject(dda),
	)
}

// commonLabelsFromObject extracts spec.global.commonLabels from a DatadogAgent
// or DatadogAgentInternal object, returning nil for any other type.
func commonLabelsFromObject(obj metav1.Object) map[string]string {
	switch d := obj.(type) {
	case *v2alpha1.DatadogAgent:
		if d.Spec.Global != nil {
			return d.Spec.Global.CommonLabels
		}
	case *v1alpha1.DatadogAgentInternal:
		if d.Spec.Global != nil {
			return d.Spec.Global.CommonLabels
		}
	}
	return nil
}
