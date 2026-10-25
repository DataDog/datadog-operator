// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/monitorpoller"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
)

func lowPriorityResult(requeueAfter time.Duration) reconcile.Result {
	return reconcile.Result{RequeueAfter: requeueAfter, Priority: ptr.To(handler.LowPriority)}
}

type conflictOnceClient struct {
	client.Client
	remainingConflicts atomic.Int32
}

func (c *conflictOnceClient) Status() client.SubResourceWriter {
	return &conflictOnceStatusWriter{
		SubResourceWriter: c.Client.Status(),
		client:            c,
	}
}

type conflictOnceStatusWriter struct {
	client.SubResourceWriter
	client *conflictOnceClient
}

func (w *conflictOnceStatusWriter) Update(ctx context.Context, obj client.Object, opts ...client.SubResourceUpdateOption) error {
	if w.client.remainingConflicts.CompareAndSwap(1, 0) {
		latest := &datadoghqv1alpha1.DatadogMonitor{}
		if err := w.client.Client.Get(ctx, client.ObjectKeyFromObject(obj), latest); err != nil {
			return err
		}
		if latest.Annotations == nil {
			latest.Annotations = map[string]string{}
		}
		latest.Annotations["issue-3272-conflict"] = "true"
		if err := w.client.Client.Update(ctx, latest); err != nil {
			return err
		}

		return apierrors.NewConflict(
			datadoghqv1alpha1.GroupVersion.WithResource("datadogmonitors").GroupResource(),
			obj.GetName(),
			fmt.Errorf("injected status update conflict"),
		)
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

func TestReconcileDatadogMonitor_RetriesCreatedStatusAfterConflict(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogMonitor_RetriesCreatedStatusAfterConflict"})

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})

	var createCount atomic.Int32
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validate"):
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/monitor":
			createCount.Add(1)
			fmt.Fprint(w, `{"id":12345,"name":"test monitor","query":"q","type":"metric alert","created":"2026-07-21T08:00:00Z","creator":{"email":"test@example.com"}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/monitor/12345":
			fmt.Fprint(w, `{"id":12345,"name":"test monitor","query":"q","type":"metric alert"}`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/monitor/12345":
			fmt.Fprint(w, `{"id":12345,"name":"test monitor","query":"q","type":"metric alert"}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer httpServer.Close()

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	ddClient := datadogV1.NewMonitorsApi(apiClient)

	baseClient := fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build()
	conflictingClient := &conflictOnceClient{Client: baseClient}
	r := &Reconciler{
		client:        conflictingClient,
		datadogClient: ddClient,
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      recorder,
		log:           logf.Log.WithName("created-status-conflict"),
		requeuePeriod: defaultRequeuePeriod,
	}

	dm := genericDatadogMonitor(r.client)
	dm.Finalizers = []string{datadogMonitorFinalizer}
	dm.Spec.Tags = []string{requiredTag}
	assert.NoError(t, r.client.Update(context.TODO(), dm))
	assert.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm))

	conflictingClient.remainingConflicts.Store(1)
	result, err := r.Reconcile(context.TODO(), dm)
	assert.NoError(t, err)
	assert.Equal(t, lowPriorityResult(defaultRequeuePeriod), result)
	assert.Equal(t, int32(0), conflictingClient.remainingConflicts.Load(), "the injected status conflict should be consumed")

	assert.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm))
	assert.Equal(t, 12345, dm.Status.ID, "the ID returned by the successful create must be persisted")
	assert.True(t, dm.Status.Primary)
	assert.Equal(t, "true", dm.Annotations["issue-3272-conflict"], "the concurrent metadata update must be preserved")

	_, err = r.Reconcile(context.TODO(), dm)
	assert.NoError(t, err)
	assert.Equal(t, int32(1), createCount.Load(), "a status conflict must not cause a second Datadog create")
}

func TestRequeuePeriod(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName("requeue-period-test")

	assert.Equal(t, defaultRequeuePeriod, requeuePeriod(logger, 0))
	assert.Equal(t, 2*time.Minute, requeuePeriod(logger, 2*time.Minute))
}

