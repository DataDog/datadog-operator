package datadoggenericresource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"sync/atomic"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/client-go/util/workqueue"
	"k8s.io/kubectl/pkg/scheme"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/monitorpoller"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
)

type conflictOnceClient struct {
	client.Client
	remainingConflicts int
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
	if w.client.remainingConflicts > 0 {
		w.client.remainingConflicts--

		latest := &datadoghqv1alpha1.DatadogGenericResource{}
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
			datadoghqv1alpha1.GroupVersion.WithResource("datadoggenericresources").GroupResource(),
			obj.GetName(),
			fmt.Errorf("injected status update conflict"),
		)
	}
	return w.SubResourceWriter.Update(ctx, obj, opts...)
}

func TestReconcileGenericResource_RetriesCreatedStatusAfterConflict(t *testing.T) {
	resetMockHandlerState()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(s, corev1.EventSource{Component: "TestReconcileGenericResource_RetriesCreatedStatusAfterConflict"})

	baseClient := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build()
	conflictingClient := &conflictOnceClient{Client: baseClient}
	r := NewReconciler(
		conflictingClient,
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logf.Log.WithName("created-status-conflict"),
		recorder,
	)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
		mockSubresource: &MockHandler{},
	}

	instance := mockGenericResource()
	instance.Finalizers = []string{datadogGenericResourceFinalizer}
	assert.NoError(t, r.client.Create(context.TODO(), instance))

	conflictingClient.remainingConflicts = 1
	result, err := reconcileRequest(r, context.TODO(), newRequest(resourcesNamespace, resourcesName))
	assert.NoError(t, err)
	assert.Equal(t, ctrl.Result{RequeueAfter: defaultRequeuePeriod}, result)
	assert.Equal(t, 0, conflictingClient.remainingConflicts, "the injected status conflict should be consumed")

	assert.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(instance), instance))
	assert.Equal(t, mockResourceID, instance.Status.Id, "the ID returned by the successful create must be persisted")
	assert.Equal(t, "true", instance.Annotations["issue-3272-conflict"], "the concurrent metadata update must be preserved")

	_, err = reconcileRequest(r, context.TODO(), newRequest(resourcesNamespace, resourcesName))
	assert.NoError(t, err)
	assert.Equal(t, 1, mockCreateCalls, "a status conflict must not cause a second Datadog create")
}

