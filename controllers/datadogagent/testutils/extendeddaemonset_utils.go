// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testutils_test

import (
	"testing"

	edsdatadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

// CheckExtendedDaemonSetFunc define the signature of ExtendedDaemonSet's Check function.
type CheckExtendedDaemonSetFunc func(t *testing.T, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet)

// CheckPodTemplateInEDS used to execute a CheckPodTemplateFunc function on an ExtendedDaemonSet instance
func CheckPodTemplateInEDS(templateCheck CheckPodTemplateFunc) CheckExtendedDaemonSetFunc {
	check := func(t *testing.T, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) {
		templateCheck(t, &eds.Spec.Template)
	}
	return check
}

// CheckMetadaInEDS used to execute a CheckExtendedDaemonSetFunc function on an ExtendedDaemonSet instance
func CheckMetadaInEDS(metaCheck CheckObjectMeta) CheckExtendedDaemonSetFunc {
	check := func(t *testing.T, eds *edsdatadoghqv1alpha1.ExtendedDaemonSet) {
		metaCheck(t, &eds.ObjectMeta)
	}
	return check
}
