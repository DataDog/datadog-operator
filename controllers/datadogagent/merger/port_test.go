// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
)

func TestAddPortToContainer(t *testing.T) {
	portFoo := &corev1.ContainerPort{
		Name:          "foo",
		HostPort:      1234,
		ContainerPort: 1234,
	}
	portFoo2 := &corev1.ContainerPort{
		Name:          "foo",
		HostPort:      4567,
		ContainerPort: 4567,
	}
	portBar := &corev1.ContainerPort{
		Name:          "bar",
		HostPort:      7890,
		ContainerPort: 7890,
	}
	type args struct {
		container *corev1.Container
		port      *corev1.ContainerPort
		mergeFunc PortMergeFunction
	}
	tests := []struct {
		name        string
		description string
		args        args
		want        []corev1.ContainerPort
		wantErr     bool
	}{
		{
			name:        "container.ContainerPort is empty, nil mergefunction ",
			description: "the merge function is nil, it should default to DefaultPortMergeFunction",
			args: args{
				container: &corev1.Container{},
				port:      portFoo,
				mergeFunc: nil,
			},
			wantErr: false,
			want:    []corev1.ContainerPort{*portFoo},
		},
		{
			name:        "port already set",
			description: "the merge function is nil, it should default to DefaultPortMergeFunction",
			args: args{
				container: &corev1.Container{
					Ports: []corev1.ContainerPort{*portFoo},
				},
				port:      portFoo2,
				mergeFunc: nil,
			},
			wantErr: false,
			want:    []corev1.ContainerPort{*portFoo2},
		},
		{
			name:        "port already set",
			description: "the merge function is nil, it should default to DefaultPortMergeFunction",
			args: args{
				container: &corev1.Container{
					Ports: []corev1.ContainerPort{*portFoo},
				},
				port:      portBar,
				mergeFunc: nil,
			},
			wantErr: false,
			want:    []corev1.ContainerPort{*portFoo, *portBar},
		},

		{
			name:        "port already set, ignore new value",
			description: "the merge function is IgnoreNewPortMergeFunction",
			args: args{
				container: &corev1.Container{
					Ports: []corev1.ContainerPort{*portFoo},
				},
				port:      portFoo2,
				mergeFunc: IgnoreNewPortMergeFunction,
			},
			wantErr: false,
			want:    []corev1.ContainerPort{*portFoo},
		},
		{
			name:        "port already set, avoid merge",
			description: "the merge function is nil, it should default to DefaultPortMergeFunction",
			args: args{
				container: &corev1.Container{
					Ports: []corev1.ContainerPort{*portFoo},
				},
				port:      portFoo2,
				mergeFunc: ErrorOnMergeAttemptdPortMergeFunction,
			},
			wantErr: true,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("description: %s", tt.description)
			got, err := AddPortToContainer(tt.args.container, tt.args.port, tt.args.mergeFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddPortToContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) && !IsMergeAttemptedError(err) {
				t.Errorf("error = %v should be a MergeAttemptedError type", err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddPortToContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}
