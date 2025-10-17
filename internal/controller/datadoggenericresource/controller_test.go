package datadoggenericresource

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"testing"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
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
			httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
			}))
			defer httpServer.Close()

			testConfig := datadogapi.NewConfiguration()
			testConfig.HTTPClient = httpServer.Client()
			apiClient := datadogapi.NewAPIClient(testConfig)
			synthClient := datadogV1.NewSyntheticsApi(apiClient)
			nbClient := datadogV1.NewNotebooksApi(apiClient)

			testAuth := setupTestAuth(httpServer.URL)

			// Set up
			r := &Reconciler{
				client:                  fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
				datadogSyntheticsClient: synthClient,
				datadogNotebooksClient:  nbClient,
				datadogAuth:             testAuth,
				scheme:                  s,
				recorder:                recorder,
				log:                     logf.Log.WithName(tt.name),
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
				result, err = r.Reconcile(context.TODO(), tt.args.request)
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
				_, err := r.Reconcile(context.TODO(), tt.args.request)
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

func newRequest(ns, name string) reconcile.Request {
	return reconcile.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      name,
		},
	}
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

func Test_checkAndUpdateCredentials(t *testing.T) {
	tests := []struct {
		name                string
		setupFunc           func() (*Reconciler, *config.CredentialManager)
		expectClientUpdate  bool
		expectVersionUpdate bool
		expectError         bool
		resetFunc           func()
	}{
		{
			name: "client updates when version is newer",
			setupFunc: func() (*Reconciler, *config.CredentialManager) {
				// Setup credential manager with newer version
				cm := config.NewCredentialManager()

				// Set up environment variables
				os.Setenv("DD_API_KEY", "newkey")
				os.Setenv("DD_APP_KEY", "newapp")

				// Simulate credential change by incrementing version
				cm.SetCredentialVersionForTesting(2)

				// Create reconciler with older version
				r := &Reconciler{
					credsManager:     cm,
					lastCredsVersion: 1,
					log:              logr.Discard(),
				}

				return r, cm
			},
			expectClientUpdate:  true,
			expectVersionUpdate: true,
			expectError:         false,
			resetFunc: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "no client update when versions match",
			setupFunc: func() (*Reconciler, *config.CredentialManager) {
				cm := config.NewCredentialManager()
				cm.IsRefreshEnabled = true

				os.Setenv("DD_API_KEY", "key")
				os.Setenv("DD_APP_KEY", "app")

				// Both reconciler and credential manager have same version
				cm.SetCredentialVersionForTesting(1)

				r := &Reconciler{
					credsManager:     cm,
					lastCredsVersion: 1,
					log:              logr.Discard(),
				}

				return r, cm
			},
			expectClientUpdate:  false,
			expectVersionUpdate: false,
			expectError:         false,
			resetFunc: func() {
				os.Unsetenv("DD_API_KEY")
				os.Unsetenv("DD_APP_KEY")
			},
		},
		{
			name: "error when credential retrieval fails",
			setupFunc: func() (*Reconciler, *config.CredentialManager) {
				cm := config.NewCredentialManager()
				cm.IsRefreshEnabled = true
				// Don't set env variables to make credential retrieval fail
				cm.SetCredentialVersionForTesting(2)

				r := &Reconciler{
					credsManager:     cm,
					lastCredsVersion: 1,
					log:              logr.Discard(),
				}

				return r, cm
			},
			expectClientUpdate:  false,
			expectVersionUpdate: false,
			expectError:         true,
			resetFunc:           func() {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {

			r, cm := tt.setupFunc()

			initialVersion := r.lastCredsVersion
			t.Logf("InitialVersion: %d", initialVersion)

			err := r.checkAndUpdateCredentials()
			t.Logf("UpdatedVersion: %d", r.lastCredsVersion)
			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectVersionUpdate {
				assert.Greater(t, r.lastCredsVersion, initialVersion)
				assert.Equal(t, cm.GetCredentialVersion(), r.lastCredsVersion)
			} else {
				assert.Equal(t, initialVersion, r.lastCredsVersion)
			}

			tt.resetFunc()
		})
	}
}