func TestNextRequeueAfter(t *testing.T) {
	now := metav1.NewTime(time.Now())
	lastForceSync := metav1.NewTime(now.Add(-50 * time.Minute))
	created := metav1.NewTime(now.Add(-30 * time.Minute))

	tests := []struct {
		name               string
		status             *datadoghqv1alpha1.DatadogMonitorStatus
		statePollingPeriod time.Duration
		forceSyncPeriod    time.Duration
		want               time.Duration
	}{
		{
			name:               "state polling is sooner",
			status:             &datadoghqv1alpha1.DatadogMonitorStatus{ID: 1, MonitorLastForceSyncTime: &now},
			statePollingPeriod: time.Minute,
			forceSyncPeriod:    time.Hour,
			want:               time.Minute,
		},
		{
			name:               "force sync is sooner",
			status:             &datadoghqv1alpha1.DatadogMonitorStatus{ID: 1, MonitorLastForceSyncTime: &lastForceSync},
			statePollingPeriod: time.Hour,
			forceSyncPeriod:    time.Hour,
			want:               10 * time.Minute,
		},
		{
			name:               "creation time is force sync fallback",
			status:             &datadoghqv1alpha1.DatadogMonitorStatus{ID: 1, Created: &created},
			statePollingPeriod: time.Hour,
			forceSyncPeriod:    time.Hour,
			want:               30 * time.Minute,
		},
		{
			name:               "resource without remote ID uses polling period",
			status:             &datadoghqv1alpha1.DatadogMonitorStatus{},
			statePollingPeriod: 24 * time.Hour,
			forceSyncPeriod:    time.Hour,
			want:               24 * time.Hour,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, nextRequeueAfter(tt.status, now, tt.statePollingPeriod, tt.forceSyncPeriod))
		})
	}
}

func TestReconcileDatadogMonitor_CreateErrorUsesErrorRequeuePeriod(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "temporary failure", http.StatusInternalServerError)
	}))
	defer httpServer.Close()

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	ddClient := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(testConfig))
	dm := &datadoghqv1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:       resourcesName,
			Namespace:  resourcesNamespace,
			Finalizers: []string{datadogMonitorFinalizer},
		},
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_5m):avg:system.load.1{*} > 100000",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
			Name:    "test monitor",
			Message: "load test",
			Tags:    []string{requiredTag},
		},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).WithObjects(dm).Build()
	r := &Reconciler{
		client:        kubeClient,
		datadogClient: ddClient,
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      record.NewFakeRecorder(1),
		log:           logf.Log.WithName("create-error-requeue"),
		requeuePeriod: 24 * time.Hour,
	}

	result, err := r.Reconcile(context.Background(), dm)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, result)
}

