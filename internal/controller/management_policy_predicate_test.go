// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
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
		{name: "orphan annotation changed to other value", old: obj(1, orphan), new: obj(1, map[string]string{v1alpha1.ManagementPolicyAnnotationKey: "other"}), want: true},
		{name: "orphan annotation unchanged", old: obj(1, orphan), new: obj(1, orphan), want: false},
		{name: "unrelated annotation", old: obj(1, nil), new: obj(1, map[string]string{"foo": "bar"}), want: false},
		{name: "status-only change", old: obj(1, nil), new: obj(1, nil), want: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got := datadogResourcePredicate().Update(event.UpdateEvent{ObjectOld: tt.old, ObjectNew: tt.new})
			assert.Equal(t, tt.want, got)
		})
	}
	assert.True(t, datadogResourcePredicate().Create(event.CreateEvent{Object: obj(1, nil)}))
	assert.True(t, datadogResourcePredicate().Delete(event.DeleteEvent{Object: obj(1, orphan)}))
}

type registerSpyForwardersManager struct {
	datadog.MetricsForwardersManager
	registered int
}

func (s *registerSpyForwardersManager) Register(client.Object) {
	s.registered++
}

func TestMonitorMetricsForwarderPredicate(t *testing.T) {
	t.Run("registers forwarder for managed monitor", func(t *testing.T) {
		spy := &registerSpyForwardersManager{}
		pred := monitorMetricsForwarderPredicate(spy)
		mon := &v1alpha1.DatadogMonitor{ObjectMeta: metav1.ObjectMeta{Name: "m"}}
		assert.True(t, pred.Create(event.CreateEvent{Object: mon}))
		assert.Equal(t, 1, spy.registered)
	})

	t.Run("skips registration for orphaned monitor", func(t *testing.T) {
		spy := &registerSpyForwardersManager{}
		pred := monitorMetricsForwarderPredicate(spy)
		mon := &v1alpha1.DatadogMonitor{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "m",
				Annotations: map[string]string{v1alpha1.ManagementPolicyAnnotationKey: v1alpha1.ManagementPolicyOrphan},
			},
		}
		assert.True(t, pred.Create(event.CreateEvent{Object: mon}))
		assert.Equal(t, 0, spy.registered)
	})
}
