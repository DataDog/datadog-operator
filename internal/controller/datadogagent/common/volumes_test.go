// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestGetVolumeForRmCorechecks(t *testing.T) {
	vol := GetVolumeForRmCorechecks()

	assert.Equal(t, RmCorechecksVolumeName, vol.Name)
	assert.NotNil(t, vol.EmptyDir, "remove-corechecks must be backed by an emptyDir so the init container can seed it")
}

func TestGetVolumeMountForRmCorechecks(t *testing.T) {
	vm := GetVolumeMountForRmCorechecks()

	assert.Equal(t, RmCorechecksVolumeName, vm.Name)
	// This mount overlays the agent conf.d so default core checks do not run.
	assert.Equal(t, fmt.Sprintf("%s/%s", ConfigVolumePath, "conf.d"), vm.MountPath)
}

func TestGetVolumeMountForRmCorechecksInit(t *testing.T) {
	vm := GetVolumeMountForRmCorechecksInit()

	// Shares the remove-corechecks volume ...
	assert.Equal(t, RmCorechecksVolumeName, vm.Name)
	// ... but mounts it at a scratch path, NOT over the agent conf.d, so the
	// init container can still read the image's packaged conf.d assets.
	assert.Equal(t, RmCorechecksConfdInitPath, vm.MountPath)
	assert.NotEqual(t, fmt.Sprintf("%s/%s", ConfigVolumePath, "conf.d"), vm.MountPath)
}

// TestRmCorechecksInitAndOverlayShareVolume documents the contract the init
// container relies on: the seeding mount and the runtime overlay mount refer to
// the same volume at different paths, so assets copied by the init container are
// visible under the agent conf.d overlay at runtime.
func TestRmCorechecksInitAndOverlayShareVolume(t *testing.T) {
	initMount := GetVolumeMountForRmCorechecksInit()
	overlayMount := GetVolumeMountForRmCorechecks()

	assert.Equal(t, initMount.Name, overlayMount.Name)
	assert.NotEqual(t, initMount.MountPath, overlayMount.MountPath)

	// Both mounts must reference the volume produced by GetVolumeForRmCorechecks.
	vol := GetVolumeForRmCorechecks()
	for _, vm := range []corev1.VolumeMount{initMount, overlayMount} {
		assert.Equal(t, vol.Name, vm.Name)
	}
}