func TestReconcileDatadogMonitor_PermanentCreateErrorUsesForceSyncBackstop(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"errors":["invalid query"]}`, http.StatusBadRequest)
	}))
	defer httpServer.Close()

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	ddClient := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(testConfig))
	dm := &datadoghqv1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:       resourcesName,
			Namespace:  resourcesNamespace,
			Finalizers: []string{datadogMonitorFinalizer},
		},
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "avg(last_5m):avg:system.load.1{*} > 100000",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
			Name:    "test monitor",
			Message: "load test",
			Tags:    []string{requiredTag},
		},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).WithObjects(dm).Build()
	r := &Reconciler{
		client:        kubeClient,
		datadogClient: ddClient,
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      record.NewFakeRecorder(1),
		log:           logf.Log.WithName("permanent-create-error-backstop"),
		requeuePeriod: 24 * time.Hour,
	}

	result, err := r.Reconcile(context.Background(), dm)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: defaultForceSyncPeriod}, result)
	assert.Equal(t, 0, dm.Status.ID, "the monitor must not be considered created")
}

func TestReconcileDatadogMonitor_SpecFixAfterPermanentErrorRetriesImmediately(t *testing.T) {
	// A spec fix bumps .metadata.generation, so the watch reconciles
	// immediately regardless of the previous RequeueAfter.
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	var succeed atomic.Bool
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validate"):
			if succeed.Load() {
				fmt.Fprint(w, `{}`)
				return
			}
			http.Error(w, `{"errors":["invalid query"]}`, http.StatusBadRequest)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/monitor":
			fmt.Fprint(w, `{"id":5001,"name":"test monitor","query":"q","type":"metric alert","created":"2026-07-21T08:00:00Z","creator":{"email":"test@example.com"}}`)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer httpServer.Close()

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	ddClient := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(testConfig))
	dm := &datadoghqv1alpha1.DatadogMonitor{
		ObjectMeta: metav1.ObjectMeta{
			Name:       resourcesName,
			Namespace:  resourcesNamespace,
			Finalizers: []string{datadogMonitorFinalizer},
		},
		Spec: datadoghqv1alpha1.DatadogMonitorSpec{
			Query:   "not a valid query",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeMetric,
			Name:    "test monitor",
			Message: "load test",
			Tags:    []string{requiredTag},
		},
	}
	kubeClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).WithObjects(dm).Build()
	r := &Reconciler{
		client:        kubeClient,
		datadogClient: ddClient,
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      record.NewFakeRecorder(1),
		log:           logf.Log.WithName("spec-fix-after-permanent-error"),
		requeuePeriod: 24 * time.Hour,
	}

	result, err := r.Reconcile(context.Background(), dm)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: defaultForceSyncPeriod}, result)
	assert.Equal(t, 0, dm.Status.ID, "the monitor must not be considered created while the query is invalid")

	// Simulate the spec fix + watch-triggered reconcile.
	dm.Spec.Query = "avg(last_5m):avg:system.load.1{*} > 100000"
	succeed.Store(true)

	result, err = r.Reconcile(context.Background(), dm)
	assert.NoError(t, err)
	assert.Equal(t, 5001, dm.Status.ID, "the fixed spec must be created on the very next reconcile")
	assert.NotEqual(t, ctrl.Result{RequeueAfter: defaultForceSyncPeriod}, result, "a successful reconcile must not stay on the error backstop cadence")
}

func TestReconcileDatadogMonitor_StatePollingUsesLowPriority(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/monitor/12345" {
			fmt.Fprint(w, `{"id":12345,"name":"test monitor","query":"q","type":"metric alert"}`)
			return
		}
		http.Error(w, "unexpected request", http.StatusNotFound)
	}))
	defer httpServer.Close()

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	ddClient := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(testConfig))

	requeuePeriod := 2 * time.Minute
	now := metav1.Now()
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
		Status: datadoghqv1alpha1.DatadogMonitorStatus{
			ID:                         12345,
			MonitorLastForceSyncTime:   &now,
			MonitorStateLastUpdateTime: &metav1.Time{Time: now.Add(-2 * requeuePeriod)},
		},
	}
	dm.Status.CurrentHash, _ = comparison.GenerateMD5ForSpec(&dm.Spec)

	client := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).WithObjects(dm).Build()
	r := &Reconciler{
		client:        client,
		datadogClient: ddClient,
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      record.NewFakeRecorder(1),
		log:           logf.Log.WithName("state-polling-priority"),
		requeuePeriod: requeuePeriod,
	}

	result, err := r.Reconcile(context.Background(), dm)
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: requeuePeriod, Priority: ptr.To(handler.LowPriority)}, result)
}

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
			wantResult: reconcile.Result{Requeue: true},
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			name: "DatadogMonitor exists, adds required tags before create",
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
				assert.False(t, dm.Status.Primary)
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			name: "DatadogMonitor, error-tracking alert",
			args: args{
				request:             newRequest(resourcesNamespace, resourcesName),
				loadFunc:            testErrorTrackingMonitor,
				firstReconcileCount: 10,
			},
			wantResult: lowPriorityResult(defaultRequeuePeriod),
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
			// Unsupported type: no longer retried on the fast error cadence.
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
			os.Setenv("DD_URL", httpServer.URL)
			os.Setenv("DD_API_KEY", "DUMMY_API_KEY")
			os.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
			defer os.Unsetenv("DD_API_KEY")
			defer os.Unsetenv("DD_APP_KEY")
			defer os.Unsetenv("DD_URL")
			testCredsManager := config.NewCredentialManager(fake.NewClientBuilder().Build())
			r := &Reconciler{
				client:        fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
				datadogClient: client,
				credsManager:  testCredsManager,
				scheme:        s,
				recorder:      recorder,
				log:           logf.Log.WithName(tt.name),
				requeuePeriod: defaultRequeuePeriod,
			}
			_ = testAuth

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

func TestReconcileDatadogMonitor_CredentialRefresh(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogMonitor_Reconcile"})

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})

	// Track the API key seen by the mock Datadog server on each request.
	var lastSeenAPIKey string
	var mu sync.Mutex

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		lastSeenAPIKey = r.Header.Get("Dd-Api-Key")
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	ddClient := datadogV1.NewMonitorsApi(apiClient)

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "api-1")
	t.Setenv("DD_APP_KEY", "app-1")

	credsManager := config.NewCredentialManager(fake.NewClientBuilder().Build())

	r := &Reconciler{
		client:        fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
		datadogClient: ddClient,
		credsManager:  credsManager,
		scheme:        s,
		recorder:      recorder,
		log:           logf.Log.WithName("credential-refresh-test"),
		requeuePeriod: defaultRequeuePeriod,
	}

	dm := genericDatadogMonitor(r.client)

	// Reconcile 3 times: add finalizer → add required tags → create monitor
	for i := 0; i < 3; i++ {
		_, err := r.Reconcile(context.TODO(), dm)
		assert.NoError(t, err)
		// Re-fetch to pick up finalizer/tag updates
		r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)
	}

	// Assert the server saw the initial API key
	mu.Lock()
	assert.Equal(t, "api-1", lastSeenAPIKey, "server should have received initial API key")
	mu.Unlock()

	t.Setenv("DD_API_KEY", "api-2")
	t.Setenv("DD_APP_KEY", "app-2")

	err := credsManager.Refresh(logf.Log)
	assert.NoError(t, err)

	// Trigger another reconcile (force sync by zeroing the last sync time)
	r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)
	dm.Status.MonitorLastForceSyncTime = nil
	r.client.Status().Update(context.TODO(), dm)

	_, err = r.Reconcile(context.TODO(), dm)
	assert.NoError(t, err)

	// Assert the server now sees the rotated API key
	mu.Lock()
	assert.Equal(t, "api-2", lastSeenAPIKey, "server should have received rotated API key after refresh")
	mu.Unlock()
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

func testErrorTrackingMonitor(c client.Client) *datadoghqv1alpha1.DatadogMonitor {
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
			Query:   "error-tracking-query",
			Type:    datadoghqv1alpha1.DatadogMonitorTypeErrorTracking,
			Name:    "test error tracking monitor",
			Message: "something is wrong",
		},
	}
	_ = c.Create(context.TODO(), dm)
	return dm
}

// TestReconcileDatadogMonitor_RecreatesOnOutOfBandDelete reproduces CONS-8333:
// when a monitor is deleted out-of-band (e.g. from the Datadog UI) and the CR
// spec then changes, the controller should detect the 404 and recreate the
// monitor rather than looping on "Monitor not found".
func TestReconcileDatadogMonitor_RecreatesOnOutOfBandDelete(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogMonitor_RecreatesOnOutOfBandDelete"})

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})

	var (
		createCount atomic.Int32
		nextID      atomic.Int64
		monitorGone atomic.Bool
		updateCount atomic.Int32
	)
	nextID.Store(1000)

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case r.Method == http.MethodPost && (path == "/api/v1/monitor/validate" || strings.HasSuffix(path, "/validate")):
			// Validate always succeeds.
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodPost && path == "/api/v1/monitor":
			// Create succeeds and returns a fresh monitor ID.
			createCount.Add(1)
			id := nextID.Add(1)
			fmt.Fprintf(w, `{"id":%d,"name":"test monitor","query":"q","type":"metric alert"}`, id)
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/api/v1/monitor/"):
			if monitorGone.Load() {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"errors":["Monitor not found"]}`)
				return
			}
			fmt.Fprint(w, `{"id":1001,"name":"test monitor","query":"q","type":"metric alert"}`)
		case r.Method == http.MethodPut && strings.HasPrefix(path, "/api/v1/monitor/"):
			updateCount.Add(1)
			if monitorGone.Load() {
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"errors":["Monitor not found"]}`)
				return
			}
			fmt.Fprint(w, `{"id":1001,"name":"test monitor","query":"q","type":"metric alert"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	ddClient := datadogV1.NewMonitorsApi(apiClient)

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	credsManager := config.NewCredentialManager(fake.NewClientBuilder().Build())
	r := &Reconciler{
		client:        fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
		datadogClient: ddClient,
		credsManager:  credsManager,
		scheme:        s,
		recorder:      recorder,
		log:           logf.Log.WithName("recreate-on-404"),
		requeuePeriod: defaultRequeuePeriod,
	}

	dm := genericDatadogMonitor(r.client)

	// Reconcile a few times: add finalizer → add required tags → create monitor.
	for i := 0; i < 3; i++ {
		_, err := r.Reconcile(context.TODO(), dm)
		assert.NoError(t, err)
		r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)
	}

	assert.Equal(t, int32(1), createCount.Load(), "monitor should have been created once")
	assert.NotZero(t, dm.Status.ID, "status.ID should be set after create")
	initialID := dm.Status.ID

	// Simulate out-of-band deletion: subsequent GET/PUT return 404.
	monitorGone.Store(true)

	// Mutate the CR spec so the controller takes the "manifest changed" path.
	dm.Spec.Query = "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.42"
	assert.NoError(t, r.client.Update(context.TODO(), dm))
	r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)

	// Before the fix, the controller would issue a PUT and log a 404 forever.
	// With the fix, the GET-before-update returns 404, so we go down the
	// create path and recreate the monitor. The mock server's create handler
	// is unaffected by monitorGone, so the recreate POST succeeds.
	_, err := r.Reconcile(context.TODO(), dm)
	assert.NoError(t, err)
	r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)

	assert.Equal(t, int32(2), createCount.Load(), "controller should have recreated the monitor after 404")
	assert.NotEqual(t, initialID, dm.Status.ID, "status.ID should be updated to the recreated monitor ID")
	assert.Equal(t, int32(0), updateCount.Load(), "controller should not have issued any updates")
}

