// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

// newStatePollingTestReconciler returns a reconciler for an existing monitor
// whose force sync is not due, and a counter of Datadog GET requests.
func newStatePollingTestReconciler(t *testing.T, status datadoghqv1alpha1.DatadogMonitorStatus, pollingDisabled bool) (*Reconciler, *datadoghqv1alpha1.DatadogMonitor, *atomic.Int32) {
	t.Helper()
	gets := &atomic.Int32{}
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/monitor/12345" {
			gets.Add(1)
			fmt.Fprint(w, `{"id":12345,"name":"test monitor","query":"q","type":"metric alert","overall_state":"OK"}`)
			return
		}
		http.Error(w, "unexpected request", http.StatusNotFound)
	}))
	t.Cleanup(httpServer.Close)

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()

	dm := &datadoghqv1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:       resourcesName,
			Namespace:  resourcesNamespace,
			Finalizers: []string{datadogMonitorFinalizer},
		},
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_10m):avg:system.cpu.user{*} > 1",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
			Name:    "test monitor",
			Message: "something is wrong",
		},
		Status: status,
	}
	now := metav1.Now()
	dm.Status.ID = 12345
	dm.Status.MonitorLastForceSyncTime = &now
	dm.Status.CurrentHash, _ = comparison.GenerateMD5ForSpec(&dm.Spec)

	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).WithObjects(dm).Build()
	r := &Reconciler{
		client:               c,
		apiReader:            c,
		datadogClient:        datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(testConfig)),
		credsManager:         config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:               s,
		recorder:             record.NewFakeRecorder(10),
		log:                  logf.Log.WithName("state-polling"),
		requeuePeriod:        defaultRequeuePeriod,
		statePollingDisabled: pollingDisabled,
	}
	return r, dm, gets
}

func findMonitorCondition(status datadoghqv1alpha1.DatadogMonitorStatus, t datadoghqv1alpha1.DatadogMonitorConditionType) *datadoghqv1alpha1.DatadogMonitorCondition {
	for i := range status.Conditions {
		if status.Conditions[i].Type == t {
			return &status.Conditions[i]
		}
	}
	return nil
}

func TestReconcileDatadogMonitor_StatePollingDisabled(t *testing.T) {
	stale := metav1.NewTime(time.Now().Add(-time.Hour))
	r, dm, gets := newStatePollingTestReconciler(t, datadoghqv1alpha1.DatadogMonitorStatus{
		MonitorState:               datadoghqv1alpha1.DatadogMonitorStateAlert,
		MonitorStateLastUpdateTime: &stale,
		TriggeredState:             []datadoghqv1alpha1.DatadogMonitorTriggeredState{{MonitorGroup: "host:a", State: datadoghqv1alpha1.DatadogMonitorStateAlert}},
	}, true)

	result, err := r.Reconcile(context.Background(), dm)
	require.NoError(t, err)
	assert.Equal(t, int32(0), gets.Load(), "state polling must not call Datadog")
	assert.Equal(t, ptr.To(handler.LowPriority), result.Priority)
	assert.LessOrEqual(t, result.RequeueAfter, defaultForceSyncPeriod)
	assert.Greater(t, result.RequeueAfter, defaultForceSyncPeriod-time.Minute, "requeue should wait for the force-sync deadline")

	got := &datadoghqv1alpha1.DatadogMonitor{}
	require.NoError(t, r.client.Get(context.Background(), client.ObjectKeyFromObject(dm), got))
	assert.Equal(t, datadoghqv1alpha1.DatadogMonitorState(ctrutils.StatePollingDisabledState), got.Status.MonitorState)
	assert.Nil(t, got.Status.MonitorStateLastUpdateTime)
	assert.NotNil(t, got.Status.MonitorStateLastTransitionTime)
	assert.Empty(t, got.Status.TriggeredState)
	c := findMonitorCondition(got.Status, datadoghqv1alpha1.DatadogMonitorConditionTypeStateSynced)
	require.NotNil(t, c)
	assert.Equal(t, corev1.ConditionFalse, c.Status)
	assert.Equal(t, ctrutils.StatePollingDisabledState, c.Reason)
}

func TestReconcileDatadogMonitor_StatePollingReenabled(t *testing.T) {
	now := metav1.Now()
	r, dm, gets := newStatePollingTestReconciler(t, datadoghqv1alpha1.DatadogMonitorStatus{
		MonitorState: datadoghqv1alpha1.DatadogMonitorState(ctrutils.StatePollingDisabledState),
		Conditions: []datadoghqv1alpha1.DatadogMonitorCondition{{
			Type:           datadoghqv1alpha1.DatadogMonitorConditionTypeStateSynced,
			Status:         corev1.ConditionFalse,
			Reason:         ctrutils.StatePollingDisabledState,
			LastUpdateTime: now, LastTransitionTime: now,
		}},
	}, false)

	result, err := r.Reconcile(context.Background(), dm)
	require.NoError(t, err)
	assert.Equal(t, int32(1), gets.Load(), "state should be polled immediately")
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)

	got := &datadoghqv1alpha1.DatadogMonitor{}
	require.NoError(t, r.client.Get(context.Background(), client.ObjectKeyFromObject(dm), got))
	assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateOK, got.Status.MonitorState)
	assert.NotNil(t, got.Status.MonitorStateLastUpdateTime)
	assert.Nil(t, findMonitorCondition(got.Status, datadoghqv1alpha1.DatadogMonitorConditionTypeStateSynced))
}
