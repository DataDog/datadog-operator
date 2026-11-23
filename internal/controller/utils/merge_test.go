// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package utils

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestMergeStringMaps(t *testing.T) {
	tests := []struct {
		name   string
		values []map[string]string
		want   map[string]string
	}{
		{
			name: "nil",
		},
		{
			name:   "merge in order",
			values: []map[string]string{{"shared": "first", "first": "value"}, nil, {"shared": "second", "second": "value"}},
			want:   map[string]string{"shared": "second", "first": "value", "second": "value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeStringMaps(tt.values...)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MergeStringMaps() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeEnv(t *testing.T) {
	tests := []struct {
		name     string
		base     []corev1.EnvVar
		override []corev1.EnvVar
		want     []corev1.EnvVar
	}{
		{
			name: "nil",
		},
		{
			name:     "replace by name and append new values",
			base:     []corev1.EnvVar{{Name: "FIRST", Value: "base"}, {Name: "SHARED", Value: "base"}},
			override: []corev1.EnvVar{{Name: "SHARED", Value: "override"}, {Name: "LAST", Value: "override"}},
			want:     []corev1.EnvVar{{Name: "FIRST", Value: "base"}, {Name: "SHARED", Value: "override"}, {Name: "LAST", Value: "override"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeEnv(tt.base, tt.override)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MergeEnv() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeVolumes(t *testing.T) {
	tests := []struct {
		name     string
		base     []corev1.Volume
		override []corev1.Volume
		want     []corev1.Volume
	}{
		{
			name: "nil",
		},
		{
			name: "replace by name and append new values",
			base: []corev1.Volume{
				{Name: "first", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "shared", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
			},
			override: []corev1.Volume{
				{Name: "shared", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "shared"}}},
				{Name: "last", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "last"}}}},
			},
			want: []corev1.Volume{
				{Name: "first", VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}}},
				{Name: "shared", VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{SecretName: "shared"}}},
				{Name: "last", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{LocalObjectReference: corev1.LocalObjectReference{Name: "last"}}}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeVolumes(tt.base, tt.override)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MergeVolumes() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeVolumeMounts(t *testing.T) {
	tests := []struct {
		name     string
		base     []corev1.VolumeMount
		override []corev1.VolumeMount
		want     []corev1.VolumeMount
	}{
		{
			name: "nil",
		},
		{
			name:     "replace by mount path and append new values",
			base:     []corev1.VolumeMount{{Name: "first", MountPath: "/first"}, {Name: "base-name", MountPath: "/shared"}},
			override: []corev1.VolumeMount{{Name: "override-name", MountPath: "/shared", ReadOnly: true}, {Name: "last", MountPath: "/last"}},
			want:     []corev1.VolumeMount{{Name: "first", MountPath: "/first"}, {Name: "override-name", MountPath: "/shared", ReadOnly: true}, {Name: "last", MountPath: "/last"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeVolumeMounts(tt.base, tt.override)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MergeVolumeMounts() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeTopologySpreadConstraints(t *testing.T) {
	tests := []struct {
		name     string
		base     []corev1.TopologySpreadConstraint
		override []corev1.TopologySpreadConstraint
		want     []corev1.TopologySpreadConstraint
	}{
		{
			name: "nil",
		},
		{
			name: "replace by topology key and action",
			base: []corev1.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: corev1.LabelHostname, WhenUnsatisfiable: corev1.DoNotSchedule},
				{MaxSkew: 1, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.DoNotSchedule},
			},
			override: []corev1.TopologySpreadConstraint{
				{MaxSkew: 2, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.DoNotSchedule},
				{MaxSkew: 3, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.ScheduleAnyway},
			},
			want: []corev1.TopologySpreadConstraint{
				{MaxSkew: 1, TopologyKey: corev1.LabelHostname, WhenUnsatisfiable: corev1.DoNotSchedule},
				{MaxSkew: 2, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.DoNotSchedule},
				{MaxSkew: 3, TopologyKey: corev1.LabelTopologyZone, WhenUnsatisfiable: corev1.ScheduleAnyway},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MergeTopologySpreadConstraints(tt.base, tt.override)
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MergeTopologySpreadConstraints() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}

func TestMergeAffinity(t *testing.T) {
	nodeAffinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{NodeSelectorTerms: []corev1.NodeSelectorTerm{{
			MatchExpressions: []corev1.NodeSelectorRequirement{{Key: "node", Operator: corev1.NodeSelectorOpIn, Values: []string{"base"}}},
		}}},
	}
	podAffinity := &corev1.PodAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: []corev1.PodAffinityTerm{{
			LabelSelector: &metav1.LabelSelector{MatchLabels: map[string]string{"app": "override"}},
			TopologyKey:   corev1.LabelHostname,
		}},
	}
	tests := []struct {
		name     string
		base     *corev1.Affinity
		override *corev1.Affinity
		want     *corev1.Affinity
	}{
		{
			name: "nil",
		},
		{
			name: "base only",
			base: &corev1.Affinity{NodeAffinity: nodeAffinity},
			want: &corev1.Affinity{NodeAffinity: nodeAffinity},
		},
		{
			name:     "override only",
			override: &corev1.Affinity{PodAffinity: podAffinity},
			want:     &corev1.Affinity{PodAffinity: podAffinity},
		},
		{
			name:     "merge fields",
			base:     &corev1.Affinity{NodeAffinity: nodeAffinity},
			override: &corev1.Affinity{PodAffinity: podAffinity},
			want:     &corev1.Affinity{NodeAffinity: nodeAffinity, PodAffinity: podAffinity},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := MergeAffinity(tt.base, tt.override)
			if err != nil {
				t.Fatalf("MergeAffinity() unexpected error: %v", err)
			}
			if diff := cmp.Diff(tt.want, got); diff != "" {
				t.Errorf("MergeAffinity() mismatch (-want +got):\n%s", diff)
			}
		})
	}
}
