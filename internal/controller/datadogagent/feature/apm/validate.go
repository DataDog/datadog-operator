// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apm

import (
	"fmt"

	"k8s.io/utils/ptr"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func validateSSISharedComponentPrerequisites(spec *v2alpha1.DatadogAgentSpec) error {
	// SSI is rendered on the Cluster Agent admission controller path, so
	// profile-contributed SSI is only valid when that shared path exists.
	if !admissionControllerEnabled(spec) {
		return fmt.Errorf("features.admissionController.enabled must be true on the base DatadogAgent when APM instrumentation is configured")
	}
	if defaultClusterAgentDisabled(spec) {
		return fmt.Errorf("clusterAgent cannot be disabled on the base DatadogAgent when APM instrumentation is configured")
	}
	return nil
}

func validateSSITargetsSupported(spec *v2alpha1.DatadogAgentSpec) error {
	// Target-based SSI needs Cluster Agent support introduced in this version.
	if supportsInstrumentationTargets(spec) {
		return nil
	}
	return fmt.Errorf("features.apm.instrumentation.targets requires Cluster Agent version >= %s", minInstrumentationTargetsVersion)
}

func admissionControllerEnabled(spec *v2alpha1.DatadogAgentSpec) bool {
	return spec != nil &&
		spec.Features != nil &&
		spec.Features.AdmissionController != nil &&
		ptr.Deref(spec.Features.AdmissionController.Enabled, false)
}

func defaultClusterAgentDisabled(spec *v2alpha1.DatadogAgentSpec) bool {
	if spec == nil || spec.Override == nil || spec.Override[v2alpha1.ClusterAgentComponentName] == nil {
		return false
	}
	return ptr.Deref(spec.Override[v2alpha1.ClusterAgentComponentName].Disabled, false)
}