func TestReconcileGenericResource_Reconcile(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileGenericResource_Reconcile"})

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})

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
			name: "DatadogGenericResource not created",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
			},
			wantResult: reconcile.Result{},
		},
		{
			name: "DatadogGenericResource create backend error keeps normal priority",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), mockGenericResource())
					mockCreateErr = fmt.Errorf("error creating mock resource: 429 Too Many Requests")
				},
				// reconcile 1: add finalizer; 2: create remote resource and fail
				firstReconcileCount: 2,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultErrRequeuePeriod},
			wantFunc: func(c client.Client) error {
				assert.Equal(t, 1, mockCreateCalls)
				obj := &datadoghqv1alpha1.DatadogGenericResource{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj); err != nil {
					return err
				}
				assert.Empty(t, obj.Status.Id)
				return nil
			},
		},
		{
			name: "DatadogGenericResource created, add finalizer",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), mockGenericResource())
				},
			},
			wantResult: reconcile.Result{Requeue: true},
			wantFunc: func(c client.Client) error {
				obj := &datadoghqv1alpha1.DatadogGenericResource{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj); err != nil {
					return err
				}
				assert.Contains(t, obj.GetFinalizers(), "finalizer.datadoghq.com/genericresource")
				return nil
			},
		},
		{
			name: "DatadogGenericResource exists, needs update",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), mockGenericResource())
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					_ = c.Update(context.TODO(), &datadoghqv1alpha1.DatadogGenericResource{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogGenericResource",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: resourcesNamespace,
							Name:      resourcesName,
						},
						Spec: datadoghqv1alpha1.DatadogGenericResourceSpec{
							Type:     mockSubresource,
							JsonSpec: "{\"bar\": \"baz\"}",
						},
					})
				},
				secondReconcileCount: 2,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantFunc: func(c client.Client) error {
				obj := &datadoghqv1alpha1.DatadogGenericResource{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj); err != nil {
					return err
				}
				// Make sure status hash is up to date
				hash, _ := comparison.GenerateMD5ForSpec(obj.Spec)
				assert.Equal(t, obj.Status.CurrentHash, hash)
				return nil
			},
		},
		{
			name: "DatadogGenericResource recreates on update when remote resource is missing",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), mockGenericResource())
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					mockResourceID = "mock-id-recreated"
					mockUpdateErr = fmt.Errorf("error updating mock resource: 404 Not Found")
					obj := &datadoghqv1alpha1.DatadogGenericResource{}
					err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj)
					assert.NoError(t, err)
					obj.Spec.JsonSpec = "{\"bar\": \"baz\"}"
					err = c.Update(context.TODO(), obj)
					assert.NoError(t, err)
				},
				secondReconcileCount: 1,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantFunc: func(c client.Client) error {
				obj := &datadoghqv1alpha1.DatadogGenericResource{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj); err != nil {
					return err
				}
				hash, _ := comparison.GenerateMD5ForSpec(obj.Spec)
				// Recreating the Datadog resource yields a fresh remote ID, so we only
				// assert that the stored ID changed from the original one.
				assert.NotEqual(t, "mock-id", obj.Status.Id)
				assert.NotEmpty(t, obj.Status.Id)
				assert.Equal(t, hash, obj.Status.CurrentHash)
				assert.Equal(t, 2, mockCreateCalls)
				return nil
			},
		},
		{
			name: "DatadogGenericResource exists, idle tick refreshes Datadog state",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), mockGenericResource())
					alert := "Alert"
					mockRefreshStateResult = &alert
				},
				// reconcile 1: add finalizer; 2: create remote resource; 3: idle tick triggers refresh
				firstReconcileCount: 3,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod, Priority: ptr.To(handler.LowPriority)},
			wantFunc: func(c client.Client) error {
				obj := &datadoghqv1alpha1.DatadogGenericResource{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj); err != nil {
					return err
				}
				assert.Equal(t, "Alert", obj.Status.State, "expected state to be refreshed to Alert")
				assert.NotNil(t, obj.Status.StateLastUpdateTime, "expected StateLastUpdateTime to be set")
				assert.NotNil(t, obj.Status.StateLastTransitionTime, "expected StateLastTransitionTime to be set on first refresh")
				assert.GreaterOrEqual(t, mockRefreshStateCalls, 1, "expected refreshState to have been called")
				// StateSynced condition should be True
				var found bool
				for _, cond := range obj.Status.Conditions {
					if cond.Type == "StateSynced" {
						found = true
						assert.Equal(t, metav1.ConditionTrue, cond.Status, "expected StateSynced=True after a successful refresh")
					}
				}
				assert.True(t, found, "expected a StateSynced condition")
				return nil
			},
		},
		{
			name: "DatadogGenericResource exists, state refresh failure preserves last-known state",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), mockGenericResource())
					alert := "Alert"
					mockRefreshStateResult = &alert
				},
				firstReconcileCount: 3,
				secondAction: func(c client.Client) {
					// Backdate StateLastUpdateTime so the next reconcile re-enters
					// the refresh branch, and switch the mock to fail.
					obj := &datadoghqv1alpha1.DatadogGenericResource{}
					_ = c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj)
					past := metav1.NewTime(time.Now().Add(-2 * defaultRequeuePeriod))
					obj.Status.StateLastUpdateTime = &past
					_ = c.Status().Update(context.TODO(), obj)
					mockRefreshStateErr = fmt.Errorf("error getting mock resource: 503 Service Unavailable")
					mockRefreshStateResult = nil
				},
				secondReconcileCount: 1,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod, Priority: ptr.To(handler.LowPriority)},
			wantFunc: func(c client.Client) error {
				obj := &datadoghqv1alpha1.DatadogGenericResource{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj); err != nil {
					return err
				}
				// Last-known state from first refresh should be preserved.
				assert.Equal(t, "Alert", obj.Status.State, "expected last-known state to be preserved on refresh failure")
				// StateSynced condition flips to False with Reason=GetError
				var found bool
				for _, cond := range obj.Status.Conditions {
					if cond.Type == "StateSynced" {
						found = true
						assert.Equal(t, metav1.ConditionFalse, cond.Status, "expected StateSynced=False on refresh failure")
						assert.Equal(t, "GetError", cond.Reason, "expected Reason=GetError on refresh failure")
					}
				}
				assert.True(t, found, "expected a StateSynced condition after a failure")
				return nil
			},
		},
		{
			name: "DatadogGenericResource exists, needs delete",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					err := c.Create(context.TODO(), mockGenericResource())
					assert.NoError(t, err)
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					err := c.Delete(context.TODO(), mockGenericResource())
					assert.NoError(t, err)
				},
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    true,
			wantFunc: func(c client.Client) error {
				obj := &datadoghqv1alpha1.DatadogGenericResource{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj); err != nil {
					return err
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetMockHandlerState()

			os.Setenv("DD_API_KEY", "DUMMY_API_KEY")
			os.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
			defer os.Unsetenv("DD_API_KEY")
			defer os.Unsetenv("DD_APP_KEY")

			// Use mock handlers so tests don't hit real APIs.
			r := NewReconciler(
				fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
				config.NewCredentialManager(fake.NewClientBuilder().Build()),
				s,
				logf.Log.WithName(tt.name),
				recorder,
			)
			r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
				mockSubresource: &MockHandler{},
			}

			// First action
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
				result, err = reconcileRequest(r, context.TODO(), tt.args.request)
			}

			assert.NoError(t, err, "unexpected error: %v", err)
			assert.Equal(t, tt.wantResult, result, "unexpected result")

			// Second action
			if tt.args.secondAction != nil {
				tt.args.secondAction(r.client)
				// Make sure there's minimum 1 reconcile loop
				if tt.args.secondReconcileCount == 0 {
					tt.args.secondReconcileCount = 1
				}
			}
			for i := 0; i < tt.args.secondReconcileCount; i++ {
				_, err := reconcileRequest(r, context.TODO(), tt.args.request)
				assert.NoError(t, err, "unexpected error: %v", err)
			}

			if tt.wantFunc != nil {
				err := tt.wantFunc(r.client)
				if tt.wantErr {
					assert.Error(t, err, "expected an error")
				} else {
					assert.NoError(t, err, "wantFunc validation error: %v", err)
				}
			}
		})

	}
}

