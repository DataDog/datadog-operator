// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

func newMockReconciler(t *testing.T) (*Reconciler, *record.FakeRecorder) {
	t.Helper()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	recorder := record.NewFakeRecorder(100)
	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logf.Log.WithName(t.Name()),
		recorder,
	)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{mockSubresource: &MockHandler{}}
	return r, recorder
}

func setOrphanAnnotation(t *testing.T, r *Reconciler, orphan bool) {
	t.Helper()
	obj := getGenericResource(t, r)
	if orphan {
		obj.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	} else {
		delete(obj.Annotations, datadoghqv1alpha1.ManagementPolicyAnnotationKey)
	}
	require.NoError(t, r.client.Update(context.TODO(), obj))
}

func mockCalls() int {
	return mockCreateCalls + mockGetCalls + mockUpdateCalls + mockDeleteCalls + mockRefreshStateCalls
}

func drainEvents(recorder *record.FakeRecorder) []string {
	var events []string
	for {
		select {
		case e := <-recorder.Events:
			events = append(events, e)
		default:
			return events
		}
	}
}

func TestReconcileGenericResource_OrphanPolicy(t *testing.T) {
	resetMockHandlerState()
	r, recorder := newMockReconciler(t)
	ctx := context.TODO()
	req := newRequest(resourcesNamespace, resourcesName)
	require.NoError(t, r.client.Create(ctx, mockGenericResource()))

	// Add the finalizer, then create the Datadog resource.
	_, err := reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	_, err = reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	require.Equal(t, 1, mockCreateCalls)
	require.Equal(t, mockResourceID, getGenericResource(t, r).Status.Id)

	// Orphan the CR, change its spec and make a force sync and a state refresh due.
	setOrphanAnnotation(t, r, true)
	obj := getGenericResource(t, r)
	obj.Spec.JsonSpec = `{"foo": "baz"}`
	require.NoError(t, r.client.Update(ctx, obj))
	obj.Status.LastForceSyncTime = nil
	obj.Status.StateLastUpdateTime = nil
	require.NoError(t, r.client.Status().Update(ctx, obj))
	statusBefore := getGenericResource(t, r).Status

	callsBefore := mockCalls()
	result, err := reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	assert.Equal(t, reconcile.Result{}, result, "an orphaned CR is not requeued")
	assert.Equal(t, callsBefore, mockCalls(), "no Datadog API call while orphaned")
	obj = getGenericResource(t, r)
	assert.True(t, meta.IsStatusConditionTrue(obj.Status.Conditions, string(condition.DatadogConditionTypeOrphaned)))
	assert.Equal(t, statusBefore.Id, obj.Status.Id)
	assert.Equal(t, statusBefore.CurrentHash, obj.Status.CurrentHash, "other status fields keep their last values")

	// Reconciling again doesn't call the API either.
	_, err = reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	assert.Equal(t, callsBefore, mockCalls())

	// Removing the annotation resumes reconciliation: the changed spec is applied.
	setOrphanAnnotation(t, r, false)
	_, err = reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 1, mockUpdateCalls)
	obj = getGenericResource(t, r)
	assert.Nil(t, meta.FindStatusCondition(obj.Status.Conditions, string(condition.DatadogConditionTypeOrphaned)))

	// Deleting an orphaned CR keeps the Datadog object.
	setOrphanAnnotation(t, r, true)
	drainEvents(recorder)
	require.NoError(t, r.client.Delete(ctx, getGenericResource(t, r)))
	_, err = reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 0, mockDeleteCalls)
	err = r.client.Get(ctx, req.NamespacedName, &datadoghqv1alpha1.DatadogGenericResource{})
	assert.True(t, apierrors.IsNotFound(err), "the CR is gone once the finalizer is removed")
	events := drainEvents(recorder)
	require.Len(t, events, 1)
	assert.Contains(t, events[0], finalizer.OrphanedEventReason)
}

func TestReconcileGenericResource_OrphanReleasesStuckDeletion(t *testing.T) {
	resetMockHandlerState()
	t.Cleanup(resetMockHandlerState)
	r, _ := newMockReconciler(t)
	ctx := context.TODO()
	req := newRequest(resourcesNamespace, resourcesName)
	require.NoError(t, r.client.Create(ctx, mockGenericResource()))
	for range 2 {
		_, err := reconcileRequest(r, ctx, req)
		require.NoError(t, err)
	}

	// The remote delete keeps failing, so the CR stays terminating.
	mockDeleteErr = assert.AnError
	require.NoError(t, r.client.Delete(ctx, getGenericResource(t, r)))
	_, err := reconcileRequest(r, ctx, req)
	require.Error(t, err)
	require.Equal(t, 1, mockDeleteCalls)
	require.NotNil(t, getGenericResource(t, r).DeletionTimestamp)

	// Annotations can still be edited on a terminating CR: orphan releases it.
	setOrphanAnnotation(t, r, true)
	_, err = reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	assert.Equal(t, 1, mockDeleteCalls, "no further delete attempt")
	err = r.client.Get(ctx, req.NamespacedName, &datadoghqv1alpha1.DatadogGenericResource{})
	assert.True(t, apierrors.IsNotFound(err))
}

