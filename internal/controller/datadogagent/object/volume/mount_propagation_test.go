// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package volume

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/utils/ptr"
)

func TestApplyMountPropagation(t *testing.T) {
	hostToContainer := corev1.MountPropagationHostToContainer
	bidirectional := corev1.MountPropagationBidirectional

	tests := []struct {
		name        string
		podTemplate *corev1.PodTemplateSpec
		mode        *corev1.MountPropagationMode
		wantMounts  map[string]*corev1.MountPropagationMode // container name -> mount name -> expected propagation
	}{
		{
			name: "nil mode is a no-op",
			podTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "hostVol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/host"}}},
					},
					Containers: []corev1.Container{
						{
							Name: "agent",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "hostVol", MountPath: "/host"},
							},
						},
					},
				},
			},
			mode:       nil,
			wantMounts: map[string]*corev1.MountPropagationMode{"hostVol": nil},
		},
		{
			name: "sets propagation on host path volume mounts",
			podTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "hostVol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/host"}}},
						{Name: "emptyVol", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
					},
					Containers: []corev1.Container{
						{
							Name: "agent",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "hostVol", MountPath: "/host"},
								{Name: "emptyVol", MountPath: "/empty"},
							},
						},
					},
				},
			},
			mode: &hostToContainer,
			wantMounts: map[string]*corev1.MountPropagationMode{
				"hostVol":  &hostToContainer,
				"emptyVol": nil,
			},
		},
		{
			name: "does not override explicitly set propagation",
			podTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "hostVol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/host"}}},
					},
					Containers: []corev1.Container{
						{
							Name: "agent",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "hostVol", MountPath: "/host", MountPropagation: ptr.To(bidirectional)},
							},
						},
					},
				},
			},
			mode: &hostToContainer,
			wantMounts: map[string]*corev1.MountPropagationMode{
				"hostVol": &bidirectional, // existing value preserved
			},
		},
		{
			name: "applies to init containers",
			podTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "hostVol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/host"}}},
					},
					InitContainers: []corev1.Container{
						{
							Name: "init",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "hostVol", MountPath: "/host"},
							},
						},
					},
				},
			},
			mode:       &hostToContainer,
			wantMounts: map[string]*corev1.MountPropagationMode{"hostVol": &hostToContainer},
		},
		{
			name: "applies to multiple containers",
			podTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{Name: "hostVol", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/host"}}},
					},
					Containers: []corev1.Container{
						{
							Name: "agent",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "hostVol", MountPath: "/host"},
							},
						},
						{
							Name: "process-agent",
							VolumeMounts: []corev1.VolumeMount{
								{Name: "hostVol", MountPath: "/host"},
							},
						},
					},
				},
			},
			mode: &hostToContainer,
		},
		{
			name: "no volumes is a no-op",
			podTemplate: &corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "agent"},
					},
				},
			},
			mode: &hostToContainer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ApplyMountPropagation(tt.podTemplate, tt.mode)

			if tt.wantMounts != nil {
				// Check regular containers
				for _, c := range tt.podTemplate.Spec.Containers {
					for _, vm := range c.VolumeMounts {
						if expected, ok := tt.wantMounts[vm.Name]; ok {
							assert.Equal(t, expected, vm.MountPropagation,
								"container %s, mount %s", c.Name, vm.Name)
						}
					}
				}
				// Check init containers
				for _, c := range tt.podTemplate.Spec.InitContainers {
					for _, vm := range c.VolumeMounts {
						if expected, ok := tt.wantMounts[vm.Name]; ok {
							assert.Equal(t, expected, vm.MountPropagation,
								"init container %s, mount %s", c.Name, vm.Name)
						}
					}
				}
			}

			// For multi-container test, verify all containers got the propagation
			if tt.name == "applies to multiple containers" {
				for _, c := range tt.podTemplate.Spec.Containers {
					for _, vm := range c.VolumeMounts {
						if vm.Name == "hostVol" {
							assert.Equal(t, tt.mode, vm.MountPropagation,
								"container %s, mount %s", c.Name, vm.Name)
						}
					}
				}
			}
		})
	}
}
