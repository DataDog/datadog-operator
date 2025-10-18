// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
)

var (
	testLogger logr.Logger = zap.New(zap.UseDevMode(true))
)

func Test_handleFinalizer(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	metaNow := metav1.NewTime(time.Now())

	r := &Reconciler{
		client: fake.NewClientBuilder().
			WithRuntimeObjects(
				&datadoghqv1alpha1.DatadogMonitor{
					TypeMeta: metav1.TypeMeta{
						Kind: "DatadogMonitor",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "monitor-create",
						Namespace: "foo",
					},
				},
				// Fake client preventes deletion timestamp from being set, so we init the store with an object that has:
				// - deletion timestamp (added by running kubectl delete)
				// - finalizer (added by the reconciler at creation time (see first test case))
				// Ref: https://github.com/kubernetes-sigs/controller-runtime/commit/7a66d580c0c53504f5b509b45e9300cc18a1cc30#diff-20ecedbf30721c01c33fb67d911da11c277e29990497a600d20cb0ec7215affdR683-R686
				&datadoghqv1alpha1.DatadogMonitor{
					TypeMeta: metav1.TypeMeta{
						Kind: "DatadogMonitor",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "monitor-to-delete",
						Namespace:         "foo",
						DeletionTimestamp: &metaNow,
						Finalizers:        []string{datadogMonitorFinalizer},
					},
					Status: datadoghqv1alpha1.DatadogMonitorStatus{
						Primary: false,
					},
				},
			).
			WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
		scheme: s,
		log:    testLogger,
	}

	testCases := []struct {
		name                 string
		objectName           string
		finalizerShouldExist bool
	}{
		{
			name:                 "a new DatadogMonitor object gets a finalizer added successfully",
			objectName:           "monitor-create",
			finalizerShouldExist: true,
		},
		{
			name:                 "a DatadogMonitor object (with the finalizer) has a deletion timestamp",
			objectName:           "monitor-to-delete",
			finalizerShouldExist: false,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			reqLogger := testLogger.WithValues("test:", test.name)
			testMonitor := &datadoghqv1alpha1.DatadogMonitor{}
			_ = r.client.Get(context.TODO(), client.ObjectKey{Namespace: "foo", Name: test.objectName}, testMonitor)

			_, err := r.handleFinalizer(reqLogger, testMonitor)

			assert.NoError(t, err)
			if test.finalizerShouldExist {
				assert.True(t, controllerutil.ContainsFinalizer(testMonitor, datadogMonitorFinalizer))
			} else {
				assert.False(t, controllerutil.ContainsFinalizer(testMonitor, datadogMonitorFinalizer))
			}
		})
	}
}
