// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

func TestReconcileDatadogMonitor_OrphanPolicy(t *testing.T) {
	r, api := newForceSyncTestReconciler(t)
	dm := genericDatadogMonitor(r.client)
	key := client.ObjectKeyFromObject(dm)
	get := func() *datadoghqv1alpha1.DatadogMonitor {
		t.Helper()
		got := &datadoghqv1alpha1.DatadogMonitor{}
		require.NoError(t, r.client.Get(context.TODO(), key, got))
		return got
	}
	apiCalls := func() int32 {
		api.mu.Lock()
		defer api.mu.Unlock()
		return api.puts.Load() + api.validates.Load() + api.deletes.Load() + int32(api.reads)
	}
	orphaned := func(dm *datadoghqv1alpha1.DatadogMonitor) *datadoghqv1alpha1.DatadogMonitorCondition {
		for i := range dm.Status.Conditions {
			if dm.Status.Conditions[i].Type == datadoghqv1alpha1.DatadogMonitorConditionTypeOrphaned {
				return &dm.Status.Conditions[i]
			}
		}
		return nil
	}

	// Add the finalizer, add the required tags, then create the monitor.
	for range 3 {
		_, err := r.Reconcile(context.TODO(), get())
		require.NoError(t, err)
	}
	require.NotZero(t, get().Status.ID)
	require.True(t, get().Status.Primary)

	// Orphan the CR and change its spec: nothing is sent to Datadog.
	dm = get()
	dm.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	dm.Spec.Message = "changed while orphaned"
	require.NoError(t, r.client.Update(context.TODO(), dm))
	before := apiCalls()
	result, err := r.Reconcile(context.TODO(), get())
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result)
	assert.Equal(t, before, apiCalls(), "no Datadog API call while orphaned")
	cond := orphaned(get())
	require.NotNil(t, cond)
	assert.Equal(t, corev1.ConditionTrue, cond.Status)
	assert.Equal(t, condition.OrphanedReason, cond.Reason)

	// Removing the annotation resumes reconciliation and applies the spec.
	dm = get()
	dm.Annotations = nil
	require.NoError(t, r.client.Update(context.TODO(), dm))
	putsBefore := api.puts.Load()
	_, err = r.Reconcile(context.TODO(), get())
	require.NoError(t, err)
	assert.Equal(t, putsBefore+1, api.puts.Load())
	assert.Nil(t, orphaned(get()))

	// Deleting an orphaned CR keeps the monitor, even though the CR created it.
	dm = get()
	dm.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	require.NoError(t, r.client.Update(context.TODO(), dm))
	require.NoError(t, r.client.Delete(context.TODO(), get()))
	_, err = r.Reconcile(context.TODO(), get())
	require.NoError(t, err)
	assert.Equal(t, int32(0), api.deletes.Load())
	assert.True(t, apierrors.IsNotFound(r.client.Get(context.TODO(), key, &datadoghqv1alpha1.DatadogMonitor{})))
}

func TestReconcileDatadogMonitor_OrphanWithoutCredentials(t *testing.T) {
	r, api := newForceSyncTestReconciler(t)
	dm := genericDatadogMonitor(r.client)
	key := client.ObjectKeyFromObject(dm)
	require.NoError(t, r.client.Get(context.TODO(), key, dm))
	dm.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	dm.Finalizers = []string{datadogMonitorFinalizer}
	require.NoError(t, r.client.Update(context.TODO(), dm))
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_APP_KEY", "")

	require.NoError(t, r.client.Get(context.TODO(), key, dm))
	_, err := r.Reconcile(context.TODO(), dm)
	require.NoError(t, err)
	require.NoError(t, r.client.Get(context.TODO(), key, dm))
	require.NotEmpty(t, dm.Status.Conditions, "the Orphaned condition is set without credentials")

	require.NoError(t, r.client.Delete(context.TODO(), dm))
	require.NoError(t, r.client.Get(context.TODO(), key, dm))
	_, err = r.Reconcile(context.TODO(), dm)
	require.NoError(t, err)
	assert.True(t, apierrors.IsNotFound(r.client.Get(context.TODO(), key, &datadoghqv1alpha1.DatadogMonitor{})))
	assert.Equal(t, int32(0), api.deletes.Load())
}

func TestReconcileDatadogMonitor_CreateSkippedWhenOrphanedInAPIServer(t *testing.T) {
	r, api := newForceSyncTestReconciler(t)
	dm := genericDatadogMonitor(r.client)
	key := client.ObjectKeyFromObject(dm)
	// Add the finalizer and the required tags, stopping before the create.
	for range 2 {
		require.NoError(t, r.client.Get(context.TODO(), key, dm))
		_, err := r.Reconcile(context.TODO(), dm)
		require.NoError(t, err)
	}
	require.NoError(t, r.client.Get(context.TODO(), key, dm))
	require.Zero(t, dm.Status.ID)

	// The API server has the annotation, but the reconcile works from a cache
	// entry that doesn't have it yet.
	stale := dm.DeepCopy()
	dm.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	require.NoError(t, r.client.Update(context.TODO(), dm))
	stale.ResourceVersion = dm.ResourceVersion
	result, err := r.Reconcile(context.TODO(), stale)
	require.NoError(t, err)
	assert.Equal(t, defaultErrRequeuePeriod, result.RequeueAfter)
	api.mu.Lock()
	defer api.mu.Unlock()
	assert.Nil(t, api.monitor, "no monitor is created")
}
