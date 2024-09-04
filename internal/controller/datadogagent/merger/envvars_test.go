// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package merger

import (
	"reflect"
	"testing"

	commonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	"github.com/stretchr/testify/assert"

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

func TestAddEnvVarWithMergeFunc(t *testing.T) {
	podTmpl := corev1.PodTemplateSpec{}
	envvarFoo := &corev1.EnvVar{
		Name:  "foo",
		Value: "foovalue",
	}
	tests := []struct {
		name        string
		description string
		containers  []corev1.Container
		want        []corev1.EnvVar
	}{
		{
			name:        "node agent container",
			description: "all containers should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.CoreAgentContainerName),
				},
				{
					Name: string(commonv1.TraceAgentContainerName),
				},
				{
					Name: string(commonv1.ProcessAgentContainerName),
				},
				{
					Name: string(commonv1.SecurityAgentContainerName),
				},
				{
					Name: string(commonv1.SystemProbeContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "node agent container with fips",
			description: "all containers except fips should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.CoreAgentContainerName),
				},
				{
					Name: string(commonv1.TraceAgentContainerName),
				},
				{
					Name: string(commonv1.FIPSProxyContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "dca container",
			description: "all containers should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.ClusterAgentContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "dca container with fips",
			description: "all containers except fips should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.ClusterAgentContainerName),
				},
				{
					Name: string(commonv1.FIPSProxyContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "ccr container",
			description: "all containers should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.ClusterChecksRunnersContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "ccr container with fips",
			description: "all containers except fips should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.ClusterChecksRunnersContainerName),
				},
				{
					Name: string(commonv1.FIPSProxyContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "optimized container",
			description: "all containers should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.UnprivilegedSingleAgentContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
		{
			name:        "optimized container with fips",
			description: "all containers except fips should have env var added",
			containers: []corev1.Container{
				{
					Name: string(commonv1.UnprivilegedSingleAgentContainerName),
				},
				{
					Name: string(commonv1.FIPSProxyContainerName),
				},
			},
			want: []corev1.EnvVar{*envvarFoo},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("description: %s", tt.description)
			manager := &envVarManagerImpl{
				podTmpl: &podTmpl,
			}
			err := manager.AddEnvVarWithMergeFunc(envvarFoo, DefaultEnvVarMergeFunction)
			assert.NoError(t, err)

			for _, cont := range manager.podTmpl.Spec.Containers {
				if cont.Name == string(commonv1.FIPSProxyContainerName) {
					assert.Len(t, cont.Env, 0)
				} else {
					assert.Contains(t, cont.Env, tt.want)
				}
			}
		})
	}
}
