// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"reflect"
	"testing"

	"github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestAddVolumeMountToContainer(t *testing.T) {
	volumemountFoo := &corev1.VolumeMount{
		Name:      "foo",
		MountPath: "/path/foo",
	}
	volumemountFoo2 := &corev1.VolumeMount{
		Name:      "foo",
		MountPath: "/path/foo2",
	}
	volumemountBar := &corev1.VolumeMount{
		Name:      "bar",
		MountPath: "/path/bar",
	}
	type args struct {
		container   *corev1.Container
		volumemount *corev1.VolumeMount
		mergeFunc   VolumeMountMergeFunction
	}
	tests := []struct {
		name        string
		description string
		args        args
		want        []corev1.VolumeMount
		wantErr     bool
	}{
		{
			name:        "container.volumeMount is empty, nil mergefunction ",
			description: "the merge function is nil, it should default to DefaultVolumeMountMergeFunction",
			args: args{
				container:   &corev1.Container{},
				volumemount: volumemountFoo,
				mergeFunc:   nil,
			},
			wantErr: false,
			want:    []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "volumemount already set",
			description: "the merge function is nil, it should default to DefaultVolumeMountMergeFunction",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{*volumemountFoo},
				},
				volumemount: volumemountFoo2,
				mergeFunc:   nil,
			},
			wantErr: false,
			want:    []corev1.VolumeMount{*volumemountFoo2},
		},
		{
			name:        "volumemount already set",
			description: "the merge function is nil, it should default to DefaultVolumeMountMergeFunction",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{*volumemountFoo},
				},
				volumemount: volumemountBar,
				mergeFunc:   nil,
			},
			wantErr: false,
			want:    []corev1.VolumeMount{*volumemountFoo, *volumemountBar},
		},

		{
			name:        "volumemount already set, ignore new value",
			description: "the merge function is IgnoreNewVolumeMountMergeFunction",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{*volumemountFoo},
				},
				volumemount: volumemountFoo2,
				mergeFunc:   IgnoreNewVolumeMountMergeFunction,
			},
			wantErr: false,
			want:    []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "volumemount already set, avoid merge",
			description: "the merge function is nil, it should default to DefaultVolumeMountMergeFunction",
			args: args{
				container: &corev1.Container{
					VolumeMounts: []corev1.VolumeMount{*volumemountFoo},
				},
				volumemount: volumemountFoo2,
				mergeFunc:   ErrorOnMergeAttemptdVolumeMountMergeFunction,
			},
			wantErr: true,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("description: %s", tt.description)
			got, err := AddVolumeMountToContainerWithMergeFunc(tt.args.container, tt.args.volumemount, tt.args.mergeFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddVolumeMountToContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) && !IsMergeAttemptedError(err) {
				t.Errorf("error = %v should be a MergeAttemptedError type", err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddVolumeMountToContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddVolumeMountWithMergeFunc(t *testing.T) {
	podTmpl := corev1.PodTemplateSpec{}
	volumemountFoo := &corev1.VolumeMount{
		Name:      "foo",
		MountPath: "/path/foo",
	}
	tests := []struct {
		name        string
		description string
		containers  []corev1.Container
		want        []corev1.VolumeMount
	}{
		{
			name:        "node agent container",
			description: "all containers should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.CoreAgentContainerName),
				},
				{
					Name: string(common.TraceAgentContainerName),
				},
				{
					Name: string(common.ProcessAgentContainerName),
				},
				{
					Name: string(common.SecurityAgentContainerName),
				},
				{
					Name: string(common.SystemProbeContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "node agent container with fips",
			description: "all containers except fips should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.CoreAgentContainerName),
				},
				{
					Name: string(common.TraceAgentContainerName),
				},
				{
					Name: string(common.FIPSProxyContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "dca container",
			description: "all containers should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.ClusterAgentContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "dca container with fips",
			description: "all containers except fips should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.ClusterAgentContainerName),
				},
				{
					Name: string(common.FIPSProxyContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "ccr container",
			description: "all containers should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.ClusterChecksRunnersContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "ccr container with fips",
			description: "all containers except fips should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.ClusterChecksRunnersContainerName),
				},
				{
					Name: string(common.FIPSProxyContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "optimized container",
			description: "all containers should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.UnprivilegedSingleAgentContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
		{
			name:        "optimized container with fips",
			description: "all containers except fips should have volume mount added",
			containers: []corev1.Container{
				{
					Name: string(common.UnprivilegedSingleAgentContainerName),
				},
				{
					Name: string(common.FIPSProxyContainerName),
				},
			},
			want: []corev1.VolumeMount{*volumemountFoo},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("description: %s", tt.description)
			manager := &volumeMountManagerImpl{
				podTmpl: &podTmpl,
			}
			err := manager.AddVolumeMountWithMergeFunc(volumemountFoo, DefaultVolumeMountMergeFunction)
			assert.NoError(t, err)

			for _, cont := range manager.podTmpl.Spec.Containers {
				if cont.Name == string(common.FIPSProxyContainerName) {
					assert.Len(t, cont.VolumeMounts, 0)
				} else {
					assert.Contains(t, cont.VolumeMounts, tt.want)
				}
			}
		})
	}
}
