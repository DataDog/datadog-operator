// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
)

const (
	genericResourceKind = "DatadogGenericResource"
	testNamespace       = "foo"
)

var (
	testMgr, _ = ctrl.NewManager(&rest.Config{}, manager.Options{})
	testLogger = zap.New(zap.UseDevMode(true))
)

func Test_handleFinalizer(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	metaNow := metav1.NewTime(time.Now())

	r := &Reconciler{
		handlers: map[v1alpha1.SupportedResourcesType]ResourceHandler{
			v1alpha1.Notebook: &MockHandler{},
		},
		client: fake.NewClientBuilder().
			WithRuntimeObjects(
				&datadoghqv1alpha1.DatadogGenericResource{
					TypeMeta: metav1.TypeMeta{
						Kind: genericResourceKind,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "genericresource-create",
						Namespace: testNamespace,
					},
				},
				// Fake client preventes deletion timestamp from being set, so we init the store with an object that has:
				// - deletion timestamp (added by running kubectl delete)
				// - finalizer (added by the reconciler at creation time (see first test case))
				// Ref: https://github.com/kubernetes-sigs/controller-runtime/commit/7a66d580c0c53504f5b509b45e9300cc18a1cc30#diff-20ecedbf30721c01c33fb67d911da11c277e29990497a600d20cb0ec7215affdR683-R686
				&datadoghqv1alpha1.DatadogGenericResource{
					TypeMeta: metav1.TypeMeta{
						Kind: genericResourceKind,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "genericresource-delete",
						Namespace:         testNamespace,
						Finalizers:        []string{datadogGenericResourceFinalizer},
						DeletionTimestamp: &metaNow,
					},
					Spec: datadoghqv1alpha1.DatadogGenericResourceSpec{
						Type: v1alpha1.Notebook,
					},
				},
			).
			WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		scheme:   s,
		log:      testLogger,
		recorder: testMgr.GetEventRecorderFor(genericResourceKind),
	}

	tests := []struct {
		testName             string
		resourceName         string
		finalizerShouldExist bool
	}{
		{
			testName:             "a new DatadogGenericResource object gets a finalizer added successfully",
			resourceName:         "genericresource-create",
			finalizerShouldExist: true,
		},
		{
			testName:             "a DatadogGenericResource object (with the finalizer) has a deletion timestamp",
			resourceName:         "genericresource-delete",
			finalizerShouldExist: false,
		},
	}
	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			ctx := ctrl.LoggerInto(context.TODO(), testLogger.WithValues("test:", test.testName))
			reqLogger := ctrl.LoggerFrom(ctx)
			testGcr := &datadoghqv1alpha1.DatadogGenericResource{}
			err := r.client.Get(ctx, client.ObjectKey{Name: test.resourceName, Namespace: testNamespace}, testGcr)

			final := finalizer.NewFinalizer(reqLogger, r.client, r.deleteResource(reqLogger), defaultRequeuePeriod, defaultErrRequeuePeriod)
			_, err = final.HandleFinalizer(context.TODO(), testGcr, testGcr.Status.Id, datadogGenericResourceFinalizer)

			assert.NoError(t, err)
			if test.finalizerShouldExist {
				assert.True(t, controllerutil.ContainsFinalizer(testGcr, datadogGenericResourceFinalizer))
			} else {
				assert.False(t, controllerutil.ContainsFinalizer(testGcr, datadogGenericResourceFinalizer))
			}
		})
	}

}