func TestForceSyncPeriodFromEnv(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName("force-sync-period-test")

	tests := []struct {
		name     string
		envValue string
		want     time.Duration
	}{
		{
			name:     "valid value",
			envValue: "1",
			want:     time.Minute,
		},
		{
			name:     "invalid string falls back to default",
			envValue: "abc",
			want:     defaultForceSyncPeriod,
		},
		{
			name:     "zero falls back to default",
			envValue: "0",
			want:     defaultForceSyncPeriod,
		},
		{
			name:     "negative value falls back to default",
			envValue: "-1",
			want:     defaultForceSyncPeriod,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv(ddGenericResourceForceSyncPeriodEnvVar, tt.envValue)

			assert.Equal(t, tt.want, forceSyncPeriodFromEnv(logger))
		})
	}
}

func TestRequeuePeriodDefault(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName("requeue-period-test")

	assert.Equal(t, defaultRequeuePeriod, requeuePeriod(logger, 0))
}

func TestRequeuePeriodConfiguredOption(t *testing.T) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName("requeue-period-test")

	assert.Equal(t, 2*time.Minute, requeuePeriod(logger, 2*time.Minute))
}

func TestReconcileGenericResource_ForceSyncPeriodTriggersRemoteUpdate(t *testing.T) {
	resetMockHandlerState()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
	t.Setenv(ddGenericResourceForceSyncPeriodEnvVar, "1")

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileGenericResource_ForceSyncPeriodTriggersRemoteUpdate"})

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName("force-sync-period-test")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})

	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logger,
		recorder,
	)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
		mockSubresource: &MockHandler{},
	}

	err := r.client.Create(context.TODO(), mockGenericResource())
	assert.NoError(t, err)

	req := newRequest(resourcesNamespace, resourcesName)

	// Add finalizer, then create the Datadog resource.
	result, err := reconcileRequest(r, context.TODO(), req)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{Requeue: true}, result)

	result, err = reconcileRequest(r, context.TODO(), req)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{RequeueAfter: defaultRequeuePeriod}, result)
	assert.Equal(t, 1, mockCreateCalls)
	assert.Equal(t, 0, mockGetCalls)
	assert.Equal(t, 0, mockUpdateCalls)

	obj := &datadoghqv1alpha1.DatadogGenericResource{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj)
	assert.NoError(t, err)
	lastForceSyncTime := metav1.NewTime(time.Now().Add(-2 * time.Minute))
	obj.Status.LastForceSyncTime = &lastForceSyncTime
	err = r.client.Status().Update(context.TODO(), obj)
	assert.NoError(t, err)

	// Once the configured force-sync period has elapsed, the next reconcile
	// should force a remote get and update even though the spec hash is unchanged.
	result, err = reconcileRequest(r, context.TODO(), req)
	assert.NoError(t, err)
	assert.Equal(t, reconcile.Result{RequeueAfter: defaultRequeuePeriod}, result)
	assert.Equal(t, 1, mockGetCalls)
	assert.Equal(t, 1, mockUpdateCalls)
}

