// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package equality

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestIsEqualOperatorAnnotations(t *testing.T) {
	tests := []struct {
		name string
		objA metav1.Object
		objB metav1.Object
		want bool
	}{
		{
			name: "obj annotations equal",
			objA: &metav1.ObjectMeta{
				Name: "foo",
				Annotations: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			objB: &metav1.ObjectMeta{
				Name: "foo2",
				Annotations: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			want: true,
		},
		{
			name: "objs not equal",
			objA: &metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			objB: &metav1.ObjectMeta{
				Annotations: map[string]string{
					"foo":  "bar",
					"one2": "two",
				},
			},
			want: false,
		},
		{
			name: "objs not equal, but annotations equal",
			objA: &metav1.ObjectMeta{
				Name: "foo",
				Labels: map[string]string{
					"foo": "bar",
					"one": "two",
				},
				Annotations: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			objB: &metav1.ObjectMeta{
				Name: "foo2",
				Labels: map[string]string{
					"foo2": "bar",
					"one2": "two",
				},
				Annotations: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsEqualOperatorAnnotations(tt.objA, tt.objB))
		})
	}
}
func TestIsEqualOperatorLabels(t *testing.T) {
	tests := []struct {
		name string
		objA metav1.Object
		objB metav1.Object
		want bool
	}{
		{
			name: "obj labels equal",
			objA: &metav1.ObjectMeta{
				Name: "foo",
				Labels: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			objB: &metav1.ObjectMeta{
				Name: "foo2",
				Labels: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			want: true,
		},
		{
			name: "objs not equal",
			objA: &metav1.ObjectMeta{
				Labels: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			objB: &metav1.ObjectMeta{
				Labels: map[string]string{
					"foo":  "bar",
					"one2": "two",
				},
			},
			want: false,
		},
		{
			name: "objs not equal, but labels equal",
			objA: &metav1.ObjectMeta{
				Name: "foo",
				Annotations: map[string]string{
					"foo": "bar",
					"one": "two",
				},
				Labels: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			objB: &metav1.ObjectMeta{
				Name: "foo2",
				Annotations: map[string]string{
					"foo2": "bar",
					"one2": "two",
				},
				Labels: map[string]string{
					"foo": "bar",
					"one": "two",
				},
			},
			want: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsEqualOperatorLabels(tt.objA, tt.objB))
		})
	}
}
func TestIsEqualSecrets(t *testing.T) {
	tests := []struct {
		name string
		objA client.Object
		objB client.Object
		want bool
	}{
		{
			name: "objs equal",
			objA: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
				Data: map[string][]byte{
					"foo": {1, 2, 3},
				},
			},
			objB: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
				Data: map[string][]byte{
					"foo": {1, 2, 3},
				},
			},
			want: true,
		},
		{
			name: "objs not equal",
			objA: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
				Data: map[string][]byte{
					"foo": {1, 2, 3},
				},
			},
			objB: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
				Data: map[string][]byte{
					"foo": {3, 2, 1},
				},
			},
			want: false,
		},
		{
			name: "objs not equal, but data and stringdata equal",
			objA: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
				Type: corev1.SecretTypeBasicAuth,
				Data: map[string][]byte{
					"foo": {1, 2, 3},
				},
			},
			objB: &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
				Type: corev1.SecretTypeBootstrapToken,
				Data: map[string][]byte{
					"foo": {1, 2, 3},
				},
			},
			want: true,
		},
		{
			name: "objs not secrets",
			objA: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
			},
			objB: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name: "secret-foo",
				},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsEqualSecrets(tt.objA, tt.objB))
		})
	}
}
