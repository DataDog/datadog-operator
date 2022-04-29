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

func TestAddEnvVarToContainer(t *testing.T) {
	envvarFoo := &corev1.EnvVar{
		Name:  "foo",
		Value: "foovalue",
	}
	envvarFoo2 := &corev1.EnvVar{
		Name:  "foo",
		Value: "foovalue2",
	}
	envvarBar := &corev1.EnvVar{
		Name:  "bar",
		Value: "barvalue",
	}
	type args struct {
		container *corev1.Container
		envvar    *corev1.EnvVar
		mergeFunc EnvVarMergeFunction
	}
	tests := []struct {
		name        string
		description string
		args        args
		want        []corev1.EnvVar
		wantErr     bool
	}{
		{
			name:        "container.env is empty, nil mergefunction ",
			description: "the merge function is nil, it should default to DefaultEnvVarMergeFunction",
			args: args{
				container: &corev1.Container{},
				envvar:    envvarFoo,
				mergeFunc: nil,
			},
			wantErr: false,
			want:    []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "envvar already set",
			description: "the merge function is nil, it should default to DefaultEnvVarMergeFunction",
			args: args{
				container: &corev1.Container{
					Env: []corev1.EnvVar{*envvarFoo},
				},
				envvar:    envvarFoo2,
				mergeFunc: nil,
			},
			wantErr: false,
			want:    []corev1.EnvVar{*envvarFoo2},
		},
		{
			name:        "envvar already set",
			description: "the merge function is nil, it should default to DefaultEnvVarMergeFunction",
			args: args{
				container: &corev1.Container{
					Env: []corev1.EnvVar{*envvarFoo},
				},
				envvar:    envvarBar,
				mergeFunc: nil,
			},
			wantErr: false,
			want:    []corev1.EnvVar{*envvarFoo, *envvarBar},
		},

		{
			name:        "envvar already set, ignore new value",
			description: "the merge function is IgnoreNewEnvVarMergeFunction",
			args: args{
				container: &corev1.Container{
					Env: []corev1.EnvVar{*envvarFoo},
				},
				envvar:    envvarFoo2,
				mergeFunc: IgnoreNewEnvVarMergeFunction,
			},
			wantErr: false,
			want:    []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "envvar already set, avoid merge",
			description: "the merge function is nil, it should default to DefaultEnvVarMergeFunction",
			args: args{
				container: &corev1.Container{
					Env: []corev1.EnvVar{*envvarFoo},
				},
				envvar:    envvarFoo2,
				mergeFunc: ErrorOnMergeAttemptdEnvVarMergeFunction,
			},
			wantErr: true,
			want:    nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("description: %s", tt.description)
			got, err := AddEnvVarToContainer(tt.args.container, tt.args.envvar, tt.args.mergeFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddEnvVarToContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) && !IsMergeAttemptedError(err) {
				t.Errorf("error = %v should be a MergeAttemptedError type", err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddEnvVarToContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}
