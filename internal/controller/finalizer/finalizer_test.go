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
	"k8s.io/client-go/tools/record"
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
	defaultRequeuePeriod := 30 * time.Second
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
	}{
		{
			name: "not deleting, no finalizer: adds finalizer and requeues immediately",
			clientObject: testResource{
				TypeMeta: metav1.TypeMeta{
					Kind: "TestResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test resource",
				},
			},
			finalizerShouldExists: true,
			expectedResult:        ctrl.Result{Requeue: true},
			deleterFunc:           noopDelete,
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
			name: "deleting, has finalizer, deleteFunc succeeds: removes finalizer, requeues with defaultRequeuePeriod",
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
			expectedResult:        ctrl.Result{RequeueAfter: defaultRequeuePeriod},
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
			finalizer := NewFinalizer(testLogger, fakeClient, tt.deleterFunc, defaultRequeuePeriod, errRequeuePeriod)

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

func Test_HandleFinalizer_OrphanPolicy(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &testResource{})
	finalizerName := "test_resource.finalizer"
	metaNow := metav1.NewTime(time.Now())
	defaultRequeuePeriod := 30 * time.Second
	errRequeuePeriod := time.Minute
	orphanAnnotation := map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}

	tests := []struct {
		name              string
		annotations       map[string]string
		deleting          bool
		withPolicy        bool
		deleteErr         error
		wantDeleteCalled  bool
		wantOnOrphan      bool
		wantFinalizer     bool
		wantEvent         bool
		wantErr           bool
		wantRequeueAfter  time.Duration
		wantRequeueAtOnce bool
	}{
		{
			name:             "orphan policy, annotated, deleting: keeps the Datadog object and removes the finalizer",
			annotations:      orphanAnnotation,
			deleting:         true,
			withPolicy:       true,
			wantOnOrphan:     true,
			wantEvent:        true,
			wantRequeueAfter: defaultRequeuePeriod,
		},
		{
			name:             "orphan policy, annotated, deleting with a failing delete: releases the finalizer",
			annotations:      orphanAnnotation,
			deleting:         true,
			withPolicy:       true,
			deleteErr:        fmt.Errorf("API error"),
			wantOnOrphan:     true,
			wantEvent:        true,
			wantRequeueAfter: defaultRequeuePeriod,
		},
		{
			name:             "orphan policy, not annotated, deleting: deletes the Datadog object",
			deleting:         true,
			withPolicy:       true,
			wantDeleteCalled: true,
			wantRequeueAfter: defaultRequeuePeriod,
		},
		{
			name:             "orphan policy, unknown annotation value: deletes the Datadog object",
			annotations:      map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: "keep"},
			deleting:         true,
			withPolicy:       true,
			wantDeleteCalled: true,
			wantRequeueAfter: defaultRequeuePeriod,
		},
		{
			name:             "no orphan policy, annotated, deleting: the annotation is ignored",
			annotations:      orphanAnnotation,
			deleting:         true,
			withPolicy:       false,
			wantDeleteCalled: true,
			wantRequeueAfter: defaultRequeuePeriod,
		},
		{
			name:              "orphan policy, annotated, not deleting: adds the finalizer",
			annotations:       orphanAnnotation,
			withPolicy:        true,
			wantFinalizer:     true,
			wantRequeueAtOnce: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := testResource{
				TypeMeta: metav1.TypeMeta{Kind: "TestResource"},
				ObjectMeta: metav1.ObjectMeta{
					Name:        "test resource",
					Annotations: tt.annotations,
				},
			}
			if tt.deleting {
				obj.DeletionTimestamp = &metaNow
				obj.Finalizers = []string{finalizerName}
			}
			fakeClient := fake.NewClientBuilder().WithObjects(&obj).Build()

			deleteCalled := false
			deleteFunc := func(context.Context, client.Object, string) error {
				deleteCalled = true
				return tt.deleteErr
			}
			onOrphanCalled := false
			recorder := record.NewFakeRecorder(10)
			var opts []Option
			if tt.withPolicy {
				opts = append(opts, WithOrphanPolicy(recorder, func(client.Object) { onOrphanCalled = true }))
			}
			f := NewFinalizer(testLogger, fakeClient, deleteFunc, defaultRequeuePeriod, errRequeuePeriod, opts...)

			res, err := f.HandleFinalizer(context.TODO(), &obj, "123", finalizerName)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.wantDeleteCalled, deleteCalled, "deleteFunc called")
			assert.Equal(t, tt.wantOnOrphan, onOrphanCalled, "onOrphan called")
			assert.Equal(t, tt.wantFinalizer, controllerutil.ContainsFinalizer(&obj, finalizerName))
			if tt.wantRequeueAtOnce {
				assert.Equal(t, ctrl.Result{Requeue: true}, res)
			} else {
				assert.Equal(t, ctrl.Result{RequeueAfter: tt.wantRequeueAfter}, res)
			}
			if tt.wantEvent {
				assert.Len(t, recorder.Events, 1)
				assert.Contains(t, <-recorder.Events, OrphanedEventReason)
			} else {
				assert.Empty(t, recorder.Events)
			}
		})
	}
}
