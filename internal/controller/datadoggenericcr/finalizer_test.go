// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericcr

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	ctrl "sigs.k8s.io/controller-runtime"
)

const (
	genericCRKind = "DatadogGenericCR"
	testNamespace = "foo"
)

var (
	testMgr, _             = ctrl.NewManager(&rest.Config{}, manager.Options{})
	testLogger logr.Logger = zap.New(zap.UseDevMode(true))
)

func Test_handleFinalizer(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericCR{})
	metaNow := metav1.NewTime(time.Now())

	r := &Reconciler{
		client: fake.NewClientBuilder().
			WithRuntimeObjects(
				&datadoghqv1alpha1.DatadogGenericCR{
					TypeMeta: metav1.TypeMeta{
						Kind: genericCRKind,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "genericcr-create",
						Namespace: testNamespace,
					},
				},
				// Fake client preventes deletion timestamp from being set, so we init the store with an object that has:
				// - deletion timestamp (added by running kubectl delete)
				// - finalizer (added by the reconciler at creation time (see first test case))
				// Ref: https://github.com/kubernetes-sigs/controller-runtime/commit/7a66d580c0c53504f5b509b45e9300cc18a1cc30#diff-20ecedbf30721c01c33fb67d911da11c277e29990497a600d20cb0ec7215affdR683-R686
				&datadoghqv1alpha1.DatadogGenericCR{
					TypeMeta: metav1.TypeMeta{
						Kind: genericCRKind,
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:              "genericcr-delete",
						Namespace:         testNamespace,
						Finalizers:        []string{datadogGenericCRFinalizer},
						DeletionTimestamp: &metaNow,
					},
					// Spec: datadoghqv1alpha1.DatadogGenericCRSpec{
					// 	Type: "mock_resource",
					// },
				},
			).
			WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericCR{}).Build(),
		scheme:   s,
		log:      testLogger,
		recorder: testMgr.GetEventRecorderFor(genericCRKind),
	}

	tests := []struct {
		testName             string
		resourceName         string
		finalizerShouldExist bool
	}{
		{
			testName:             "a new DatadogGenericCR object gets a finalizer added successfully",
			resourceName:         "genericcr-create",
			finalizerShouldExist: true,
		},
		{
			testName:             "a DatadogGenericCR object (with the finalizer) has a deletion timestamp",
			resourceName:         "genericcr-delete",
			finalizerShouldExist: false,
		},
	}
	for _, test := range tests {
		t.Run(test.testName, func(t *testing.T) {
			reqLogger := testLogger.WithValues("test:", test.testName)
			testGcr := &datadoghqv1alpha1.DatadogGenericCR{}
			err := r.client.Get(context.TODO(), client.ObjectKey{Name: test.resourceName, Namespace: testNamespace}, testGcr)

			_, err = r.handleFinalizer(reqLogger, testGcr)

			assert.NoError(t, err)
			if test.finalizerShouldExist {
				assert.Contains(t, testGcr.GetFinalizers(), datadogGenericCRFinalizer)
			} else {
				assert.NotContains(t, testGcr.GetFinalizers(), datadogGenericCRFinalizer)
			}
		})
	}

}