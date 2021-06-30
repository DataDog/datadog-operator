// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package patch

import "github.com/DataDog/datadog-operator/api/v1alpha1"

// CopyAndPatchDatadogAgent used to patch the current DatadogAgent instance to use the new fields (not deprecated).
// This function is here to ease the migration to a new DatadogAgent CRD version.
func CopyAndPatchDatadogAgent(da *v1alpha1.DatadogAgent) (*v1alpha1.DatadogAgent, bool) {
	newDA := da.DeepCopy()
	patched := patchFeatures(da, newDA)
	if patched {
		return newDA, patched
	}
	return da, patched
}

func patchFeatures(oldDA, newDA *v1alpha1.DatadogAgent) bool {
	var patched bool

	patched = patched || patchLogFeatures(oldDA, newDA)

	return patched
}

func patchLogFeatures(oldDA, newDA *v1alpha1.DatadogAgent) bool {
	var patched bool
	if v1alpha1.IsEqualStruct(oldDA.Spec.Agent, v1alpha1.DatadogAgentSpecAgentSpec{}) {
		return patched
	}
	// patch only if a value is not already set
	if newDA.Spec.Features.LogCollection == nil && oldDA.Spec.Agent.Log != nil {
		patched = true
		newDA.Spec.Features.LogCollection = oldDA.Spec.Agent.Log.DeepCopy()
	}

	return patched
}
