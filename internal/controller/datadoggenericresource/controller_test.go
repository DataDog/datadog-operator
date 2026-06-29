package datadoggenericresource

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/kubectl/pkg/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
)

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
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
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
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
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
