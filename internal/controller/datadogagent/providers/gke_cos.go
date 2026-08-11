// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
)

// gkeCosDeniedVolumes are volumes (and their mounts) that must not be added
// on GKE nodes running Container-Optimized OS. Shared by Volume and
// VolumeMount so a name is only listed once.
var gkeCosDeniedVolumes = map[string]struct{}{
	common.SrcVolumeName: {}, // COS nodes have no /usr/src kernel headers
}

type gkeCosProvider struct {
	defaultProvider
}

func (gkeCosProvider) Volume(name string) (*corev1.Volume, bool) {
	if _, denied := gkeCosDeniedVolumes[name]; denied {
		return nil, true
	}
	return nil, false
}

func (gkeCosProvider) VolumeMount(name string) (*corev1.VolumeMount, bool) {
	if _, denied := gkeCosDeniedVolumes[name]; denied {
		return nil, true
	}
	return nil, false
}
