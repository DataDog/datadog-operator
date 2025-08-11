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
	"testing"

	"github.com/stretchr/testify/assert"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	datadogV1 "github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
)

func TestReconcileDatadogMonitor_Reconcile(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogMonitor_Reconcile"})

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{}, &datadoghqv1alpha1.DatadogSLO{})

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
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
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
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
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
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
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
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
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
					err := c.Create(context.TODO(), genericDatadogMonitor())
					assert.NoError(t, err)
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					err := c.Delete(context.TODO(), genericDatadogMonitor())
					assert.NoError(t, err)
				},
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
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
			name: "DatadogMonitor, query alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testQueryMonitor())
				},
				firstReconcileCount: 2,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, service check alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testServiceMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, event alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testEventMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, event v2 alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testEventV2Monitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, process alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testProcessMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, trace analytics alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testTraceAnalyticsMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, SLO alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testSLOMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, SLO alert with SLORef",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testSLO())
				},
				firstReconcileCount: 10,

				secondAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testSLOMonitorWithSLORef())
				},
				secondReconcileCount: 10,
			},
			wantErr: false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.Equal(t, "error_budget(\"123\").over(\"7d\") > 10", dm.Spec.Query)
				return nil
			},
		},
		{
			name: "DatadogMonitor, log alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testLogMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, rum alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testRUMMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor, audit alert",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), testAuditMonitor())
				},

				firstReconcileCount: 10,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.NotContains(t, dm.Status.Conditions[0].Message, "error")
				return nil
			},
		},
		{
			name: "DatadogMonitor of unsupported type (composite)",
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
							Type:    datadoghqv1alpha1.DatadogMonitorTypeComposite,
							Name:    "test monitor",
							Message: "something is wrong",
						},
					})
				},
				firstReconcileCount: 2,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    false,
			wantFunc: func(c client.Client) error {
				dm := &datadoghqv1alpha1.DatadogMonitor{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm); err != nil {
					return err
				}
				assert.Equal(t, dm.Status.Conditions[0].Type, datadoghqv1alpha1.DatadogMonitorConditionTypeError)
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

			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			client := datadogV1.NewMonitorsApi(apiClient)

			testAuth := setupTestAuth(httpServer.URL)

			// Set up
			r := &Reconciler{
				client:        fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
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
	now := metav1.Unix(secondTriggerTs, 0)

	okState := datadogV1.MONITOROVERALLSTATES_OK
	alertState := datadogV1.MONITOROVERALLSTATES_ALERT
	noDataState := datadogV1.MONITOROVERALLSTATES_NO_DATA

	tests := []struct {
		name       string
		monitor    func() datadogV1.Monitor
		status     *datadoghqv1alpha1.DatadogMonitorStatus
		wantStatus *datadoghqv1alpha1.DatadogMonitorStatus
	}{
		{
			name: "3 groups, not alerting, empty status",
			monitor: func() datadogV1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogV1.MonitorStateGroup)
				msg["groupA"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogV1.MonitorState{
					Groups: msg,
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
			monitor: func() datadogV1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogV1.MonitorStateGroup)
				msg["groupA"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogV1.MonitorStateGroup{
					Status:          &alertState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogV1.MonitorState{
					Groups: msg,
				}
				m.OverallState = &alertState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{
					{
						MonitorGroup:       "groupC",
						State:              datadoghqv1alpha1.DatadogMonitorStateAlert,
						LastTransitionTime: metav1.Unix(triggerTs, 0),
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateAlert,
			},
		},
		{
			name: "3 groups, one alerting; OK status -> Alert status",
			monitor: func() datadogV1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogV1.MonitorStateGroup)
				msg["groupA"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogV1.MonitorStateGroup{
					Status:          &alertState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogV1.MonitorState{
					Groups: msg,
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
						MonitorGroup:       "groupC",
						State:              datadoghqv1alpha1.DatadogMonitorStateAlert,
						LastTransitionTime: metav1.Unix(triggerTs, 0),
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateAlert,
			},
		},
		{
			name: "3 groups, one no data, empty status",
			monitor: func() datadogV1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogV1.MonitorStateGroup)
				msg["groupA"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogV1.MonitorStateGroup{
					Status:          &noDataState,
					LastTriggeredTs: &triggerTs,
				}

				m.State = &datadogV1.MonitorState{
					Groups: msg,
				}
				m.OverallState = &noDataState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{
					{
						MonitorGroup:       "groupC",
						State:              datadoghqv1alpha1.DatadogMonitorStateNoData,
						LastTransitionTime: metav1.Unix(triggerTs, 0),
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateNoData,
			},
		},
		{
			name: "3 groups, one alerting, one no data, empty status",
			monitor: func() datadogV1.Monitor {
				m := genericMonitor(12345)

				msg := make(map[string]datadogV1.MonitorStateGroup)
				msg["groupA"] = datadogV1.MonitorStateGroup{
					Status:          &okState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupB"] = datadogV1.MonitorStateGroup{
					Status:          &alertState,
					LastTriggeredTs: &triggerTs,
				}
				msg["groupC"] = datadogV1.MonitorStateGroup{
					Status:          &noDataState,
					LastTriggeredTs: &secondTriggerTs,
				}

				m.State = &datadogV1.MonitorState{
					Groups: msg,
				}
				m.OverallState = &alertState

				return m
			},
			status: &datadoghqv1alpha1.DatadogMonitorStatus{},
			wantStatus: &datadoghqv1alpha1.DatadogMonitorStatus{
				TriggeredState: []datadoghqv1alpha1.DatadogMonitorTriggeredState{
					{
						MonitorGroup:       "groupB",
						State:              datadoghqv1alpha1.DatadogMonitorStateAlert,
						LastTransitionTime: metav1.Unix(triggerTs, 0),
					},
					{
						MonitorGroup:       "groupC",
						State:              datadoghqv1alpha1.DatadogMonitorStateNoData,
						LastTransitionTime: metav1.Unix(secondTriggerTs, 0),
					},
				},
				MonitorState: datadoghqv1alpha1.DatadogMonitorStateAlert,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			convertStateToStatus(tt.monitor(), tt.status, now)

			assert.Equal(t, tt.wantStatus.TriggeredState, tt.status.TriggeredState)
			assert.Equal(t, tt.wantStatus.MonitorState, tt.status.MonitorState)
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

func testQueryMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Type:    datadoghqv1alpha1.DatadogMonitorTypeQuery,
			Name:    "test query monitor",
			Message: "something is wrong",
		},
	}
}

func testServiceMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "\"kubernetes.kubelet.check\".over(\"*\").by(\"check\",\"id\").last(2).count_by_status()",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeService,
			Name:    "test service check monitor",
			Message: "something is wrong",
		},
	}
}

func testEventMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "events(\"sources:nagios status:error,warning priority:normal\").rollup(\"count\").last(\"1h\") > 10",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeEvent,
			Name:    "test event monitor",
			Message: "something is wrong",
		},
	}
}

func testLogMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "logs(\"source:nagios AND status:error\").index(\"default\").rollup(\"sum\").last(\"1h\") > 5",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeLog,
			Name:    "test log monitor",
			Message: "something is wrong",
		},
	}
}

func testProcessMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "processes(\"java AND elasticsearch\").over(\"*\").rollup(\"count\").last(\"1h\") > 5",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeProcess,
			Name:    "test process monitor",
			Message: "something is wrong",
		},
	}
}

func testRUMMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "rum(\"*\").rollup(\"count\").by(\"@type\").last(\"5m\") >= 55",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeRUM,
			Name:    "test rum monitor",
			Message: "something is wrong",
		},
	}
}

func testSLOMonitor() *datadoghqv1alpha1.DatadogMonitor {
	threshold := "10"
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
			Query: "error_budget(\"slo-hash-id\").over(\"7d\") > 10",
			Options: datadoghqv1alpha1.DatadogMonitorOptions{
				Thresholds: &datadoghqv1alpha1.DatadogMonitorOptionsThresholds{
					Critical: &threshold,
				},
			},
			Type:    datadoghqv1alpha1.DatadogMonitorTypeSLO,
			Name:    "test SLO monitor",
			Message: "something is wrong",
		},
	}
}
func testSLO() *v1alpha1.DatadogSLO {
	return &v1alpha1.DatadogSLO{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogMonitor",
			APIVersion: fmt.Sprintf("%s/%s", v1alpha1.GroupVersion.Group, v1alpha1.GroupVersion.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: resourcesNamespace,
			Name:      resourcesName,
		},
		Spec: v1alpha1.DatadogSLOSpec{
			Name: "Test SLO",
			Query: &v1alpha1.DatadogSLOQuery{
				Numerator:   "sum:my.custom.count.metric{type:good_events}.as_count()",
				Denominator: "sum:my.custom.count.metric{*}.as_count()",
			},
			Type:            v1alpha1.DatadogSLOTypeMetric,
			TargetThreshold: resource.MustParse("99.0"),
			Timeframe:       v1alpha1.DatadogSLOTimeFrame30d,
			Tags:            utils.GetRequiredTags(),
		},
		Status: v1alpha1.DatadogSLOStatus{
			ID: "123",
		},
	}
}
func testSLOMonitorWithSLORef() *datadoghqv1alpha1.DatadogMonitor {
	threshold := "10"
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
			SLORef: &datadoghqv1alpha1.SLORef{
				Name:      resourcesName,
				Namespace: resourcesNamespace,
			},
			Query: "error_budget(\"slo-hash-id\").over(\"7d\") > 10",
			Options: datadoghqv1alpha1.DatadogMonitorOptions{
				Thresholds: &datadoghqv1alpha1.DatadogMonitorOptionsThresholds{
					Critical: &threshold,
				},
			},
			Type:    datadoghqv1alpha1.DatadogMonitorTypeSLO,
			Name:    "test SLO monitor",
			Message: "something is wrong",
		},
	}
}

func testEventV2Monitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "events(\"source:nagios AND status:error\").rollup(\"sum\").last(\"1h\") > 5",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeEventV2,
			Name:    "test event v2 monitor",
			Message: "something is wrong",
		},
	}
}

func testTraceAnalyticsMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "trace-analytics(\"env:prod operation_name:pylons.request\").rollup(\"count\").by(\"*\").last(\"5m\") > 100",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeTraceAnalytics,
			Name:    "test audit monitor",
			Message: "something is wrong",
		},
	}
}

func testAuditMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "audits(\"status:error\").rollup(\"cardinality\", \"@usr.id\").last(\"5m\") > 250",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeAudit,
			Name:    "test audit monitor",
			Message: "something is wrong",
		},
	}
}