func TestReconcileGenericResource_ForceSyncComparison(t *testing.T) {
	tests := []struct {
		name              string
		handler           ResourceHandler
		remoteMatchesSpec bool
		wantCompareCalls  int
		wantGetCalls      int
		wantUpdateCalls   int
	}{
		{
			name:              "opted-in handler skips the update when the remote matches",
			handler:           &MockComparingHandler{},
			remoteMatchesSpec: true,
			wantCompareCalls:  1,
		},
		{
			name:             "opted-in handler updates a drifted remote",
			handler:          &MockComparingHandler{},
			wantCompareCalls: 1,
			wantUpdateCalls:  1,
		},
		{
			name:              "other handlers always update",
			handler:           &MockHandler{},
			remoteMatchesSpec: true,
			wantGetCalls:      1,
			wantUpdateCalls:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resetMockHandlerState()
			t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
			t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
			logf.SetLogger(zap.New(zap.UseDevMode(true)))

			s := scheme.Scheme
			s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})

			instance := mockGenericResource()
			instance.Finalizers = []string{datadogGenericResourceFinalizer}
			instance.Status.Id = mockResourceID
			instance.Status.CurrentHash, _ = comparison.GenerateMD5ForSpec(&instance.Spec)
			instance.Status.SyncStatus = datadoghqv1alpha1.DatadogSyncStatusGetError
			staleForceSync := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod).Truncate(time.Second))
			instance.Status.LastForceSyncTime = &staleForceSync

			r := NewReconciler(
				fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).WithObjects(instance).Build(),
				config.NewCredentialManager(fake.NewClientBuilder().Build()),
				s,
				logf.Log.WithName(tt.name),
				record.NewFakeRecorder(10),
			)
			r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
				mockSubresource: tt.handler,
			}
			mockRemoteMatchesSpec = tt.remoteMatchesSpec

			result, err := reconcileRequest(r, context.TODO(), newRequest(resourcesNamespace, resourcesName))
			assert.NoError(t, err)
			assert.Equal(t, reconcile.Result{RequeueAfter: defaultRequeuePeriod}, result)
			assert.Equal(t, tt.wantCompareCalls, mockCompareCalls)
			assert.Equal(t, tt.wantGetCalls, mockGetCalls)
			assert.Equal(t, tt.wantUpdateCalls, mockUpdateCalls)

			obj := &datadoghqv1alpha1.DatadogGenericResource{}
			assert.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(instance), obj))
			assert.True(t, obj.Status.LastForceSyncTime.After(staleForceSync.Time), "a successful force sync must be recorded")
			assert.Equal(t, datadoghqv1alpha1.DatadogSyncStatusOK, obj.Status.SyncStatus)
		})
	}
}

