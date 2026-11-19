// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

// statelessMockHandler hides MockHandler's liveStateHandler marker.
type statelessMockHandler struct{ ResourceHandler }

func newStatePollingTestReconciler(t *testing.T, h ResourceHandler) *Reconciler {
	t.Helper()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
	t.Setenv(ctrutils.StatePollingEnabledEnvVar, "false")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logf.Log.WithName("state-polling-test"),
		record.NewFakeRecorder(10),
	)
	require.True(t, r.statePollingDisabled)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{mockSubresource: h}
	require.NoError(t, r.client.Create(context.TODO(), mockGenericResource()))
	return r
}

func getGenericResource(t *testing.T, r *Reconciler) *datadoghqv1alpha1.DatadogGenericResource {
	t.Helper()
	obj := &datadoghqv1alpha1.DatadogGenericResource{}
	require.NoError(t, r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj))
	return obj
}

// assertRequeuesAtForceSync checks the requeue lands on the force-sync deadline, not the polling period.
func assertRequeuesAtForceSync(t *testing.T, result reconcile.Result) {
	t.Helper()
	assert.LessOrEqual(t, result.RequeueAfter, defaultForceSyncPeriod)
	assert.Greater(t, result.RequeueAfter, defaultForceSyncPeriod-time.Minute)
}

func TestReconcileGenericResource_StatePollingDisabled(t *testing.T) {
	resetMockHandlerState()
	alert := "Alert"
	mockRefreshStateResult = &alert
	r := newStatePollingTestReconciler(t, &MockHandler{})
	req := newRequest(resourcesNamespace, resourcesName)

	// Add finalizer, then create the Datadog resource.
	_, err := reconcileRequest(r, context.TODO(), req)
	require.NoError(t, err)
	result, err := reconcileRequest(r, context.TODO(), req)
	require.NoError(t, err)
	assertRequeuesAtForceSync(t, result)

	// Idle tick: no state refresh, sentinel state and condition are set.
	result, err = reconcileRequest(r, context.TODO(), req)
	require.NoError(t, err)
	assertRequeuesAtForceSync(t, result)
	assert.Equal(t, 0, mockRefreshStateCalls)

	obj := getGenericResource(t, r)
	assert.Equal(t, ctrutils.StatePollingDisabledState, obj.Status.State)
	assert.Nil(t, obj.Status.StateLastUpdateTime)
	assert.NotNil(t, obj.Status.StateLastTransitionTime)
	c := meta.FindStatusCondition(obj.Status.Conditions, string(condition.DatadogConditionTypeStateSynced))
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionFalse, c.Status)
	assert.Equal(t, ctrutils.StatePollingDisabledState, c.Reason)

	// Re-enabling polling refreshes the state on the next reconcile.
	r.statePollingDisabled = false
	result, err = reconcileRequest(r, context.TODO(), req)
	require.NoError(t, err)
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)
	assert.Equal(t, 1, mockRefreshStateCalls)

	obj = getGenericResource(t, r)
	assert.Equal(t, "Alert", obj.Status.State)
	assert.NotNil(t, obj.Status.StateLastUpdateTime)
	c = meta.FindStatusCondition(obj.Status.Conditions, string(condition.DatadogConditionTypeStateSynced))
	require.NotNil(t, c)
	assert.Equal(t, metav1.ConditionTrue, c.Status)
}

func TestReconcileGenericResource_StatePollingDisabledStatelessType(t *testing.T) {
	resetMockHandlerState()
	r := newStatePollingTestReconciler(t, statelessMockHandler{&MockHandler{}})
	req := newRequest(resourcesNamespace, resourcesName)

	for range 3 {
		_, err := reconcileRequest(r, context.TODO(), req)
		require.NoError(t, err)
	}

	obj := getGenericResource(t, r)
	assert.Empty(t, obj.Status.State)
	assert.Nil(t, meta.FindStatusCondition(obj.Status.Conditions, string(condition.DatadogConditionTypeStateSynced)))
	assert.Equal(t, 0, mockRefreshStateCalls)
}

func TestUntilForceSync(t *testing.T) {
	now := metav1.Now()
	r := &Reconciler{requeuePeriod: defaultRequeuePeriod, forceSyncPeriod: defaultForceSyncPeriod}
	at := func(d time.Duration) *metav1.Time { return &metav1.Time{Time: now.Add(-d)} }

	tests := []struct {
		name   string
		status datadoghqv1alpha1.DatadogGenericResourceStatus
		want   time.Duration
	}{
		{name: "no timestamps", status: datadoghqv1alpha1.DatadogGenericResourceStatus{}, want: defaultForceSyncPeriod},
		{name: "falls back to created", status: datadoghqv1alpha1.DatadogGenericResourceStatus{Created: at(10 * time.Minute)}, want: 50 * time.Minute},
		{name: "last force sync", status: datadoghqv1alpha1.DatadogGenericResourceStatus{Created: at(5 * time.Hour), LastForceSyncTime: at(20 * time.Minute)}, want: 40 * time.Minute},
		{name: "overdue retries at requeue period", status: datadoghqv1alpha1.DatadogGenericResourceStatus{LastForceSyncTime: at(2 * time.Hour)}, want: defaultRequeuePeriod},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, r.untilForceSync(&tt.status, now))
		})
	}
}
