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
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
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
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})

	type args struct {
		request              *v1alpha1.DatadogMonitor
		loadFunc             func(c client.Client) *v1alpha1.DatadogMonitor
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
				loadFunc: genericDatadogMonitor,
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
				request:             newRequest(resourcesNamespace, resourcesName),
				loadFunc:            genericDatadogMonitor,
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
				request:             newRequest(resourcesNamespace, resourcesName),
				loadFunc:            genericDatadogMonitor,
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
				request:             newRequest(resourcesNamespace, resourcesName),
				loadFunc:            genericDatadogMonitor,
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
				request:             newRequest(resourcesNamespace, resourcesName),
				loadFunc:            genericDatadogMonitor1,
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					err := c.Delete(context.TODO(), newRequest(resourcesNamespace, resourcesName))
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
				request:             newRequest(resourcesNamespace, resourcesName),
				loadFunc:            testQueryMonitor,
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
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testServiceMonitor,

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
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testEventMonitor,

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
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testEventV2Monitor,

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
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testProcessMonitor,

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
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testTraceAnalyticsMonitor,

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
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testSLOMonitor,

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
			name: "DatadogMonitor, log alert",
			args: args{
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testLogMonitor,

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
				request:  newRequest(resourcesNamespace, resourcesName),
				loadFunc: testRUMMonitor,

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
				request:             newRequest(resourcesNamespace, resourcesName),
				loadFunc:            testAuditMonitor,
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
				loadFunc: func(c client.Client) *v1alpha1.DatadogMonitor {
					dm := &datadoghqv1alpha1.DatadogMonitor{
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
					}
					_ = c.Create(context.TODO(), dm)
					return dm
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
			dm := tt.args.request
			if tt.args.loadFunc != nil {
				dm = tt.args.loadFunc(r.client)
				// Make sure there's minimum 1 reconcile loop
				if tt.args.firstReconcileCount == 0 {
					tt.args.firstReconcileCount = 1
				}
			}
			var result ctrl.Result
			var err error
			for i := 0; i < tt.args.firstReconcileCount; i++ {
				result, err = r.Reconcile(context.TODO(), dm)
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
				r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)
				_, err := r.Reconcile(context.TODO(), dm)
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

func newRequest(ns, name string) *v1alpha1.DatadogMonitor {
	return &v1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{
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

func genericDatadogMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func genericDatadogMonitor1(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testQueryMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testServiceMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testEventMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testLogMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testProcessMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testRUMMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testSLOMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	threshold := "10"
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testEventV2Monitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testTraceAnalyticsMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

func testAuditMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
	dm := &datadoghqv1alpha1.DatadogMonitor{
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
	_ = c.Create(context.TODO(), dm)
	return dm
}

// TestReconciler_UpdateDatadogClient tests the UpdateDatadogClient method of the Reconciler
func TestReconciler_UpdateDatadogClient(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))
	recorder := record.NewFakeRecorder(10)
	scheme := scheme.Scheme
	client := fake.NewClientBuilder().Build()
	metricForwardersMgr := datadog.NewForwardersManager(client, nil, false, nil)

	tests := []struct {
		name     string
		newCreds config.Creds
		wantErr  bool
	}{
		{
			name: "valid credentials update",
			newCreds: config.Creds{
				APIKey: "test-api-key",
				AppKey: "test-app-key",
			},
			wantErr: false,
		},
		{
			name: "empty API key",
			newCreds: config.Creds{
				APIKey: "",
				AppKey: "test-app-key",
			},
			wantErr: true,
		},
		{
			name: "empty App key",
			newCreds: config.Creds{
				APIKey: "test-api-key",
				AppKey: "",
			},
			wantErr: true,
		},
		{
			name: "both keys empty",
			newCreds: config.Creds{
				APIKey: "",
				AppKey: "",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create reconciler with initial valid credentials
			initialCreds := config.Creds{
				APIKey: "initial-api-key",
				AppKey: "initial-app-key",
			}
			r, err := NewReconciler(client, initialCreds, scheme, testLogger, recorder, false, metricForwardersMgr)
			assert.NoError(t, err)

			// Store original client and auth references
			originalClient := r.datadogClient
			originalAuth := r.datadogAuth

			// Call UpdateDatadogClient
			err = r.UpdateDatadogClient(tt.newCreds)

			if tt.wantErr {
				assert.Error(t, err)
				// Verify original client and auth are preserved on error
				if originalClient != r.datadogClient {
					t.Errorf("Expected clients to be the same, but they are different")
				}
				if originalAuth != r.datadogAuth {
					t.Errorf("Expected client auth to be the same, but they are different")
				}
				assert.Equal(t, originalClient, r.datadogClient)
				assert.Equal(t, originalAuth, r.datadogAuth)
			} else {
				assert.NoError(t, err)
				// Verify client and auth are recreated
				// r.datadogAuth
				if originalClient == r.datadogClient {
					t.Errorf("Expected clients to be different, but they are the same")
				}
				if originalAuth == r.datadogAuth {
					t.Errorf("Expected auths to be different, but they are the same")
				}
			}
		})
	}
}