// TestReconcileDatadogMonitor_ClearsIDOnUpdate404 reproduces the TOCTOU race
// in CONS-8333: the GET-before-update sees the monitor exists, but it is
// deleted before the PUT lands. The controller should clear status.ID so the
// next reconcile takes the create path instead of looping on 404.
func TestReconcileDatadogMonitor_ClearsIDOnUpdate404(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogMonitor_ClearsIDOnUpdate404"})

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})

	var (
		nextID         atomic.Int64
		failNextUpdate atomic.Bool
	)
	nextID.Store(2000)

	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := r.URL.Path

		switch {
		case r.Method == http.MethodPost && (path == "/api/v1/monitor/validate" || strings.HasSuffix(path, "/validate")):
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodPost && path == "/api/v1/monitor":
			id := nextID.Add(1)
			fmt.Fprintf(w, `{"id":%d,"name":"test monitor","query":"q","type":"metric alert"}`, id)
		case r.Method == http.MethodGet && strings.HasPrefix(path, "/api/v1/monitor/"):
			fmt.Fprint(w, `{"id":2001,"name":"test monitor","query":"q","type":"metric alert"}`)
		case r.Method == http.MethodPut && strings.HasPrefix(path, "/api/v1/monitor/"):
			if failNextUpdate.Load() {
				failNextUpdate.Store(false)
				w.WriteHeader(http.StatusNotFound)
				fmt.Fprint(w, `{"errors":["Monitor not found"]}`)
				return
			}
			fmt.Fprint(w, `{"id":2001,"name":"test monitor","query":"q","type":"metric alert"}`)
		default:
			fmt.Fprint(w, `{}`)
		}
	}))
	defer httpServer.Close()

	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	apiClient := datadogapi.NewAPIClient(testConfig)
	ddClient := datadogV1.NewMonitorsApi(apiClient)

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	credsManager := config.NewCredentialManager(fake.NewClientBuilder().Build())
	rec := &Reconciler{
		client:        fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
		datadogClient: ddClient,
		credsManager:  credsManager,
		scheme:        s,
		recorder:      recorder,
		log:           logf.Log.WithName("clears-id-on-update-404"),
		requeuePeriod: defaultRequeuePeriod,
	}

	dm := genericDatadogMonitor(rec.client)

	// Get the CR through create.
	for i := 0; i < 3; i++ {
		_, err := rec.Reconcile(context.TODO(), dm)
		assert.NoError(t, err)
		rec.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)
	}
	assert.NotZero(t, dm.Status.ID, "status.ID should be set after create")

	// Arm the server to fail the next PUT with 404 (GET still succeeds).
	failNextUpdate.Store(true)

	// Mutate the CR spec so the controller takes the update path.
	dm.Spec.Query = "avg(last_10m):avg:system.disk.in_use{*} by {host} > 0.99"
	assert.NoError(t, rec.client.Update(context.TODO(), dm))
	rec.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)

	_, err := rec.Reconcile(context.TODO(), dm)
	assert.NoError(t, err)
	rec.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, dm)

	assert.Equal(t, 0, dm.Status.ID, "status.ID should be cleared after update returns 404 so next reconcile recreates")
}

