// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogdashboard

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

func TestReconcileDatadogDashboard_OrphanPolicy(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogDashboard{})

	var requests, deletes atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodDelete {
			deletes.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"abc-def-ghi","title":"test dashboard","layout_type":"ordered","widgets":[]}`))
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "api-key")
	t.Setenv("DD_APP_KEY", "app-key")

	r := &Reconciler{
		client:        fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogDashboard{}).Build(),
		datadogClient: datadogV1.NewDashboardsApi(datadogapi.NewAPIClient(testConfig)),
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      record.NewFakeRecorder(10),
		log:           logf.Log.WithName(t.Name()),
	}
	ctx := context.TODO()
	require.NoError(t, r.client.Create(ctx, genericDatadogDashboard()))
	req := newRequest(resourcesNamespace, resourcesName)
	get := func() *datadoghqv1alpha1.DatadogDashboard {
		t.Helper()
		got := &datadoghqv1alpha1.DatadogDashboard{}
		require.NoError(t, r.client.Get(ctx, req.NamespacedName, got))
		return got
	}

	// Add the finalizer, then create the dashboard.
	for range 2 {
		_, err := r.Reconcile(ctx, req)
		require.NoError(t, err)
	}
	require.Equal(t, "abc-def-ghi", get().Status.ID)

	// Orphan the CR and change its spec: nothing is sent to Datadog.
	db := get()
	db.Annotations = map[string]string{datadoghqv1alpha1.ManagementPolicyAnnotationKey: datadoghqv1alpha1.ManagementPolicyOrphan}
	db.Spec.Title = "changed while orphaned"
	require.NoError(t, r.client.Update(ctx, db))
	before := requests.Load()
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, before, requests.Load(), "no Datadog API call while orphaned")
	assert.True(t, meta.IsStatusConditionTrue(get().Status.Conditions, string(condition.DatadogConditionTypeOrphaned)))

	// Deleting the orphaned CR keeps the dashboard.
	require.NoError(t, r.client.Delete(ctx, get()))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, int32(0), deletes.Load())
	assert.True(t, apierrors.IsNotFound(r.client.Get(ctx, req.NamespacedName, &datadoghqv1alpha1.DatadogDashboard{})))
}
