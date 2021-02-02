// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2021 Datadog, Inc.

package datadogmonitor

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	logf "sigs.k8s.io/controller-runtime/pkg/runtime/log"

	datadogapiclientv1 "github.com/DataDog/datadog-api-client-go/api/v1/datadog"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
)

func TestReconcileDatadogMonitor_Reconcile(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogMonitor_Reconcile"})

	logf.SetLogger(logf.ZapLogger(true))

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})

	type args struct {
		request              reconcile.Request
		firstAction          func(c client.Client)
		firstReconcileCount  int
		secondAction         func(c client.Client)
		secondReconcileCount int
	}

	tests := []struct {
		name       string
		args       args
		wantResult reconcile.Result
		wantErr    bool
		wantFunc   func(c client.Client) error
	}{
		{
			name: "DatadogMonitor not created",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
			},
			wantResult: reconcile.Result{},
		},
		{
			name: "DatadogMonitor created, add finalizer",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), genericDatadogMonitor())
				},
			},
			wantResult: reconcile.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod},
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.Contains(t, dm.GetFinalizers(), "finalizer.monitor.datadoghq.com")
				return nil
			},
		},
		{
			name: "DatadogMonitor created, check Status.Primary",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), genericDatadogMonitor())
				},
				firstReconcileCount: 3,
			},
			wantResult: reconcile.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod},
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.True(t, dm.Status.Primary)
				return nil
			},
		},
		{
			name: "DatadogMonitor exists, check required tags",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), genericDatadogMonitor())
				},
				firstReconcileCount: 2,
			},
			wantResult: reconcile.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod},
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.Contains(t, dm.Spec.Tags, "generated:kubernetes")
				return nil
			},
		},
		{
			name: "DatadogMonitor exists, needs update",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), genericDatadogMonitor())
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					_ = c.Update(context.TODO(), &datadoghqv1alpha1.DatadogMonitor{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogMonitor",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: resourcesNamespace,
							Name:      resourcesName,
						},
						Spec: datadoghqv1alpha1.DatadogMonitorSpec{
							// Update query threshold
							Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.2",
							Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
							Name:    "test monitor",
							Message: "something is wrong",
						},
					})
				},
				secondReconcileCount: 2,
			},
			wantResult: reconcile.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod},
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				// Make sure status hash is up to date
				hash, _ := comparison.GenerateMD5ForSpec(dm.Spec)
				assert.Equal(t, dm.Status.CurrentHash, hash)
				return nil
			},
		},
		{
			name: "DatadogMonitor exists, needs delete",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), genericDatadogMonitor())
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					_ = c.Delete(context.TODO(), genericDatadogMonitor())
				},
			},
			wantResult: reconcile.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod},
			wantErr:    true,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				return nil
			},
		},
		{
			name: "DatadogMonitor of unsupported type",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), &datadoghqv1alpha1.DatadogMonitor{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogMonitor",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: resourcesNamespace,
							Name:      resourcesName,
						},
						Spec: datadoghqv1alpha1.DatadogMonitorSpec{
							Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
							Type:    datadoghqv1alpha1.DatadogMonitorTypeQuery,
							Name:    "test monitor",
							Message: "something is wrong",
						},
					})
				},
				firstReconcileCount: 2,
			},
			wantResult: reconcile.Result{Requeue: true, RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.Nil(t, dm.Status.Created)
				assert.Contains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
			}))
			defer httpServer.Close()

			testConfig := datadogapiclientv1.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			client := datadogapiclientv1.NewAPIClient(testConfig)
			testAuth := setupTestAuth(httpServer.URL)

			// Set up
			r := &Reconciler{
				client:        fake.NewFakeClient(),
				datadogClient: client,
				datadogAuth:   testAuth,
				scheme:        s,
				recorder:      recorder,
				log:           logf.Log.WithName(tt.name),
			}

			// First monitor action
			if tt.args.firstAction != nil {
				tt.args.firstAction(r.client)
				// Make sure there's minimum 1 reconcile loop
				if tt.args.firstReconcileCount == 0 {
					tt.args.firstReconcileCount = 1
				}
			}
			var result ctrl.Result
			var err error
			for i := 0; i < tt.args.firstReconcileCount; i++ {
				result, err = r.Reconcile(context.TODO(), tt.args.request)
			}

			assert.NoError(t, err, "ReconcileDatadogMonitor.Reconcile() unexpected error: %v", err)
			assert.Equal(t, tt.wantResult, result, "ReconcileDatadogMonitor.Reconcile() unexpected result")

			// Second monitor action
			if tt.args.secondAction != nil {
				tt.args.secondAction(r.client)
				// Make sure there's minimum 1 reconcile loop
				if tt.args.secondReconcileCount == 0 {
					tt.args.secondReconcileCount = 1
				}
			}
			for i := 0; i < tt.args.secondReconcileCount; i++ {
				_, err := r.Reconcile(context.TODO(), tt.args.request)
				assert.NoError(t, err, "ReconcileDatadogMonitor.Reconcile() unexpected error: %v", err)
			}

			if tt.wantFunc != nil {
				err := tt.wantFunc(r.client)
				if tt.wantErr {
					assert.Error(t, err, "ReconcileDatadogMonitor.Reconcile() expected an error")
				} else {
					assert.NoError(t, err, "ReconcileDatadogMonitor.Reconcile() wantFunc validation error: %v", err)
				}
			}
		})
	}
}

