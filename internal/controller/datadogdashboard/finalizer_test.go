// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogdashboard

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
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogDashboard{})

	r := &Reconciler{
		client: fake.NewClientBuilder().
			WithStatusSubresource(&datadoghqv1alpha1.DatadogDashboard{}).Build(),
		scheme: s,
		log:    testLogger,
	}

	testCases := []struct {
		name                 string
		db                   *datadoghqv1alpha1.DatadogDashboard
		finalizerShouldExist bool
	}{
		{
			name: "a new DatadogDashboard object gets a finalizer added successfully",
			db: &datadoghqv1alpha1.DatadogDashboard{
				TypeMeta: metav1.TypeMeta{
					Kind: "DatadogDashboard",
				},
				ObjectMeta: metav1.ObjectMeta{
					Name: "test DasDatadogDashboard",
				},
			},
			finalizerShouldExist: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			reqLogger := testLogger.WithValues("test:", test.name)
			_ = r.client.Create(context.TODO(), test.db)
			_, err := r.handleFinalizer(reqLogger, test.db)
			assert.NoError(t, err)
			if test.finalizerShouldExist {
				assert.True(t, utils.ContainsString(test.db.GetFinalizers(), datadogDashboardFinalizer))
			} else {
				assert.False(t, utils.ContainsString(test.db.GetFinalizers(), datadogDashboardFinalizer))
			}
		})
	}
}
