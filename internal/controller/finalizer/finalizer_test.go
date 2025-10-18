// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package finalizer

import (
	"context"
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
	"testing"
	"time"
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

	tests := []struct {
		name                  string
		clientObject          testResource
		finalizerShouldExists bool
		expectedResult        ctrl.Result
		expectedErr           bool
		deleterFunc           ResourceDeleteFunc
	}{
		{
			name: "check if object deletion timestamp is empty add finalizer if not exists",
			clientObject: testResource{
				TypeMeta: metav1.TypeMeta{
					Kind: "TestResource",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test resource",
				},
			},
			finalizerShouldExists: true,
			expectedResult:        ctrl.Result{},
			deleterFunc: func(ctx context.Context, k8sObj client.Object, datadogID string) error {
				return nil
			},
		},
		{
			name: "remove finalizer when deletion timestamp is set and finalizer is exits",
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
			expectedResult:        ctrl.Result{},
			deleterFunc: func(ctx context.Context, k8sObj client.Object, datadogID string) error {
				return nil
			},
		},
	}

	for _, tt := range tests {
		// arrange
		fakeClient := fake.NewClientBuilder().WithObjects(&tt.clientObject).Build()
		finalizer := NewFinalizer(testLogger, fakeClient, tt.deleterFunc, time.Minute, time.Minute)

		// act
		res, err := finalizer.HandleFinalizer(context.TODO(), &tt.clientObject, "123", finalizerName)

		// assert
		assert.NoError(t, err)
		assert.Equal(t, tt.expectedResult, res)
		if tt.finalizerShouldExists {
			assert.True(t, controllerutil.ContainsFinalizer(&tt.clientObject, finalizerName))
		} else {
			assert.False(t, controllerutil.ContainsFinalizer(&tt.clientObject, finalizerName))
		}
	}
}
