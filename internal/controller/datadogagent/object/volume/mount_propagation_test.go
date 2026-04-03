// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package volume

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func TestWithMountPropagation(t *testing.T) {
	hostToContainer := corev1.MountPropagationHostToContainer
	bidirectional := corev1.MountPropagationBidirectional

	tests := []struct {
		name     string
		mode     *corev1.MountPropagationMode
		wantMode *corev1.MountPropagationMode
	}{
		{
			name:     "nil mode does not set propagation",
			mode:     nil,
			wantMode: nil,
		},
		{
			name:     "HostToContainer is set",
			mode:     &hostToContainer,
			wantMode: &hostToContainer,
		},
		{
			name:     "Bidirectional is set",
			mode:     &bidirectional,
			wantMode: &bidirectional,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, vm := GetVolumes("test-vol", "/host/path", "/mount/path", true, WithMountPropagation(tt.mode))
			assert.Equal(t, tt.wantMode, vm.MountPropagation)
		})
	}
}

func TestGetVolumesWithoutOptions(t *testing.T) {
	// Existing callers without options should still work and have nil propagation
	_, vm := GetVolumes("test-vol", "/host/path", "/mount/path", true)
	assert.Nil(t, vm.MountPropagation)
	assert.Equal(t, "test-vol", vm.Name)
	assert.Equal(t, "/mount/path", vm.MountPath)
	assert.True(t, vm.ReadOnly)
}

func TestGetMountPropagationMode(t *testing.T) {
	hostToContainer := corev1.MountPropagationHostToContainer

	tests := []struct {
		name   string
		global *v2alpha1.GlobalConfig
		want   *corev1.MountPropagationMode
	}{
		{
			name:   "nil global config",
			global: nil,
			want:   nil,
		},
		{
			name:   "global config without propagation",
			global: &v2alpha1.GlobalConfig{},
			want:   nil,
		},
		{
			name: "global config with propagation",
			global: &v2alpha1.GlobalConfig{
				HostVolumeMountPropagation: &hostToContainer,
			},
			want: &hostToContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetMountPropagationMode(tt.global)
			assert.Equal(t, tt.want, got)
		})
	}
}
