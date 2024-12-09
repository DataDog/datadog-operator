// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
)

var (
	testLogger logr.Logger = zap.New(zap.UseDevMode(true))
)

func Test_handleFinalizer(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	// metaNow := metav1.NewTime(time.Now())

	r := &Reconciler{
		client: fake.NewClientBuilder().
			WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
		scheme: s,
		log:    testLogger,
	}

	testCases := []struct {
		name                 string
		dm                   *datadoghqv1alpha1.DatadogMonitor
		finalizerShouldExist bool
	}{
		{
			name: "a new DatadogMonitor object gets a finalizer added successfully",
			dm: &datadoghqv1alpha1.DatadogMonitor{
				TypeMeta: metav1.TypeMeta{
					Kind: "DatadogMonitor",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test monitor",
				},
			},
			finalizerShouldExist: true,
		},
		// {
		// 	name: "a new DatadogMonitor object has a deletion timestamp",
		// 	dm: &datadoghqv1alpha1.DatadogMonitor{
		// 		TypeMeta: metav1.TypeMeta{
		// 			Kind: "DatadogMonitor",
		// 		},
		// 		ObjectMeta: metav1.ObjectMeta{
		// 			Name: "test monitor",
		// 			// https://github.com/kubernetes-sigs/controller-runtime/commit/7a66d580c0c53504f5b509b45e9300cc18a1cc30#diff-20ecedbf30721c01c33fb67d911da11c277e29990497a600d20cb0ec7215affdR683-R686
		// 			// this is getting wiped upon creation with new controller-runtime
		// 			DeletionTimestamp: &metaNow,
		// 		},
		// 		Status: datadoghqv1alpha1.DatadogMonitorStatus{
		// 			Primary: false,
		// 		},
		// 	},
		// 	finalizerShouldExist: false,
		// },
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			reqLogger := testLogger.WithValues("test:", test.name)
			_ = r.client.Create(context.TODO(), test.dm)
			_, err := r.handleFinalizer(reqLogger, test.dm)
			assert.NoError(t, err)
			if test.finalizerShouldExist {
				assert.True(t, utils.ContainsString(test.dm.GetFinalizers(), datadogMonitorFinalizer))
			} else {
				assert.False(t, utils.ContainsString(test.dm.GetFinalizers(), datadogMonitorFinalizer))
			}
		})
	}
}
