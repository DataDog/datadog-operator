// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogdashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
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
			final := finalizer.NewFinalizer(reqLogger, r.client, r.deleteResource(reqLogger, context.Background()), defaultRequeuePeriod, defaultErrRequeuePeriod)
			_, err := final.HandleFinalizer(context.TODO(), test.db, test.db.Status.ID, datadogDashboardFinalizer)
			assert.NoError(t, err)
			if test.finalizerShouldExist {
				assert.True(t, controllerutil.ContainsFinalizer(test.db, datadogDashboardFinalizer))
			} else {
				assert.False(t, controllerutil.ContainsFinalizer(test.db, datadogDashboardFinalizer))
			}
		})
	}
}

func Test_handleFinalizerDeletion(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogDashboard{})

	testCases := []struct {
		name                 string
		statusCode           int
		wantErr              bool
		finalizerShouldExist bool
	}{
		{
			name:                 "finalizer is removed once the dashboard is deleted",
			statusCode:           http.StatusOK,
			finalizerShouldExist: false,
		},
		{
			name:                 "finalizer is removed when the dashboard is already gone",
			statusCode:           http.StatusNotFound,
			finalizerShouldExist: false,
		},
		{
			name:                 "finalizer is kept when the delete call fails",
			statusCode:           http.StatusInternalServerError,
			wantErr:              true,
			finalizerShouldExist: true,
		},
	}

	for _, test := range testCases {
		t.Run(test.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.statusCode)
				_, _ = w.Write([]byte(`{}`))
			}))
			defer httpServer.Close()

			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()

			now := metav1.Now()
			db := &datadoghqv1alpha1.DatadogDashboard{
				TypeMeta: metav1.TypeMeta{Kind: "DatadogDashboard"},
				ObjectMeta: metav1.ObjectMeta{
					Name:              "dashboard-to-delete",
					DeletionTimestamp: &now,
					Finalizers:        []string{datadogDashboardFinalizer},
				},
				Status: datadoghqv1alpha1.DatadogDashboardStatus{ID: "abc-def-ghi"},
			}

			r := &Reconciler{
				client: fake.NewClientBuilder().
					WithRuntimeObjects(db).
					WithStatusSubresource(&datadoghqv1alpha1.DatadogDashboard{}).Build(),
				datadogClient: datadogV1.NewDashboardsApi(datadogapi.NewAPIClient(testConfig)),
				scheme:        s,
				log:           testLogger,
				recorder:      record.NewFakeRecorder(10),
			}

			reqLogger := testLogger.WithValues("test:", test.name)
			final := finalizer.NewFinalizer(reqLogger, r.client, r.deleteResource(reqLogger, setupTestAuth(httpServer.URL)), defaultRequeuePeriod, defaultErrRequeuePeriod)
			_, err := final.HandleFinalizer(context.TODO(), db, db.Status.ID, datadogDashboardFinalizer)
			if test.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, test.finalizerShouldExist, controllerutil.ContainsFinalizer(db, datadogDashboardFinalizer))
		})
	}
}
