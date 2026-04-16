package datadogdashboard

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	corev1 "k8s.io/api/core/v1"
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

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/stretchr/testify/assert"
)

const (
	resourcesName      = "foo"
	resourcesNamespace = "bar"
)

func TestReconcileDatadogDashboard_Reconcile(t *testing.T) {
	eventBroadcaster := record.NewBroadcaster()
	recorder := eventBroadcaster.NewRecorder(scheme.Scheme, corev1.EventSource{Component: "TestReconcileDatadogDashboard_Reconcile"})

	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogDashboard{})

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
			name: "DatadogDashboard not created",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
			},
			wantResult: reconcile.Result{},
		},
		{
			name: "DatadogDashboard created, add finalizer",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), genericDatadogDashboard())
				},
			},
			wantResult: reconcile.Result{Requeue: true},
			wantFunc: func(c client.Client) error {
				db := &datadoghqv1alpha1.DatadogDashboard{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, db); err != nil {
					return err
				}
				assert.Contains(t, db.GetFinalizers(), "finalizer.datadoghq.com/dashboard")
				return nil
			},
		},
		// NOTE: omitted, re-enable check tags test once 'generated:kubernetes' is allowed in
		// {
		// 	name: "DatadogDashboard exists, check required tags",
		// 	args: args{
		// 		request: newRequest(resourcesNamespace, resourcesName),
		// 		firstAction: func(c client.Client) {
		// 			_ = c.Create(context.TODO(), genericDatadogDashboard())
		// 		},
		// 		firstReconcileCount: 2,
		// 	},
		// 	wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
		// 	wantFunc: func(c client.Client) error {
		// 		db := &datadoghqv1alpha1.DatadogDashboard{}
		// 		if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, db); err != nil {
		// 			return err
		// 		}
		// 		assert.Contains(t, db.Spec.Tags, "generated:kubernetes")
		// 		return nil
		// 	},
		// },
		{
			name: "DatadogDashboard exists, needs update",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					_ = c.Create(context.TODO(), genericDatadogDashboard())
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					_ = c.Update(context.TODO(), &datadoghqv1alpha1.DatadogDashboard{
						TypeMeta: metav1.TypeMeta{
							Kind:       "DatadogDashboard",
							APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
						},
						ObjectMeta: metav1.ObjectMeta{
							Namespace: resourcesNamespace,
							Name:      resourcesName,
						},
						Spec: datadoghqv1alpha1.DatadogDashboardSpec{
							// Update layout Type
							Title:      "test dashboard",
							LayoutType: "free",
						},
					})
				},
				secondReconcileCount: 2,
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantFunc: func(c client.Client) error {
				db := &datadoghqv1alpha1.DatadogDashboard{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, db); err != nil {
					return err
				}
				// Make sure status hash is up to date
				hash, _ := comparison.GenerateMD5ForSpec(db.Spec)
				assert.Equal(t, db.Status.CurrentHash, hash)
				return nil
			},
		},
		{
			name: "DatadogDashboard exists, needs delete",
			args: args{
				request: newRequest(resourcesNamespace, resourcesName),
				firstAction: func(c client.Client) {
					err := c.Create(context.TODO(), genericDatadogDashboard())
					assert.NoError(t, err)
				},
				firstReconcileCount: 2,
				secondAction: func(c client.Client) {
					err := c.Delete(context.TODO(), genericDatadogDashboard())
					assert.NoError(t, err)
				},
			},
			wantResult: reconcile.Result{RequeueAfter: defaultRequeuePeriod},
			wantErr:    true,
			wantFunc: func(c client.Client) error {
				db := &datadoghqv1alpha1.DatadogDashboard{}
				if err := c.Get(context.TODO(), types.NamespacedName{Name: resourcesName, Namespace: resourcesNamespace}, db); err != nil {
					return err
				}
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
			client := datadogV1.NewDashboardsApi(apiClient)

			testAuth := setupTestAuth(httpServer.URL)

			// Set up
			r := &Reconciler{
				client:        fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogDashboard{}).Build(),
				datadogClient: client,
				datadogAuth:   testAuth,
				scheme:        s,
				recorder:      recorder,
				log:           logf.Log.WithName(tt.name),
			}

			// First dashboard action
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

			assert.NoError(t, err, "ReconcileDatadogDashboard.Reconcile() unexpected error: %v", err)
			assert.Equal(t, tt.wantResult, result, "ReconcileDatadogDashboard.Reconcile() unexpected result")

			// Second dashboard action
			if tt.args.secondAction != nil {
				tt.args.secondAction(r.client)
				// Make sure there's minimum 1 reconcile loop
				if tt.args.secondReconcileCount == 0 {
					tt.args.secondReconcileCount = 1
				}
			}
			for i := 0; i < tt.args.secondReconcileCount; i++ {
				_, err := r.Reconcile(context.TODO(), tt.args.request)
				assert.NoError(t, err, "ReconcileDatadogDashboard.Reconcile() unexpected error: %v", err)
			}

			if tt.wantFunc != nil {
				err := tt.wantFunc(r.client)
				if tt.wantErr {
					assert.Error(t, err, "ReconcileDatadogDashboard.Reconcile() expected an error")
				} else {
					assert.NoError(t, err, "ReconcileDatadogDashboard.Reconcile() wantFunc validation error: %v", err)
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

func genericDatadogDashboard() *datadoghqv1alpha1.DatadogDashboard {
	return &datadoghqv1alpha1.DatadogDashboard{
		TypeMeta: metav1.TypeMeta{
			Kind:       "DatadogDashboard",
			APIVersion: fmt.Sprintf("%s/%s", datadoghqv1alpha1.GroupVersion.Group, datadoghqv1alpha1.GroupVersion.Version),
		},
		ObjectMeta: metav1.ObjectMeta{
			Namespace: resourcesNamespace,
			Name:      resourcesName,
		},
		Spec: datadoghqv1alpha1.DatadogDashboardSpec{
			Title:      "test dashboard",
			LayoutType: "ordered",
		},
	}
}

// TestReconciler_UpdateDatadogClient tests the UpdateDatadogClient method of the Reconciler
func TestReconciler_UpdateDatadogClient(t *testing.T) {
	testLogger := zap.New(zap.UseDevMode(true))
	recorder := record.NewFakeRecorder(10)
	scheme := scheme.Scheme
	client := fake.NewClientBuilder().Build()

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
			r, err := NewReconciler(client, initialCreds, scheme, testLogger, recorder)
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