// Model a reconcile reading an old cache entry before another status write is
// visible. The fake API server enforces resource versions, so the stale write
// conflicts and the retry reads the newer status.
func TestUpdateStatusIfNeeded_StaleIDConflict(t *testing.T) {
	for _, tt := range []struct {
		name           string
		originalID     int
		latestID       int
		preserveLatest bool
	}{
		{name: "failed stale create preserves newly stored ID", originalID: 0, latestID: 12345, preserveLatest: true},
		{name: "intentional reset of observed ID is allowed", originalID: 12345, latestID: 12345},
		{name: "stale reset preserves replacement ID", originalID: 12345, latestID: 67890, preserveLatest: true},
		{name: "ID-less error status can still be updated", originalID: 0, latestID: 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := scheme.Scheme
			s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
			c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build()
			instance := &datadoghqv1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{Name: "stale-status", Namespace: "default"},
			}
			assert.NoError(t, c.Create(ctx, instance))
			instance.Status.ID = tt.originalID
			assert.NoError(t, c.Status().Update(ctx, instance))
			stale := instance.DeepCopy()

			instance.Status.ID = tt.latestID
			instance.Status.CurrentHash = "latest-successful-hash"
			assert.NoError(t, c.Status().Update(ctx, instance))
			latestStatus := instance.Status.DeepCopy()
			latestVersion := instance.ResourceVersion

			desired := stale.Status.DeepCopy()
			desired.ID = 0
			desired.CurrentHash = "stale-attempt-hash"
			r := &Reconciler{client: c, requeuePeriod: defaultRequeuePeriod}
			scheduled := ctrl.Result{RequeueAfter: time.Minute}
			result, err := r.updateStatusIfNeeded(logf.Log, stale, metav1.Now(), desired, fmt.Errorf("Duplicate of an existing monitor_id"), scheduled)
			assert.NoError(t, err)
			assert.Equal(t, scheduled, result)
			assert.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(instance), instance))
			if tt.preserveLatest {
				assert.Equal(t, tt.latestID, instance.Status.ID)
				assert.True(t, assert.ObjectsAreEqual(*latestStatus, instance.Status), "preserve the entire newer status, including conditions and hash")
				assert.Equal(t, latestVersion, instance.ResourceVersion, "skipping must not perform a status write")
			} else {
				assert.Equal(t, 0, instance.Status.ID)
				assert.Equal(t, desired.CurrentHash, instance.Status.CurrentHash)
			}
			assert.True(t, assert.ObjectsAreEqual(instance.Status, stale.Status), "refresh the caller's status too")
			assert.Equal(t, instance.ResourceVersion, stale.ResourceVersion)
		})
	}
}

