// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

func TestDatadogResourcePredicate(t *testing.T) {
	orphan := map[string]string{v1alpha1.ManagementPolicyAnnotationKey: v1alpha1.ManagementPolicyOrphan}
	obj := func(generation int64, annotations map[string]string) *v1alpha1.DatadogGenericResource {
		return &v1alpha1.DatadogGenericResource{ObjectMeta: metav1.ObjectMeta{Generation: generation, Annotations: annotations}}
	}
	for _, tt := range []struct {
		name     string
		old, new *v1alpha1.DatadogGenericResource
		want     bool
	}{
		{name: "spec change", old: obj(1, nil), new: obj(2, nil), want: true},
		{name: "orphan annotation added", old: obj(1, nil), new: obj(1, orphan), want: true},
		{name: "orphan annotation removed", old: obj(1, orphan), new: obj(1, nil), want: true},
		{name: "orphan annotation unchanged", old: obj(1, orphan), new: obj(1, orphan), want: false},
		{name: "unrelated annotation", old: obj(1, nil), new: obj(1, map[string]string{"foo": "bar"}), want: false},
		{name: "status-only change", old: obj(1, nil), new: obj(1, nil), want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := datadogResourcePredicate().Update(event.UpdateEvent{ObjectOld: tt.old, ObjectNew: tt.new})
			assert.Equal(t, tt.want, got)
		})
	}
}
