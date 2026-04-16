// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package finalizer

import (
	"context"
	"fmt"
	"testing"
	"time"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

type testResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`
}

func (t testResource) DeepCopyObject() runtime.Object {
	return &t
}

func Test_HandleFinalizer(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &testResource{})
	finalizerName := "test_resource.finalizer"
	metaNow := metav1.NewTime(time.Now())
	requeuePeriod := time.Minute
	errRequeuePeriod := time.Minute

	noopDelete := func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		return nil
	}
	failDelete := func(ctx context.Context, k8sObj client.Object, datadogID string) error {
		return fmt.Errorf("API error")
	}

	tests := []struct {
		name                  string
		clientObject          testResource
		finalizerShouldExists bool
		expectedResult        ctrl.Result
		expectedErr           bool
		deleterFunc           ResourceDeleteFunc
		requeuePeriodOverride *time.Duration // if set, overrides requeuePeriod
	}{
		{
			name: "not deleting, no finalizer: adds finalizer and requeues with period",
			clientObject: testResource{
				TypeMeta: metav1.TypeMeta{
					Kind: "TestResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test resource",
				},
			},
			finalizerShouldExists: true,
			expectedResult:        ctrl.Result{RequeueAfter: requeuePeriod},
			deleterFunc:           noopDelete,
		},
		{
			name: "not deleting, no finalizer, zero period: adds finalizer and requeues immediately",
			clientObject: testResource{
				TypeMeta: metav1.TypeMeta{
					Kind: "TestResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test resource 2",
				},
			},
			finalizerShouldExists: true,
			expectedResult:        ctrl.Result{Requeue: true},
			deleterFunc:           noopDelete,
			requeuePeriodOverride: durationPtr(0),
		},
		{
			name: "not deleting, already has finalizer: no-op, proceed to reconciliation",
			clientObject: testResource{
				TypeMeta: metav1.TypeMeta{
					Kind: "TestResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:       "test resource",
					Finalizers: []string{finalizerName},
				},
			},
			finalizerShouldExists: true,
			expectedResult:        ctrl.Result{},
			deleterFunc:           noopDelete,
		},
		{
			name: "deleting, has finalizer, deleteFunc succeeds: removes finalizer, requeues with period",
			clientObject: testResource{
				TypeMeta: metav1.TypeMeta{
					Kind: "TestResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test resource",
					DeletionTimestamp: &metaNow,
					Finalizers:        []string{finalizerName},
				},
			},
			finalizerShouldExists: false,
			expectedResult:        ctrl.Result{Requeue: true, RequeueAfter: requeuePeriod},
			deleterFunc:           noopDelete,
		},
		{
			name: "deleting, has finalizer, deleteFunc fails: keeps finalizer, requeues with error period",
			clientObject: testResource{
				TypeMeta: metav1.TypeMeta{
					Kind: "TestResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test resource",
					DeletionTimestamp: &metaNow,
					Finalizers:        []string{finalizerName},
				},
			},
			finalizerShouldExists: true,
			expectedResult:        ctrl.Result{Requeue: true, RequeueAfter: errRequeuePeriod},
			expectedErr:           true,
			deleterFunc:           failDelete,
		},
		// Note: "deleting, no finalizer" is not testable because the fake client
		// (and the real API server) reject objects with a deletionTimestamp but no
		// finalizers — Kubernetes would have already garbage-collected such objects.
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().WithObjects(&tt.clientObject).Build()
			rp := requeuePeriod
			if tt.requeuePeriodOverride != nil {
				rp = *tt.requeuePeriodOverride
			}
			finalizer := NewFinalizer(testLogger, fakeClient, tt.deleterFunc, rp, errRequeuePeriod)

			res, err := finalizer.HandleFinalizer(context.TODO(), &tt.clientObject, "123", finalizerName)

			if tt.expectedErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.expectedResult, res)
			if tt.finalizerShouldExists {
				assert.True(t, controllerutil.ContainsFinalizer(&tt.clientObject, finalizerName))
			} else {
				assert.False(t, controllerutil.ContainsFinalizer(&tt.clientObject, finalizerName))
			}
		})
	}
}

func durationPtr(d time.Duration) *time.Duration {
	return &d
}