func TestReconcileDatadogMonitor_ForceSyncSkipsUpdateWhenRemoteMatches(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	// The remote monitor as Datadog reports it after applying forceSyncMonitor's
	// spec: read-only fields and option defaults are added, the type and
	// message are normalized and the tags are reordered.
	const matchingRemote = `{"id":12345,"name":"test monitor","message":"something is wrong","query":"avg(last_10m):avg:system.cpu.user{*} > 1",` +
		`"type":"query alert","priority":null,"restricted_roles":null,"tags":["team:a","generated:kubernetes"],"overall_state":"OK",` +
		`"options":{"thresholds":{"critical":1},"renotify_interval":30,"notify_audit":false,"include_tags":true,"new_host_delay":300}}`
	driftedRemote := strings.Replace(matchingRemote, `"renotify_interval":30`, `"renotify_interval":120`, 1)

	tests := []struct {
		name       string
		remote     string
		wantWrites int32
	}{
		{name: "remote matches the spec", remote: matchingRemote, wantWrites: 0},
		{name: "remote drifted from the spec", remote: driftedRemote, wantWrites: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var validateCount, updateCount atomic.Int32
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost && r.URL.Path == "/api/v1/monitor/validate":
					validateCount.Add(1)
					fmt.Fprint(w, `{}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/v1/monitor/12345":
					fmt.Fprint(w, tt.remote)
				case r.Method == http.MethodPut && r.URL.Path == "/api/v1/monitor/12345":
					updateCount.Add(1)
					fmt.Fprint(w, matchingRemote)
				default:
					http.Error(w, "unexpected request", http.StatusNotFound)
				}
			}))
			defer httpServer.Close()

			t.Setenv("DD_URL", httpServer.URL)
			t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
			t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

			s := scheme.Scheme
			s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			ddClient := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(testConfig))

			dm := forceSyncMonitor()
			lastForceSync := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod).Truncate(time.Second))
			dm.Status.MonitorLastForceSyncTime = &lastForceSync

			c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).WithObjects(dm).Build()
			r := &Reconciler{
				client:        c,
				datadogClient: ddClient,
				credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
				scheme:        s,
				recorder:      record.NewFakeRecorder(10),
				log:           logf.Log.WithName(tt.name),
				requeuePeriod: defaultRequeuePeriod,
			}

			_, err := r.Reconcile(context.Background(), dm)
			assert.NoError(t, err)
			assert.Equal(t, tt.wantWrites, validateCount.Load(), "validate requests")
			assert.Equal(t, tt.wantWrites, updateCount.Load(), "update requests")

			assert.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dm), dm))
			assert.True(t, dm.Status.MonitorLastForceSyncTime.After(lastForceSync.Time), "a successful force sync must be recorded")
			assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusOK, dm.Status.MonitorStateSyncStatus)
		})
	}
}

func Test_remoteMatchesSpec(t *testing.T) {
	logger := logf.Log.WithName("remote-matches-spec")
	remote := datadogV1.Monitor{}
	assert.NoError(t, json.Unmarshal([]byte(`{"id":12345,"name":"test monitor","message":"something is wrong","query":"avg(last_10m):avg:system.cpu.user{*} > 1",`+
		`"type":"query alert","tags":["team:a"],"options":{"thresholds":{"critical":1},"renotify_interval":30}}`), &remote))

	dm := forceSyncMonitor()
	dm.Spec.Tags = []string{"team:a"}
	assert.False(t, remoteMatchesSpec(logger, dm, remote), "the update path must add the missing required tag")

	dm.Spec.ControllerOptions.DisableRequiredTags = ptr.To(true)
	assert.True(t, remoteMatchesSpec(logger, dm, remote))
}

func forceSyncMonitor() *datadoghqv1alpha1.DatadogMonitor {
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
			Message: "something is wrong\n",
			Tags:    []string{"generated:kubernetes", "team:a"},
			Options: datadoghqv1alpha1.DatadogMonitorOptions{
				Thresholds:       &datadoghqv1alpha1.DatadogMonitorOptionsThresholds{Critical: ptr.To("1")},
				RenotifyInterval: ptr.To(int64(30)),
			},
		},
		Status: datadoghqv1alpha1.DatadogMonitorStatus{
			ID:                         12345,
			MonitorStateLastUpdateTime: ptr.To(metav1.Now()),
		},
	}
	dm.Status.CurrentHash, _ = comparison.GenerateMD5ForSpec(&dm.Spec)
	return dm
}

func batchPolledMonitorJSON(id int, state string) string {
	return fmt.Sprintf(`{"id":%d,"name":"test monitor","query":"avg(last_10m):avg:system.cpu.user{*} > 1","type":"query alert","overall_state":%q,`+
		`"state":{"groups":{"host:a":{"name":"host:a","status":%q,"last_triggered_ts":1700000000}}}}`, id, state, state)
}

func batchPolledDatadogMonitor(name string, id int) *datadoghqv1alpha1.DatadogMonitor {
	dm := forceSyncMonitor()
	dm.Name = name
	dm.Status.ID = id
	dm.Status.MonitorLastForceSyncTime = ptr.To(metav1.Now())
	dm.Status.MonitorStateLastUpdateTime = nil
	return dm
}

func TestReconcileDatadogMonitor_BatchStatePolling(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	var listRequests, getRequests, createRequests atomic.Int32
	var listedIDs atomic.Value
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/monitor":
			// Monitor 103 was deleted out of band; 104 replaces it.
			listRequests.Add(1)
			listedIDs.Store(r.URL.Query().Get("monitor_ids"))
			states := map[string]string{"101": "Alert", "102": "OK", "104": "OK"}
			var monitors []string
			for _, id := range strings.Split(r.URL.Query().Get("monitor_ids"), ",") {
				if state, ok := states[id]; ok {
					monitorID, _ := strconv.Atoi(id)
					monitors = append(monitors, batchPolledMonitorJSON(monitorID, state))
				}
			}
			fmt.Fprint(w, "["+strings.Join(monitors, ",")+"]")
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/monitor/103":
			getRequests.Add(1)
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"errors":["Monitor not found"]}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/monitor/validate":
			fmt.Fprint(w, `{}`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/monitor":
			createRequests.Add(1)
			fmt.Fprint(w, `{"id":104,"name":"test monitor","query":"q","type":"metric alert","created":"2026-07-21T08:00:00Z","creator":{"email":"test@example.com"}}`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer httpServer.Close()

	t.Setenv("DD_URL", httpServer.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	testConfig := datadogapi.NewConfiguration()
	testConfig.HTTPClient = httpServer.Client()
	ddClient := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(testConfig))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller := monitorpoller.New(ddClient, logf.Log.WithName("monitor-state-poller"), monitorpoller.Options{Interval: time.Hour, NewMonitorDelay: time.Millisecond})
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer queue.ShutDown()

	alerting := batchPolledDatadogMonitor("alerting", 101)
	ok := batchPolledDatadogMonitor("ok", 102)
	deleted := batchPolledDatadogMonitor("deleted", 103)
	c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).WithObjects(alerting, ok, deleted).Build()
	r := &Reconciler{
		client:        c,
		datadogClient: ddClient,
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      record.NewFakeRecorder(10),
		log:           logf.Log.WithName("batch-state-polling"),
		requeuePeriod: defaultRequeuePeriod,
		statePoller:   poller,
	}
	require.NoError(t, r.StateSource().Start(ctx, queue))

	reconcileMonitor := func(dm *datadoghqv1alpha1.DatadogMonitor) reconcile.Result {
		t.Helper()
		require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dm), dm))
		result, err := r.Reconcile(ctx, dm)
		require.NoError(t, err)
		require.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(dm), dm))
		return result
	}

	// Reconciling registers the monitors with the poller. Their state is not
	// fetched individually, and only the force sync needs scheduling.
	for _, dm := range []*datadoghqv1alpha1.DatadogMonitor{alerting, ok, deleted} {
		result := reconcileMonitor(dm)
		assert.Greater(t, result.RequeueAfter, 59*time.Minute)
		assert.Nil(t, dm.Status.MonitorStateLastUpdateTime)
	}

	go func() { _ = poller.Start(ctx) }()
	require.Eventually(t, func() bool { return queue.Len() == 3 }, 5*time.Second, 10*time.Millisecond, "every DatadogMonitor is enqueued once its monitor is polled")
	assert.Equal(t, int32(1), listRequests.Load(), "the monitors are listed with a single request")
	assert.ElementsMatch(t, []string{"101", "102", "103"}, strings.Split(listedIDs.Load().(string), ","))
	assert.Equal(t, int32(1), getRequests.Load(), "only the monitor missing from the list is fetched individually")

	reconcileMonitor(alerting)
	assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateAlert, alerting.Status.MonitorState)
	assert.Equal(t, []string{"host:a"}, []string{alerting.Status.TriggeredState[0].MonitorGroup})
	assert.NotNil(t, alerting.Status.MonitorStateLastUpdateTime)
	reconcileMonitor(ok)
	assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateOK, ok.Status.MonitorState)
	assert.Empty(t, ok.Status.TriggeredState)

	// Without a new poll, reconciling again leaves the status unchanged.
	lastStateUpdate := *alerting.Status.MonitorStateLastUpdateTime
	resourceVersion := alerting.ResourceVersion
	reconcileMonitor(alerting)
	assert.Equal(t, lastStateUpdate, *alerting.Status.MonitorStateLastUpdateTime)
	assert.Equal(t, resourceVersion, alerting.ResourceVersion)

	// A monitor confirmed missing is recreated, and the poller follows the new ID.
	reconcileMonitor(deleted)
	assert.Equal(t, int32(1), createRequests.Load())
	assert.Equal(t, 104, deleted.Status.ID)
	auth, err := r.credsManager.GetAuth()
	require.NoError(t, err)
	_, polled := poller.Result(auth, 103)
	assert.False(t, polled, "the deleted monitor is no longer polled")
	require.Eventually(t, func() bool { _, polled = poller.Result(auth, 104); return polled }, 5*time.Second, 10*time.Millisecond)
	assert.Equal(t, int32(2), listRequests.Load(), "the new monitor is polled on its own, ahead of the next full poll")
}
