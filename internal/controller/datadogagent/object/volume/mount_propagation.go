// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package volume

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// GetMountPropagationMode extracts the HostVolumeMountPropagation from the global config.
// Returns nil if global config is nil or the field is not set.
func GetMountPropagationMode(global *v2alpha1.GlobalConfig) *corev1.MountPropagationMode {
	if global == nil {
		return nil
	}
	return global.HostVolumeMountPropagation
}
