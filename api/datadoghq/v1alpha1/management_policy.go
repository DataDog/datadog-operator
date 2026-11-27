// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// ManagementPolicyAnnotationKey sets how the Operator manages a
	// DatadogGenericResource, DatadogMonitor, DatadogDashboard or DatadogSLO.
	// Without the annotation, the Operator manages the CR as usual.
	ManagementPolicyAnnotationKey = "datadoghq.com/management-policy"
	// ManagementPolicyOrphan stops the Operator from managing the CR: it no
	// longer creates, updates or polls the Datadog object, and deleting the CR
	// leaves the Datadog object in place.
	ManagementPolicyOrphan = "orphan"
)

// IsOrphaned reports whether obj has the orphan management policy.
func IsOrphaned(obj metav1.Object) bool {
	if obj == nil {
		return false
	}
	return obj.GetAnnotations()[ManagementPolicyAnnotationKey] == ManagementPolicyOrphan
}