func TestReconcileGenericResource_ForceSyncComparisonNotFoundRecreates(t *testing.T) {
	resetMockHandlerState()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})

	instance := mockGenericResource()
	instance.Finalizers = []string{datadogGenericResourceFinalizer}
	instance.Status.Id = "deleted-id"
	instance.Status.CurrentHash, _ = comparison.GenerateMD5ForSpec(&instance.Spec)

	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).WithObjects(instance).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logf.Log.WithName("force-sync-comparison-not-found"),
		record.NewFakeRecorder(10),
	)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
		mockSubresource: &MockComparingHandler{},
	}
	mockGetErr = fmt.Errorf("error getting resource: 404 Not Found")

	_, err := reconcileRequest(r, context.TODO(), newRequest(resourcesNamespace, resourcesName))
	assert.NoError(t, err)
	assert.Equal(t, 1, mockCompareCalls)
	assert.Equal(t, 1, mockCreateCalls, "a remote deleted out of band must be recreated")
	assert.Equal(t, 0, mockUpdateCalls)
}

func TestReconcileGenericResource_BatchStatePolling(t *testing.T) {
	resetMockHandlerState()
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	var listRequests, getRequests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/monitor":
			listRequests.Add(1)
			assert.Equal(t, "12345", r.URL.Query().Get("monitor_ids"))
			fmt.Fprint(w, `[{"id":12345,"name":"High CPU","type":"query alert","query":"q","overall_state":"Alert"}]`)
		default:
			getRequests.Add(1)
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	t.Setenv("DD_URL", server.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	ddClient := datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	poller := monitorpoller.New(ddClient, logf.Log.WithName("monitor-state-poller"), monitorpoller.Options{Interval: time.Hour, NewMonitorDelay: time.Millisecond})
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer queue.ShutDown()

	// The monitor handler opts in to batch state polling; the mock handler does not.
	monitor := mockGenericResource()
	monitor.Name = "monitor"
	monitor.Spec.Type = datadoghqv1alpha1.Monitor
	other := mockGenericResource()
	other.Name = "other"
	for _, instance := range []*datadoghqv1alpha1.DatadogGenericResource{monitor, other} {
		instance.Finalizers = []string{datadogGenericResourceFinalizer}
		instance.Status.CurrentHash, _ = comparison.GenerateMD5ForSpec(&instance.Spec)
		instance.Status.LastForceSyncTime = ptr.To(metav1.Now())
	}
	monitor.Status.Id = "12345"
	other.Status.Id = mockResourceID
	mockRefreshStateResult = ptr.To("mock-state")

	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).WithObjects(monitor, other).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logf.Log.WithName("batch-state-polling"),
		record.NewFakeRecorder(10),
		ReconcilerOptions{StatePoller: poller},
	)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
		datadoghqv1alpha1.Monitor: &MonitorHandler{client: ddClient},
		mockSubresource:           &MockHandler{},
	}
	require.NoError(t, r.StateSource().Start(ctx, queue))

	reconcileResource := func(instance *datadoghqv1alpha1.DatadogGenericResource) reconcile.Result {
		t.Helper()
		result, err := reconcileRequest(r, ctx, newRequest(resourcesNamespace, instance.Name))
		require.NoError(t, err)
		require.NoError(t, r.client.Get(ctx, client.ObjectKeyFromObject(instance), instance))
		return result
	}

	result := reconcileResource(monitor)
	assert.Greater(t, result.RequeueAfter, 59*time.Minute, "only the force sync needs scheduling")
	assert.Nil(t, monitor.Status.StateLastUpdateTime)
	result = reconcileResource(other)
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)
	assert.Equal(t, 1, mockRefreshStateCalls, "handlers that do not opt in keep refreshing their own state")
	assert.Equal(t, "mock-state", other.Status.State)

	go func() { _ = poller.Start(ctx) }()
	require.Eventually(t, func() bool { return queue.Len() == 1 }, 5*time.Second, 10*time.Millisecond)
	request, _ := queue.Get()
	assert.Equal(t, "monitor", request.Name)

	reconcileResource(monitor)
	assert.Equal(t, int32(1), listRequests.Load())
	assert.Equal(t, int32(0), getRequests.Load())
	assert.Equal(t, string(datadoghqv1alpha1.DatadogMonitorStateAlert), monitor.Status.State)
	assert.NotNil(t, monitor.Status.StateLastUpdateTime)
	stateSynced := meta.FindStatusCondition(monitor.Status.Conditions, string(condition.DatadogConditionTypeStateSynced))
	require.NotNil(t, stateSynced)
	assert.Equal(t, metav1.ConditionTrue, stateSynced.Status)
}

