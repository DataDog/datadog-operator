// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package comparison

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	datadoghqv1alpha1 "github.com/DataDog/extendeddaemonset/api/v1alpha1"
)

func TestGenerateHashFromEDSResourceNodeAnnotation(t *testing.T) {
	type args struct {
		edsNamespace    string
		edsName         string
		nodeAnnotations map[string]string
	}
	tests := []struct {
		name string
		args args
		want string
	}{
		{
			name: "no annotations",
			args: args{edsNamespace: "bar", edsName: "foo", nodeAnnotations: nil},
			want: "",
		},
		{
			name: "annotations present but not for this EDS",
			args: args{
				edsNamespace: "bar",
				edsName:      "foo",
				nodeAnnotations: map[string]string{
					"resources.extendeddaemonset.datadoghq.com/default.foo.daemons": "{\"limits\":{\"cpu\": \"1\"}}",
				},
			},
			want: "",
		},
		{
			name: "annotations present for this EDS",
			args: args{
				edsNamespace: "bar",
				edsName:      "foo",
				nodeAnnotations: map[string]string{
					"resources.extendeddaemonset.datadoghq.com/bar.foo.daemons": "{\"limits\":{\"cpu\": \"1\"}}",
				},
			},
			want: "bc9eacb89b7a44531492e87a37922dc3",
		},
		{
			name: "annotations present but not all for this EDS",
			args: args{
				edsNamespace: "bar",
				edsName:      "foo",
				nodeAnnotations: map[string]string{
					"resources.extendeddaemonset.datadoghq.com/default.foo.daemons": "{\"requests\":{\"cpu\": \"1\"}}",
					"resources.extendeddaemonset.datadoghq.com/bar.foo.daemons":     "{\"limits\":{\"cpu\": \"1\"}}",
				},
			},
			want: "bc9eacb89b7a44531492e87a37922dc3",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GenerateHashFromEDSResourceNodeAnnotation(tt.args.edsNamespace, tt.args.edsName, tt.args.nodeAnnotations); got != tt.want {
				t.Errorf("GenerateHashFromEDSResourceNodeAnnotation() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGenerateMD5PodTemplateSpec(t *testing.T) {
	ds := &datadoghqv1alpha1.ExtendedDaemonSet{}
	ds = datadoghqv1alpha1.DefaultExtendedDaemonSet(ds, datadoghqv1alpha1.ExtendedDaemonSetSpecStrategyCanaryValidationModeAuto)
	got, err := GenerateMD5PodTemplateSpec(&ds.Spec.Template)
	assert.Equal(t, "a2bb34618483323482d9a56ae2515eed", got)
	require.NoError(t, err)
}

func TestComparePodTemplateSpecMD5Hash(t *testing.T) {
	// no annotation
	hash := "somerandomhash"
	rs := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
	got := ComparePodTemplateSpecMD5Hash(hash, rs)
	assert.False(t, got)

	// non-matching annotation
	hash = "somerandomhash"
	rs = &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				string(datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey): "adifferenthash",
			},
		},
	}
	got = ComparePodTemplateSpecMD5Hash(hash, rs)
	assert.False(t, got)

	// matching annotation
	hash = "ahashthatmatches"
	rs = &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				string(datadoghqv1alpha1.MD5ExtendedDaemonSetAnnotationKey): "ahashthatmatches",
			},
		},
	}
	got = ComparePodTemplateSpecMD5Hash(hash, rs)
	assert.True(t, got)
}

func TestSetMD5PodTemplateSpecAnnotation(t *testing.T) {
	rs := &datadoghqv1alpha1.ExtendedDaemonSetReplicaSet{}
	ds := &datadoghqv1alpha1.ExtendedDaemonSet{}
	got, err := SetMD5PodTemplateSpecAnnotation(rs, ds)
	assert.Equal(t, "a2bb34618483323482d9a56ae2515eed", got)
	require.NoError(t, err)
}

func Test_StringsContains(t *testing.T) {
	tests := []struct {
		name string
		a    []string
		x    string
		want bool
	}{
		{
			name: "a contains x",
			a: []string{
				"hello",
				"goodbye",
			},
			x:    "hello",
			want: true,
		},
		{
			name: "a does not contain x",
			a: []string{
				"hello",
				"goodbye",
			},
			x:    "hi",
			want: false,
		},
		{
			name: "a does not contain x (but it is a substring of a string in a)",
			a: []string{
				"hello",
				"goodbye",
			},
			x:    "good",
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StringsContains(tt.a, tt.x)
			assert.Equal(t, tt.want, got)
		})
	}
}