// Test_handleFinalizer_deleteErrorPropagates verifies that when the handler's
// deleteResource returns an error (e.g. Datadog API rejected the delete with
// 400 because the monitor is referenced by a composite), the shared Finalizer
// propagates the error and keeps the finalizer in place. Before the CONS-8253
// fix, this error was swallowed and the finalizer was removed, silently
// orphaning the remote Datadog resource.
func Test_handleFinalizer_deleteErrorPropagates(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	metaNow := metav1.NewTime(time.Now())

	defer resetMockHandlerState()
	mockDeleteErr = errors.New("error deleting monitor: 400 Bad Request: monitor [12345] is referenced in composite monitors: [67890]")

	obj := &datadoghqv1alpha1.DatadogGenericResource{
		TypeMeta: metav1.TypeMeta{
			Kind: genericResourceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "genericresource-delete-blocked",
			Namespace:         testNamespace,
			Finalizers:        []string{datadogGenericResourceFinalizer},
			DeletionTimestamp: &metaNow,
		},
		Spec: datadoghqv1alpha1.DatadogGenericResourceSpec{
			Type: v1alpha1.Notebook,
		},
		Status: datadoghqv1alpha1.DatadogGenericResourceStatus{
			Id: "12345",
		},
	}

	r := &Reconciler{
		handlers: map[v1alpha1.SupportedResourcesType]ResourceHandler{
			v1alpha1.Notebook: &MockHandler{},
		},
		client: fake.NewClientBuilder().
			WithRuntimeObjects(obj).
			WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		scheme:   s,
		log:      testLogger,
		recorder: testMgr.GetEventRecorderFor(genericResourceKind),
	}

	ctx := ctrl.LoggerInto(context.TODO(), testLogger)
	reqLogger := ctrl.LoggerFrom(ctx)
	testGcr := &datadoghqv1alpha1.DatadogGenericResource{}
	getErr := r.client.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: testNamespace}, testGcr)
	assert.NoError(t, getErr)

	final := finalizer.NewFinalizer(reqLogger, r.client, r.deleteResource(reqLogger), defaultRequeuePeriod, defaultErrRequeuePeriod)
	_, err := final.HandleFinalizer(context.TODO(), testGcr, testGcr.Status.Id, datadogGenericResourceFinalizer)

	// The error must propagate up from the handler so the shared Finalizer
	// skips RemoveFinalizer and requeues.
	assert.Error(t, err)
	// And the finalizer must still be present so the object stays in
	// Terminating state until the underlying issue is resolved.
	assert.True(t, controllerutil.ContainsFinalizer(testGcr, datadogGenericResourceFinalizer))
}

func Test_handleFinalizer_deleteWithoutStatusIDRemovesFinalizer(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	metaNow := metav1.NewTime(time.Now())

	defer resetMockHandlerState()
	mockDeleteErr = errors.New("delete should not be called when status.id is empty")

	obj := &datadoghqv1alpha1.DatadogGenericResource{
		TypeMeta: metav1.TypeMeta{
			Kind: genericResourceKind,
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "genericresource-delete-without-status-id",
			Namespace:         testNamespace,
			Finalizers:        []string{datadogGenericResourceFinalizer},
			DeletionTimestamp: &metaNow,
		},
		Spec: datadoghqv1alpha1.DatadogGenericResourceSpec{
			Type: v1alpha1.Notebook,
		},
	}

	r := &Reconciler{
		handlers: map[v1alpha1.SupportedResourcesType]ResourceHandler{
			v1alpha1.Notebook: &MockHandler{},
		},
		client: fake.NewClientBuilder().
			WithRuntimeObjects(obj).
			WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		scheme:   s,
		log:      testLogger,
		recorder: testMgr.GetEventRecorderFor(genericResourceKind),
	}

	ctx := ctrl.LoggerInto(context.TODO(), testLogger)
	reqLogger := ctrl.LoggerFrom(ctx)
	testGcr := &datadoghqv1alpha1.DatadogGenericResource{}
	getErr := r.client.Get(ctx, client.ObjectKey{Name: obj.Name, Namespace: testNamespace}, testGcr)
	assert.NoError(t, getErr)

	final := finalizer.NewFinalizer(reqLogger, r.client, r.deleteResource(reqLogger), defaultRequeuePeriod, defaultErrRequeuePeriod)
	result, err := final.HandleFinalizer(context.TODO(), testGcr, testGcr.Status.Id, datadogGenericResourceFinalizer)

	// Finalization should succeed because an empty status ID means there is no remote Datadog object to delete.
	assert.NoError(t, err)
	// Once the finalizer is cleared, reconciliation falls back to the normal post-delete requeue cadence.
	assert.Equal(t, ctrl.Result{RequeueAfter: defaultRequeuePeriod}, result)
	// The handler-level delete must be skipped entirely when there is no Datadog ID.
	assert.Equal(t, 0, mockDeleteCalls)
	// Clearing the finalizer allows Kubernetes garbage collection to complete the deletion.
	assert.False(t, controllerutil.ContainsFinalizer(testGcr, datadogGenericResourceFinalizer))
}
