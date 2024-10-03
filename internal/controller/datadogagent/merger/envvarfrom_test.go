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

func TestAddEnvFromSourceFromToContainer(t *testing.T) {
	envFromSourceFoo := &corev1.EnvFromSource{
		Prefix: "FOO_",
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "foo",
			},
		},
	}
	envFromSourceFoo2 := &corev1.EnvFromSource{
		Prefix: "FOO2_",
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "foo",
			},
		},
	}
	envFromSourceBar := &corev1.EnvFromSource{
		Prefix: "BAR_",
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "foo",
			},
		},
	}
	envFromSourceBar2 := &corev1.EnvFromSource{
		Prefix: "BAR2_",
		SecretRef: &corev1.SecretEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "foo",
			},
		},
	}
	type args struct {
		container     *corev1.Container
		envFromSource *corev1.EnvFromSource
		mergeFunc     EnvFromSourceFromMergeFunction
	}
	tests := []struct {
		name    string
		args    args
		want    []corev1.EnvFromSource
		wantErr bool
	}{
		// configmap
		{
			name: "Container.EnvFrom no present",
			args: args{
				container: &corev1.Container{
					EnvFrom: nil,
				},
				envFromSource: envFromSourceFoo2,
				mergeFunc:     nil,
			},
			want: []corev1.EnvFromSource{
				*envFromSourceFoo2,
			},
			wantErr: false,
		},
		{
			name: "Container.EnvFrom already present, allow override",
			args: args{
				container: &corev1.Container{
					EnvFrom: []corev1.EnvFromSource{
						*envFromSourceFoo,
					},
				},
				envFromSource: envFromSourceFoo2,
				mergeFunc:     nil,
			},
			want: []corev1.EnvFromSource{
				*envFromSourceFoo2,
			},
			wantErr: false,
		},
		// secret
		{
			name: "Container.EnvFrom(secret) already present, allow override",
			args: args{
				container: &corev1.Container{
					EnvFrom: []corev1.EnvFromSource{
						*envFromSourceBar,
					},
				},
				envFromSource: envFromSourceBar2,
				mergeFunc:     nil,
			},
			want: []corev1.EnvFromSource{
				*envFromSourceBar2,
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AddEnvFromSourceFromToContainer(tt.args.container, tt.args.envFromSource, tt.args.mergeFunc)
			if (err != nil) != tt.wantErr {
				t.Errorf("AddEnvFromSourceFromToContainer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if (err != nil) && !IsMergeAttemptedError(err) {
				t.Errorf("error = %v should be a MergeAttemptedError type", err)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("AddEnvFromSourceFromToContainer() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAddEnvFromVarWithMergeFunc(t *testing.T) {
	podTmpl := corev1.PodTemplateSpec{}
	envFromVarFoo := &corev1.EnvFromSource{
		Prefix: "FOO",
		ConfigMapRef: &corev1.ConfigMapEnvSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: "foo",
			},
		},
	}
	tests := []struct {
		name        string
		description string
		containers  []corev1.Container
		want        []corev1.EnvFromSource
	}{
		{
			name:        "overrides for nodeAgent, clusterAgent, clusterChecks",
			description: "all containers should have env var added",
			containers: []corev1.Container{
				{
					Name: string(common.ClusterChecksRunnersContainerName),
				},
				{
					Name: string(common.CoreAgentContainerName),
				},
				{
					Name: string(common.ClusterAgentContainerName),
				},
			},
			want: []corev1.EnvFromSource{*envFromVarFoo},
		},
		{
			name:        "extra containers shouldn't be overridden",
			description: "only agent containers should have env var added",
			containers: []corev1.Container{
				{
					Name: string(common.ClusterChecksRunnersContainerName),
				},
				{
					Name: string(common.CoreAgentContainerName),
				},
				{
					Name: string(common.ClusterAgentContainerName),
				},
				{
					Name: string(common.InitVolumeContainerName),
				},
			},
			want: []corev1.EnvFromSource{*envFromVarFoo},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("description: %s", tt.description)
			podTmpl.Spec.Containers = tt.containers
			manager := &envFromVarManagerImpl{
				podTmpl: &podTmpl,
			}
			err := manager.AddEnvFromVarWithMergeFunc(envFromVarFoo, DefaultEnvFromSourceFromMergeFunction)
			assert.NoError(t, err)

			for _, cont := range manager.podTmpl.Spec.Containers {
				if cont.Name == string(common.InitVolumeContainerName) {
					assert.Len(t, cont.EnvFrom, 0)
				} else {
					assert.ElementsMatch(t, cont.EnvFrom, tt.want)
				}
			}
		})
	}
}