func TestReconcileGenericResource_OrphanWithoutCredentials(t *testing.T) {
	resetMockHandlerState()
	r, _ := newMockReconciler(t)
	ctx := context.TODO()
	req := newRequest(resourcesNamespace, resourcesName)
	obj := mockGenericResource()
	obj.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	// A type without a handler: the orphan path must not need one either.
	obj.Spec.Type = "removed_type"
	obj.Status.Id = "123"
	require.NoError(t, r.client.Create(ctx, obj))
	t.Setenv("DD_API_KEY", "")
	t.Setenv("DD_APP_KEY", "")

	// Not deleting: the finalizer is added, then the Orphaned condition is set.
	for range 2 {
		_, err := reconcileRequest(r, ctx, req)
		require.NoError(t, err)
	}
	assert.True(t, meta.IsStatusConditionTrue(getGenericResource(t, r).Status.Conditions, string(condition.DatadogConditionTypeOrphaned)))

	// Deleting: released without credentials, the object is kept.
	require.NoError(t, r.client.Delete(ctx, getGenericResource(t, r)))
	_, err := reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	assert.True(t, apierrors.IsNotFound(r.client.Get(ctx, req.NamespacedName, &datadoghqv1alpha1.DatadogGenericResource{})))
	assert.Equal(t, 0, mockCalls())
}

func TestReconcileGenericResource_CreateSkippedWhenOrphanedInAPIServer(t *testing.T) {
	resetMockHandlerState()
	r, _ := newMockReconciler(t)
	ctx := context.TODO()
	req := newRequest(resourcesNamespace, resourcesName)
	require.NoError(t, r.client.Create(ctx, mockGenericResource()))

	// Add the finalizer, stopping before the create.
	_, err := reconcileRequest(r, ctx, req)
	require.NoError(t, err)
	obj := getGenericResource(t, r)
	require.Empty(t, obj.Status.Id)

	// The API server has the annotation, but the reconcile works from a cache
	// entry that doesn't have it yet.
	stale := obj.DeepCopy()
	obj.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	require.NoError(t, r.client.Update(ctx, obj))
	stale.ResourceVersion = obj.ResourceVersion

	result, err := r.Reconcile(ctx, stale)
	require.NoError(t, err)
	assert.Equal(t, defaultErrRequeuePeriod, result.RequeueAfter)
	assert.Equal(t, 0, mockCreateCalls, "no resource is created")
}

func TestReconcileGenericResource_OrphanStatusConflictRequeuesWithoutClobbering(t *testing.T) {
	resetMockHandlerState()
	r, _ := newMockReconciler(t)
	ctx := context.TODO()
	obj := mockGenericResource()
	obj.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	obj.Finalizers = []string{datadogGenericResourceFinalizer}
	require.NoError(t, r.client.Create(ctx, obj))

	// Capture a stale copy of the orphaned CR before a concurrent update removes
	// the annotation and persists a newer status (for example an in-flight create).
	stale := getGenericResource(t, r)
	latest := stale.DeepCopy()
	latest.Annotations = nil
	require.NoError(t, r.client.Update(ctx, latest))
	latest = getGenericResource(t, r)
	latest.Status.Id = "persisted-id"
	latest.Status.CurrentHash = "newer-hash"
	require.NoError(t, r.client.Status().Update(ctx, latest))

	result, err := r.Reconcile(ctx, stale)
	require.NoError(t, err)
	assert.Equal(t, defaultErrRequeuePeriod, result.RequeueAfter)

	after := getGenericResource(t, r)
	assert.Equal(t, "persisted-id", after.Status.Id, "newer status ID must not be clobbered")
	assert.Equal(t, "newer-hash", after.Status.CurrentHash, "newer status hash must not be clobbered")
	assert.Nil(t, meta.FindStatusCondition(after.Status.Conditions, string(condition.DatadogConditionTypeOrphaned)), "stale Orphaned condition must not overwrite newer status")
}
