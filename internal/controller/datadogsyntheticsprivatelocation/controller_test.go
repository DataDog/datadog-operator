// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogsyntheticsprivatelocation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	fake "sigs.k8s.io/controller-runtime/pkg/client/fake"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
)

const testPLID = "pl-test-id"

// ddMock is a mock Datadog Synthetics API server that records every call.
type ddMock struct {
	server *httptest.Server
	mu     sync.Mutex
	calls  []string
	drift  bool // GET on the private location returns 404
}

func newDDMock(t *testing.T) *ddMock {
	t.Helper()
	m := &ddMock{}

	creationResponse := `{"config": {"id": "` + testPLID + `", "accessKey": "ak", "secretAccessKey": "sk", "publicKey": "pk", "privateKey": "priv"},
		"private_location": {"id": "` + testPLID + `", "name": "my-pl", "description": "test", "tags": ["generated:kubernetes"]}}`
	privateLocationResponse := `{"id": "` + testPLID + `", "name": "my-pl", "description": "test", "tags": ["generated:kubernetes"]}`

	m.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		m.mu.Lock()
		m.calls = append(m.calls, r.Method+" "+r.URL.Path)
		drift := m.drift
		m.mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost:
			_, _ = w.Write([]byte(creationResponse))
		case r.Method == http.MethodGet && drift:
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"errors": ["not found"]}`))
		case r.Method == http.MethodGet || r.Method == http.MethodPut:
			_, _ = w.Write([]byte(privateLocationResponse))
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusMethodNotAllowed)
		}
	}))
	t.Cleanup(m.server.Close)

	return m
}

func (m *ddMock) callCount(method string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	count := 0
	for _, c := range m.calls {
		if strings.HasPrefix(c, method+" ") {
			count++
		}
	}
	return count
}

func newTestReconciler(t *testing.T, mock *ddMock) (*Reconciler, client.Client) {
	t.Helper()

	t.Setenv("DD_URL", mock.server.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := newSchemeWithSPL(t)
	k8sClient := fake.NewClientBuilder().
		WithScheme(s).
		WithStatusSubresource(&datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}).
		Build()

	return &Reconciler{
		client:             k8sClient,
		scheme:             s,
		log:                ctrl.Log.WithName("test"),
		recorder:           record.NewFakeRecorder(100),
		credsManager:       config.NewCredentialManager(fake.NewClientBuilder().Build()),
		ddClientSynthetics: newTestSyntheticsClient(mock.server),
		requeuePeriod:      defaultRequeuePeriod,
		forceSyncPeriod:    defaultForceSyncPeriod,
	}, k8sClient
}

// createInstance runs the initial reconciles (finalizer then creation) and
// returns the persisted instance.
func createInstance(t *testing.T, r *Reconciler, k8sClient client.Client) *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation {
	t.Helper()

	instance := newTestInstance()
	require.NoError(t, k8sClient.Create(context.TODO(), instance))

	// The first pass only adds the finalizer and requeues.
	var result ctrl.Result
	var err error
	for i := 0; i < 2; i++ {
		result, err = r.Reconcile(context.TODO(), getInstance(t, k8sClient))
		require.NoError(t, err)
	}
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)

	return getInstance(t, k8sClient)
}

func getInstance(t *testing.T, k8sClient client.Client) *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation {
	t.Helper()
	instance := &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}
	require.NoError(t, k8sClient.Get(context.TODO(),
		types.NamespacedName{Name: "my-pl", Namespace: "default"}, instance))
	return instance
}

func getCondition(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation, conditionType string) *metav1.Condition {
	for i := range instance.Status.Conditions {
		if instance.Status.Conditions[i].Type == conditionType {
			return &instance.Status.Conditions[i]
		}
	}
	return nil
}

func TestForceSyncPeriodFromEnv(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  time.Duration
	}{
		{"empty uses default", "", defaultForceSyncPeriod},
		{"valid override", "10m", 10 * time.Minute},
		{"invalid falls back", "banana", defaultForceSyncPeriod},
		{"negative falls back", "-5m", defaultForceSyncPeriod},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DD_SYNTHETICS_PRIVATE_LOCATION_FORCE_SYNC_PERIOD", tt.value)
			if got := forceSyncPeriodFromEnv(logr.Discard(), defaultForceSyncPeriod); got != tt.want {
				t.Errorf("forceSyncPeriodFromEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReconciler_Reconcile_create(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)

	instance := createInstance(t, r, k8sClient)

	assert.Equal(t, 1, mock.callCount(http.MethodPost))
	assert.Equal(t, testPLID, instance.Status.ID)
	assert.Equal(t, "my-pl-config", instance.Status.ConfigSecretName)
	assert.NotEmpty(t, instance.Status.CurrentHash)
	assert.Equal(t, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusOK, instance.Status.SyncStatus)
	require.NotNil(t, instance.Status.LastForceSyncTime)
	require.NotNil(t, instance.Status.Created)

	created := getCondition(instance, "Created")
	require.NotNil(t, created)
	assert.Equal(t, metav1.ConditionTrue, created.Status)
	active := getCondition(instance, "Active")
	require.NotNil(t, active)

	// Owned resources exist and are owned by the instance.
	secret := &corev1.Secret{}
	require.NoError(t, k8sClient.Get(context.TODO(),
		types.NamespacedName{Name: "my-pl-config", Namespace: "default"}, secret))
	assertOwnedByInstance(t, secret, instance)
	assert.Contains(t, string(secret.Data[datadoghqv1alpha1.DatadogSPLConfigSecretDataKey]), `"site":"datadoghq.com"`)

	sa := &corev1.ServiceAccount{}
	require.NoError(t, k8sClient.Get(context.TODO(),
		types.NamespacedName{Name: "my-pl", Namespace: "default"}, sa))
	assertOwnedByInstance(t, sa, instance)

	deployment := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(context.TODO(),
		types.NamespacedName{Name: "my-pl", Namespace: "default"}, deployment))
	assertOwnedByInstance(t, deployment, instance)
	// The Deployment status is only observable from the next reconcile.
	assert.Nil(t, instance.Status.Deployment)

	// The finalizer is set on the fresh instance.
	assert.Contains(t, instance.Finalizers, datadogSyntheticsPrivateLocationFinalizerName)
}

func TestReconciler_Reconcile_statusProbesUnsupportedWorkerVersion(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)

	instance := newTestInstance()
	instance.Annotations = map[string]string{datadoghqv1alpha1.DatadogSPLStatusProbesEnabledAnnotation: "true"}
	instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
		Image: &datadoghqv1alpha1.DatadogSPLImage{Tag: "1.11.0"},
	}
	require.NoError(t, k8sClient.Create(context.TODO(), instance))
	for i := 0; i < 2; i++ {
		_, err := r.Reconcile(context.TODO(), getInstance(t, k8sClient))
		require.NoError(t, err)
	}

	deployment := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(context.TODO(),
		types.NamespacedName{Name: "my-pl", Namespace: "default"}, deployment))
	assert.Nil(t, deployment.Spec.Template.Spec.Containers[0].LivenessProbe)
	assert.Nil(t, deployment.Spec.Template.Spec.Containers[0].ReadinessProbe)

	recorder, ok := r.recorder.(*record.FakeRecorder)
	require.True(t, ok)
	close(recorder.Events)
	var warnings []string
	for event := range recorder.Events {
		if strings.HasPrefix(event, corev1.EventTypeWarning+" "+eventReasonPrefix+"StatusProbesUnsupported") {
			warnings = append(warnings, event)
		}
	}
	assert.NotEmpty(t, warnings)
}

func TestReconciler_Reconcile_noChangeRefreshesOnly(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)
	_ = createInstance(t, r, k8sClient)

	assert.Zero(t, mock.callCount(http.MethodPut))

	instance := getInstance(t, k8sClient)
	result, err := r.Reconcile(context.TODO(), instance)
	require.NoError(t, err)
	// No remote work was due: requeue at the requeue period (bounded from
	// the 1h force-sync period).
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)

	assert.Equal(t, 1, mock.callCount(http.MethodPost), "must not re-create")
	assert.Zero(t, mock.callCount(http.MethodPut), "must not update")
	assert.Equal(t, 1, mock.callCount(http.MethodGet))
	assert.Equal(t, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusOK, instance.Status.SyncStatus)
	// The worker Deployment status is now reported.
	assert.NotNil(t, instance.Status.Deployment)
}

func TestReconciler_Reconcile_specChangeTriggersUpdate(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)
	instance := createInstance(t, r, k8sClient)

	instance.Spec.Tags = []string{"env:prod"}
	require.NoError(t, k8sClient.Update(context.TODO(), instance))

	result, err := r.Reconcile(context.TODO(), instance)
	require.NoError(t, err)

	assert.Equal(t, 1, mock.callCount(http.MethodPost), "must not re-create")
	assert.Equal(t, 1, mock.callCount(http.MethodPut))
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)

	persisted := getInstance(t, k8sClient)
	assert.Equal(t, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusOK, persisted.Status.SyncStatus)
	updated := getCondition(persisted, "Updated")
	require.NotNil(t, updated)
	assert.Equal(t, metav1.ConditionTrue, updated.Status)
}

func TestReconciler_Reconcile_forceSyncAfterPeriod(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)
	instance := createInstance(t, r, k8sClient)

	// Pretend the last force sync happened long ago.
	instance.Status.LastForceSyncTime = ptr.To(metav1.NewTime(time.Now().Add(-2 * time.Hour)))
	require.NoError(t, k8sClient.Status().Update(context.TODO(), instance))

	result, err := r.Reconcile(context.TODO(), instance)
	require.NoError(t, err)
	assert.Equal(t, 1, mock.callCount(http.MethodPut))
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)

	persisted := getInstance(t, k8sClient)
	require.NotNil(t, persisted.Status.LastForceSyncTime)
	assert.WithinDuration(t, time.Now(), persisted.Status.LastForceSyncTime.Time, time.Minute)
}

func TestReconciler_Reconcile_outOfBandDeletionDoesNotRecreate(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)
	instance := createInstance(t, r, k8sClient)

	mock.mu.Lock()
	mock.drift = true
	mock.mu.Unlock()

	result, err := r.Reconcile(context.TODO(), instance)
	require.NoError(t, err)

	assert.Equal(t, 1, mock.callCount(http.MethodPost), "must not auto-recreate the private location")
	assert.Equal(t, defaultForceSyncPeriod, result.RequeueAfter)

	persisted := getInstance(t, k8sClient)
	assert.Equal(t, datadoghqv1alpha1.DatadogSyntheticsPrivateLocationSyncStatusDeleted, persisted.Status.SyncStatus)
	errCond := getCondition(persisted, "Error")
	require.NotNil(t, errCond)
	assert.Equal(t, metav1.ConditionTrue, errCond.Status)
	assert.Equal(t, reasonPrivateLocationDeleted, errCond.Reason)

	// The worker Deployment is left in place.
	deployment := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(context.TODO(),
		types.NamespacedName{Name: "my-pl", Namespace: "default"}, deployment))
}

func TestReconciler_Reconcile_missingConfigSecretSkipsDeployment(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)
	instance := createInstance(t, r, k8sClient)

	require.NoError(t, k8sClient.Delete(context.TODO(),
		&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "my-pl-config", Namespace: "default"}}))

	result, err := r.Reconcile(context.TODO(), instance)
	require.NoError(t, err)
	assert.Equal(t, defaultErrRequeuePeriod, result.RequeueAfter)

	persisted := getInstance(t, k8sClient)
	errCond := getCondition(persisted, "Error")
	require.NotNil(t, errCond)
	assert.Equal(t, metav1.ConditionTrue, errCond.Status)
	assert.Equal(t, reasonConfigMissing, errCond.Reason)

	// No new Deployment is created without a usable config.
	pdbList := &appsv1.DeploymentList{}
	require.NoError(t, k8sClient.List(context.TODO(), pdbList))
	// The Deployment created during createInstance is still there; nothing
	// new is added.
	assert.Len(t, pdbList.Items, 1)
}

func TestReconciler_Reconcile_invalidSpec(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*datadoghqv1alpha1.DatadogSyntheticsPrivateLocation)
	}{
		{
			name: "pdb without minAvailable or maxUnavailable",
			mutate: func(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) {
				instance.Spec.Worker = &datadoghqv1alpha1.DatadogSPLWorker{
					PodDisruptionBudget: &datadoghqv1alpha1.DatadogSPLPodDisruptionBudget{
						Enabled: true,
					},
				}
			},
		},
		{
			name: "config override annotation is not valid JSON",
			mutate: func(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) {
				instance.Annotations = map[string]string{datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation: `{invalid`}
			},
		},
		{
			name: "config override annotation sets a Datadog-managed key",
			mutate: func(instance *datadoghqv1alpha1.DatadogSyntheticsPrivateLocation) {
				instance.Annotations = map[string]string{datadoghqv1alpha1.DatadogSPLWorkerConfigOverrideAnnotation: `{"accessKey": "hacked"}`}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := newDDMock(t)
			r, k8sClient := newTestReconciler(t, mock)

			instance := newTestInstance()
			tt.mutate(instance)
			require.NoError(t, k8sClient.Create(context.TODO(), instance))

			var result ctrl.Result
			var err error
			for i := 0; i < 2; i++ {
				result, err = r.Reconcile(context.TODO(), instance)
				require.NoError(t, err)
			}
			assert.Empty(t, result.RequeueAfter, "invalid spec is not retried")

			assert.Zero(t, mock.callCount(http.MethodPost))
			persisted := getInstance(t, k8sClient)
			errCond := getCondition(persisted, "Error")
			require.NotNil(t, errCond)
			assert.Equal(t, "InvalidSpec", errCond.Reason)
		})
	}
}

func TestReconciler_Reconcile_delete(t *testing.T) {
	mock := newDDMock(t)
	r, k8sClient := newTestReconciler(t, mock)
	instance := createInstance(t, r, k8sClient)

	require.NoError(t, k8sClient.Delete(context.TODO(), instance))

	fetched := getInstance(t, k8sClient)
	result, err := r.Reconcile(context.TODO(), fetched)
	require.NoError(t, err)
	// Finalization completed; the finalizer slow-cadence requeue is the
	// garbage-collection safety net.
	assert.Equal(t, defaultRequeuePeriod, result.RequeueAfter)

	assert.Equal(t, 1, mock.callCount(http.MethodDelete))

	// The finalizer cleared and the instance is gone.
	gone := &datadoghqv1alpha1.DatadogSyntheticsPrivateLocation{}
	err = k8sClient.Get(context.TODO(),
		types.NamespacedName{Name: "my-pl", Namespace: "default"}, gone)
	assert.True(t, apierrors.IsNotFound(err))
}
