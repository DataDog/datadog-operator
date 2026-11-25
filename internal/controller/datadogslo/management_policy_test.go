// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogslo

import (
	"context"
	"encoding/json"
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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

func TestReconciler_OrphanPolicy(t *testing.T) {
	s := scheme.Scheme
	s.AddKnownTypes(v1alpha1.GroupVersion, &v1alpha1.DatadogSLO{})

	var requests, deletes atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method == http.MethodDelete {
			deletes.Add(1)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(defaultDatadogSLOResponse())
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	kubeClient := fake.NewClientBuilder().WithStatusSubresource(&v1alpha1.DatadogSLO{}).Build()
	require.NoError(t, kubeClient.Create(context.TODO(), defaultSLO()))
	r := &Reconciler{
		client:        kubeClient,
		datadogClient: datadogV1.NewServiceLevelObjectivesApi(datadogapi.NewAPIClient(testConfig)),
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		recorder:      record.NewFakeRecorder(10),
		log:           zap.New(zap.UseDevMode(true)),
	}
	ctx := context.Background()
	req := ctrl.Request{NamespacedName: types.NamespacedName{Namespace: resourceNamespace, Name: resourceName}}
	get := func() *v1alpha1.DatadogSLO {
		t.Helper()
		got := &v1alpha1.DatadogSLO{}
		require.NoError(t, kubeClient.Get(ctx, req.NamespacedName, got))
		return got
	}

	// Add the finalizer, then create the SLO.
	for range 2 {
		_, err := r.Reconcile(ctx, req)
		require.NoError(t, err)
	}
	require.Equal(t, "SLO123", get().Status.ID)

	// Orphan the CR and change its spec: nothing is sent to Datadog.
	slo := get()
	slo.Annotations = map[string]string{v1alpha1.ManagementPolicyAnnotationKey: v1alpha1.ManagementPolicyOrphan}
	slo.Spec.Description = new("changed while orphaned")
	require.NoError(t, kubeClient.Update(ctx, slo))
	before := requests.Load()
	result, err := r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, ctrl.Result{}, result)
	assert.Equal(t, before, requests.Load(), "no Datadog API call while orphaned")
	assert.True(t, meta.IsStatusConditionTrue(get().Status.Conditions, string(condition.DatadogConditionTypeOrphaned)))

	// Deleting the orphaned CR keeps the SLO.
	require.NoError(t, kubeClient.Delete(ctx, get()))
	_, err = r.Reconcile(ctx, req)
	require.NoError(t, err)
	assert.Equal(t, int32(0), deletes.Load())
	assert.True(t, apierrors.IsNotFound(kubeClient.Get(ctx, req.NamespacedName, &v1alpha1.DatadogSLO{})))
}
