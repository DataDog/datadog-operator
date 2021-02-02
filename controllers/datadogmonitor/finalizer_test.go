// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package datadogmonitor

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
)

var (
	testLogger logr.Logger = logf.ZapLogger(true)
)

func Test_handleFinalizer(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	metaNow := metav1.NewTime(time.Now())

	r := &Reconciler{
		client: fake.NewFakeClient(),
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
		{
			name: "a new DatadogMonitor object has a deletion timestamp",
			dm: &datadoghqv1alpha1.DatadogMonitor{
				TypeMeta: metav1.TypeMeta{
					Kind: "DatadogMonitor",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test monitor",
					DeletionTimestamp: &metaNow,
				},
				Status: datadoghqv1alpha1.DatadogMonitorStatus{
					Primary: false,
				},
			},
			finalizerShouldExist: false,
		},
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