func TestReconcileGenericResource_ForceSyncFailureDoesNotDeferRetry(t *testing.T) {
	resetMockHandlerState()
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
	t.Setenv(ddGenericResourceForceSyncPeriodEnvVar, "1")

	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileGenericResource_ForceSyncFailureDoesNotDeferRetry"})

	logf.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := logf.Log.WithName("force-sync-failure-test")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})

	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logger,
		recorder,
	)
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
		mockSubresource: &MockHandler{},
	}

	err := r.client.Create(context.TODO(), mockGenericResource())
	assert.NoError(t, err)

	req := newRequest(resourcesNamespace, resourcesName)

	// Add finalizer, then create the Datadog resource.
	_, err = reconcileRequest(r, context.TODO(), req)
	assert.NoError(t, err)
	_, err = reconcileRequest(r, context.TODO(), req)
	assert.NoError(t, err)
	assert.Equal(t, 1, mockCreateCalls)

	obj := &datadoghqv1alpha1.DatadogGenericResource{}
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj)
	assert.NoError(t, err)
	obj.Status.LastForceSyncTime = ptr.To(metav1.NewTime(time.Now().Add(-2 * time.Minute)))
	err = r.client.Status().Update(context.TODO(), obj)
	assert.NoError(t, err)

	// Read back the round-tripped (second-precision) value as our baseline, since
	// metav1.Time truncates to seconds on serialization.
	err = r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj)
	assert.NoError(t, err)
	staleLastForceSyncTime := *obj.Status.LastForceSyncTime

	// Simulate a transient failure (e.g. a Datadog API timeout) on the force-synced update.
	mockUpdateErr = fmt.Errorf("504 Gateway Timeout: stream timeout")

	result, err := reconcileRequest(r, context.TODO(), req)
	assert.NoError(t, err)
	assert.Equal(t, 1, mockGetCalls)
	assert.Equal(t, 1, mockUpdateCalls)
	assert.Equal(t, ctrl.Result{RequeueAfter: defaultErrRequeuePeriod}, result)

	err = r.client.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, obj)
	assert.NoError(t, err)
	// LastForceSyncTime must not advance on failure: advancing it would make the
	// controller wait out a full forceSyncPeriod before retrying, instead of
	// retrying on the fast error-requeue cadence.
	assert.True(t, obj.Status.LastForceSyncTime.Equal(&staleLastForceSyncTime), "LastForceSyncTime should not advance when the force-synced update fails")

	// The next reconcile should retry the force sync immediately rather than
	// waiting out the (already-elapsed, but now-irrelevant) forceSyncPeriod window.
	mockUpdateErr = nil
	result, err = reconcileRequest(r, context.TODO(), req)
	assert.NoError(t, err)
	assert.Equal(t, 2, mockGetCalls)
	assert.Equal(t, 2, mockUpdateCalls)
	assert.Equal(t, reconcile.Result{RequeueAfter: defaultRequeuePeriod}, result)
}

func newRequest(ns, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      name,
		},
	}
}

// reconcileRequest mirrors reconcile.AsReconciler: fetches the object then calls Reconcile.
// If the object is not found, it returns a zero Result with no error (matching controller-runtime behaviour).
func reconcileRequest(r *Reconciler, ctx context.Context, req reconcile.Request) (ctrl.Result, error) {
	instance := &datadoghqv1alpha1.DatadogGenericResource{}
	if err := r.client.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}
	return r.Reconcile(ctx, instance)
}