func genericDatadogMonitor() *datadoghqv1alpha1.DatadogMonitor {
	return &datadoghqv1alpha1.DatadogMonitor{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogMonitor",
			APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: resourcesNamespace,
			Name:      resourcesName,
		},
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.1",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
			Name:    "test monitor",
			Message: "something is wrong",
		},
	}
}

func newRequest(ns, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      name,
		},
	}
}

func Test_convertStateToStatus(t *testing.T) {
	triggerTs := int64(1612244495)
	secondTriggerTs := triggerTs + 300

	okState := datadogapiclientv1.MONITOROVERALLSTATES_OK
	alertState := datadogapiclientv1.MONITOROVERALLSTATES_ALERT
	noDataState := datadogapiclientv1.MONITOROVERALLSTATES_NO_DATA

	tests := []struct {
		name       string
		monitor    func() datadogapiclientv1.Monitor
		status     *datadoghqv1alpha1.DatadogMonitorStatus
		wantStatus *datadoghqv1alpha1.DatadogMonitorStatus
	}{
		{
			name: "3 groups, not alerting, empty status",
			monitor: func() datadogapiclientv1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogapiclientv1.MonitorStateGroup)
				msg["groupA"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogapiclientv1.MonitorState{
					Groups: &msg,
				}
				m.OverallState = &okState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{},
				MonitorState:   datadoghqv1alpha1.DatadogMonitorStateOK,
			},
		},
		{
			name: "3 groups, one alerting, empty status",
			monitor: func() datadogapiclientv1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogapiclientv1.MonitorStateGroup)
				msg["groupA"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &alertState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogapiclientv1.MonitorState{
					Groups: &msg,
				}
				m.OverallState = &alertState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{
					{
						MonitorGroup:     "groupC",
						State:            datadoghqv1alpha1.DatadogMonitorStateAlert,
						LastTransitionTs: triggerTs,
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateAlert,
			},
		},
		{
			name: "3 groups, one alerting; OK status -> Alert status",
			monitor: func() datadogapiclientv1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogapiclientv1.MonitorStateGroup)
				msg["groupA"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &alertState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogapiclientv1.MonitorState{
					Groups: &msg,
				}
				m.OverallState = &alertState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateOK,
			},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{
					{
						MonitorGroup:     "groupC",
						State:            datadoghqv1alpha1.DatadogMonitorStateAlert,
						LastTransitionTs: triggerTs,
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateAlert,
			},
		},
		{
			name: "3 groups, one no data, empty status",
			monitor: func() datadogapiclientv1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogapiclientv1.MonitorStateGroup)
				msg["groupA"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &noDataState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogapiclientv1.MonitorState{
					Groups: &msg,
				}
				m.OverallState = &noDataState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{
					{
						MonitorGroup:     "groupC",
						State:            datadoghqv1alpha1.DatadogMonitorStateNoData,
						LastTransitionTs: triggerTs,
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateNoData,
			},
		},
		{
			name: "3 groups, one alerting, one no data, empty status",
			monitor: func() datadogapiclientv1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogapiclientv1.MonitorStateGroup)
				msg["groupA"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &alertState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogapiclientv1.MonitorStateGroup{
					Status:          &noDataState,
					LastTriggeredTs: &secondTriggerTs,
				}

				m.State = &datadogapiclientv1.MonitorState{
					Groups: &msg,
				}
				m.OverallState = &alertState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{
					{
						MonitorGroup:     "groupB",
						State:            datadoghqv1alpha1.DatadogMonitorStateAlert,
						LastTransitionTs: triggerTs,
					},
					{
						MonitorGroup:     "groupC",
						State:            datadoghqv1alpha1.DatadogMonitorStateNoData,
						LastTransitionTs: secondTriggerTs,
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateAlert,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			convertStateToStatus(tt.monitor(), tt.status)

			assert.Equal(t, tt.wantStatus.TriggeredState, tt.status.TriggeredState)
			assert.Equal(t, tt.wantStatus.MonitorState, tt.status.MonitorState)
		})
	}
}
