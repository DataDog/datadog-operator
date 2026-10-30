// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package common

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha2"
)

func getDPA(t *testing.T, c client.Client, namespace, name string) *v1alpha2.DatadogPodAutoscaler {
	t.Helper()
	dpa := &v1alpha2.DatadogPodAutoscaler{}
	require.NoError(t, c.Get(context.Background(), client.ObjectKey{Namespace: namespace, Name: name}, dpa))
	return dpa
}

func TestPatchAnnotation(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before map[string]string
		value  AnnotationValue
		want   map[string]string
	}{
		{
			name:   "set, no annotations",
			before: nil,
			value:  Set("true"),
			want:   map[string]string{PauseAnnotationKey: "true"},
		},
		{
			name:   "set keeps other annotations",
			before: map[string]string{"other": "x"},
			value:  Set("true"),
			want:   map[string]string{"other": "x", PauseAnnotationKey: "true"},
		},
		{
			name:   "set overwrites an existing value",
			before: map[string]string{PauseAnnotationKey: "yes"},
			value:  Set("true"),
			want:   map[string]string{PauseAnnotationKey: "true"},
		},
		{
			name:   "remove keeps other annotations",
			before: map[string]string{PauseAnnotationKey: "true", "other": "x"},
			value:  Removed,
			want:   map[string]string{"other": "x"},
		},
		{
			name:   "remove an absent key is a no-op",
			before: map[string]string{"other": "x"},
			value:  Removed,
			want:   map[string]string{"other": "x"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			initial := newDPA("ns", "a", tc.before, nil)
			initial.Spec.Owner = "Local"
			c := newFakeClient(t, initial)

			require.NoError(t, patchAnnotation(context.Background(), c, getDPA(t, c, "ns", "a"), PauseAnnotationKey, tc.value))

			got := getDPA(t, c, "ns", "a")
			assert.Equal(t, tc.want, got.Annotations)
			assert.Equal(t, initial.Spec.Owner, got.Spec.Owner, "the spec is never touched")
		})
	}
}

func TestPatchAnnotationNotFound(t *testing.T) {
	c := newFakeClient(t)
	err := patchAnnotation(context.Background(), c, newDPA("ns", "gone", nil, nil), PauseAnnotationKey, Set("true"))
	assert.Error(t, err)
}