func mockGenericResource() *datadoghqv1alpha1.DatadogGenericResource {
	return &datadoghqv1alpha1.DatadogGenericResource{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogGenericResource",
			APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: resourcesNamespace,
			Name:      resourcesName,
		},
		Spec: datadoghqv1alpha1.DatadogGenericResourceSpec{
			Type:     mockSubresource,
			JsonSpec: "{\"foo\": \"bar\"}",
		},
	}
}

func setupTestAuth(apiURL string) context.Context {
	testAuth := context.WithValue(
		context.Background(),
		datadogapi.ContextAPIKeys,
		map[string]datadogapi.APIKey{
			"apiKeyAuth": {
				Key: "DUMMY_API_KEY",
			},
			"appKeyAuth": {
				Key: "DUMMY_APP_KEY",
			},
		},
	)
	parsedAPIURL, _ := url.Parse(apiURL)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerIndex, 1)
	testAuth = context.WithValue(testAuth, datadogapi.ContextServerVariables, map[string]string{
		"name":     parsedAPIURL.Host,
		"protocol": parsedAPIURL.Scheme,
	})

	return testAuth
}

// Model a reconcile reading an old cache entry before another status write is
// visible. The fake API server enforces resource versions, so the stale write
// conflicts and the retry reads the newer status.
func TestUpdateStatusIfNeeded_StaleIDConflict(t *testing.T) {
	for _, tt := range []struct {
		name           string
		originalID     string
		latestID       string
		preserveLatest bool
	}{
		{name: "failed stale create preserves newly stored ID", originalID: "", latestID: "old-id", preserveLatest: true},
		{name: "intentional reset of observed ID is allowed", originalID: "old-id", latestID: "old-id"},
		{name: "stale reset preserves replacement ID", originalID: "old-id", latestID: "new-id", preserveLatest: true},
		{name: "ID-less error status can still be updated", originalID: "", latestID: ""},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := scheme.Scheme
			s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
			c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build()
			instance := &datadoghqv1alpha1.DatadogGenericResource{
				ObjectMeta: metav1.ObjectMeta{Name: "stale-status", Namespace: "default"},
			}
			assert.NoError(t, c.Create(ctx, instance))
			instance.Status.Id = tt.originalID
			assert.NoError(t, c.Status().Update(ctx, instance))
			stale := instance.DeepCopy()

			instance.Status.Id = tt.latestID
			instance.Status.CurrentHash = "latest-successful-hash"
			assert.NoError(t, c.Status().Update(ctx, instance))
			latestStatus := instance.Status.DeepCopy()
			latestVersion := instance.ResourceVersion

			desired := stale.Status.DeepCopy()
			desired.Id = ""
			desired.CurrentHash = "stale-attempt-hash"
			r := &Reconciler{client: c, requeuePeriod: defaultRequeuePeriod}
			scheduled := ctrl.Result{RequeueAfter: time.Minute}
			result, err := r.updateStatusIfNeeded(ctx, stale, desired, scheduled)
			assert.NoError(t, err)
			assert.Equal(t, scheduled, result)
			assert.NoError(t, c.Get(ctx, client.ObjectKeyFromObject(instance), instance))
			if tt.preserveLatest {
				assert.Equal(t, tt.latestID, instance.Status.Id)
				assert.True(t, assert.ObjectsAreEqual(*latestStatus, instance.Status), "preserve the entire newer status, including conditions and hash")
				assert.Equal(t, latestVersion, instance.ResourceVersion, "skipping must not perform a status write")
			} else {
				assert.Equal(t, "", instance.Status.Id)
				assert.Equal(t, desired.CurrentHash, instance.Status.CurrentHash)
			}
			assert.True(t, assert.ObjectsAreEqual(instance.Status, stale.Status), "refresh the caller's status too")
			assert.Equal(t, instance.ResourceVersion, stale.ResourceVersion)
		})
	}
}
