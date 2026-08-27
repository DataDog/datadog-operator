// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package providers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func TestResolve(t *testing.T) {
	current := &corev1.EnvVar{Name: "DD_X", Value: "baseline"}
	replacement := &corev1.EnvVar{Name: "DD_X", Value: "provider"}

	tests := []struct {
		name   string
		rules  map[string]Rule[corev1.EnvVar]
		want   *corev1.EnvVar
		wantOk bool
	}{
		{
			name:   "no rule keeps the caller's value",
			rules:  nil,
			want:   current,
			wantOk: true,
		},
		{
			name:   "rule for another name keeps the caller's value",
			rules:  map[string]Rule[corev1.EnvVar]{"DD_Y": {Verb: Deny}},
			want:   current,
			wantOk: true,
		},
		{
			name:   "deny drops the config",
			rules:  map[string]Rule[corev1.EnvVar]{"DD_X": {Verb: Deny}},
			want:   nil,
			wantOk: false,
		},
		{
			name:   "replace substitutes the value",
			rules:  map[string]Rule[corev1.EnvVar]{"DD_X": {Verb: Replace, Value: replacement}},
			want:   replacement,
			wantOk: true,
		},
		{
			name:   "replace with no value keeps the caller's value",
			rules:  map[string]Rule[corev1.EnvVar]{"DD_X": {Verb: Replace}},
			want:   current,
			wantOk: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, allow := resolve(tt.rules, current.Name, current)
			assert.Equal(t, tt.wantOk, allow)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestApply(t *testing.T) {
	baseline := []corev1.Volume{{Name: "logs"}, {Name: "src"}, {Name: "tmp"}}

	tests := []struct {
		name     string
		rules    map[string]Rule[corev1.Volume]
		baseline []corev1.Volume
		want     []corev1.Volume
	}{
		{
			name:     "no rules returns the baseline untouched",
			rules:    nil,
			baseline: baseline,
			want:     baseline,
		},
		{
			name:     "deny drops the entry",
			rules:    map[string]Rule[corev1.Volume]{"src": {Verb: Deny}},
			baseline: baseline,
			want:     []corev1.Volume{{Name: "logs"}, {Name: "tmp"}},
		},
		{
			name:     "replace substitutes in place",
			rules:    map[string]Rule[corev1.Volume]{"src": {Verb: Replace, Value: &corev1.Volume{Name: "src", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}}}},
			baseline: baseline,
			want: []corev1.Volume{
				{Name: "logs"},
				{Name: "src", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "tmp"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, Provider{volumes: tt.rules}.ApplyVolumes(tt.baseline))
		})
	}
}

func TestRegister(t *testing.T) {
	name := Name("test-provider")
	t.Cleanup(func() { delete(registry, name) })

	assert.NoError(t, register(name, Provider{}))
	assert.Error(t, register(name, Provider{}), "a duplicate name is an error")
}

func TestGet(t *testing.T) {
	assert.Equal(t, Default(), Get("no-such-provider"), "an unknown name deviates in nothing")
	assert.Equal(t, Default(), Get(""), "an empty name deviates in nothing")

	gke := Get(kubernetes.GKECosProvider)
	assert.Equal(t, Rule[corev1.Volume]{Verb: Deny}, gke.volumes[common.SrcVolumeName])
	assert.Equal(t, Rule[corev1.VolumeMount]{Verb: Deny}, gke.volumeMounts[common.SrcVolumeName])
}
