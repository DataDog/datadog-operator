// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"encoding/json"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
)

// fakeMonitorAPI stores a single monitor. Like the Datadog API, an update
// replaces each top-level field it sends and keeps the others, and every read
// returns fresh state fields that are not part of the definition.
type fakeMonitorAPI struct {
	mu        sync.Mutex
	monitor   map[string]any
	reads     int
	puts      atomic.Int32
	validates atomic.Int32
	failPuts  atomic.Bool
}

func (f *fakeMonitorAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	defer f.mu.Unlock()

	var body map[string]any
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		_ = json.Unmarshal(data, &body)
	}
	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validate"):
		f.validates.Add(1)
		_, _ = w.Write([]byte(`{}`))
		return
	case r.Method == http.MethodPost:
		f.monitor = body
		f.monitor["id"] = 1001
		f.monitor["created"] = time.Now().Format(time.RFC3339)
	case r.Method == http.MethodPut:
		f.puts.Add(1)
		if f.failPuts.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":["boom"]}`))
			return
		}
		maps.Copy(f.monitor, body)
	case r.Method == http.MethodGet:
		f.reads++
		f.monitor["overall_state"] = []string{"OK", "Alert"}[f.reads%2]
	}
	f.monitor["modified"] = time.Now().Add(time.Duration(f.reads) * time.Second).Format(time.RFC3339Nano)
	_ = json.NewEncoder(w).Encode(f.monitor)
}

// editRemote simulates an out-of-band edit of the stored monitor.
func (f *fakeMonitorAPI) editRemote(edit func(map[string]any)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	edit(f.monitor)
}

// newForceSyncTestReconciler returns a reconciler that talks to a fakeMonitorAPI.
func newForceSyncTestReconciler(t *testing.T) (*Reconciler, *fakeMonitorAPI) {
	t.Helper()
	api := &fakeMonitorAPI{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	t.Setenv("DD_URL", server.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogMonitor{})
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = server.Client()
	r := &Reconciler{
		client:        fake.NewClientBuilder().WithStatusSubresource(&datadoghqv1alpha1.DatadogMonitor{}).Build(),
		datadogClient: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg)),
		credsManager:  config.NewCredentialManager(fake.NewClientBuilder().Build()),
		scheme:        s,
		recorder:      record.NewBroadcaster().NewRecorder(scheme.Scheme, corev1.EventSource{Component: t.Name()}),
		log:           logf.Log.WithName(t.Name()),
		requeuePeriod: defaultRequeuePeriod,
	}
	r.apiReader = r.client
	return r, api
}

func TestReconcileDatadogMonitor_ForceSyncSkipsUpdateWhileRemoteUnchanged(t *testing.T) {
	r, api := newForceSyncTestReconciler(t)
	dm := genericDatadogMonitor(r.client)
	get := func() *datadoghqv1alpha1.DatadogMonitor {
		t.Helper()
		require.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm))
		return dm
	}
	// forceSync makes the force sync due and reconciles once.
	forceSync := func() *datadoghqv1alpha1.DatadogMonitor {
		t.Helper()
		dm := get()
		stale := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod))
		dm.Status.MonitorLastForceSyncTime = &stale
		require.NoError(t, r.client.Status().Update(context.TODO(), dm))
		_, err := r.Reconcile(context.TODO(), get())
		require.NoError(t, err)
		return get()
	}

	// Add the finalizer, add the required tags, then create the monitor.
	for range 3 {
		_, err := r.Reconcile(context.TODO(), get())
		require.NoError(t, err)
	}
	created := get()
	require.NotZero(t, created.Status.ID)
	createdHash := created.Status.RemoteDefinitionHash
	require.NotEmpty(t, createdHash, "create records the hash of the created monitor")

	t.Run("unchanged remote skips validate and update", func(t *testing.T) {
		before := api.puts.Load()
		synced := forceSync()
		assert.Equal(t, before, api.puts.Load())
		assert.Equal(t, int32(1), api.validates.Load(), "only the create validated")
		assert.WithinDuration(t, time.Now(), synced.Status.MonitorLastForceSyncTime.Time, time.Minute)
		assert.Equal(t, createdHash, synced.Status.RemoteDefinitionHash)
	})

	t.Run("edit of a field the spec does not set is reverted", func(t *testing.T) {
		api.editRemote(func(m map[string]any) {
			m["options"].(map[string]any)["renotify_interval"] = 120
		})
		before := api.puts.Load()
		synced := forceSync()
		assert.Equal(t, before+1, api.puts.Load())
		assert.Equal(t, createdHash, synced.Status.RemoteDefinitionHash, "the update restores the created definition")

		before = api.puts.Load()
		forceSync()
		assert.Equal(t, before, api.puts.Load(), "the next force sync is skipped again")
	})

	t.Run("edit of a field the spec sets is reverted", func(t *testing.T) {
		api.editRemote(func(m map[string]any) { m["message"] = "edited out of band" })
		before := api.puts.Load()
		forceSync()
		assert.Equal(t, before+1, api.puts.Load())
	})

	t.Run("missing hash updates once", func(t *testing.T) {
		dm := get()
		dm.Status.RemoteDefinitionHash = ""
		require.NoError(t, r.client.Status().Update(context.TODO(), dm))
		before := api.puts.Load()
		synced := forceSync()
		assert.Equal(t, before+1, api.puts.Load())
		assert.Equal(t, createdHash, synced.Status.RemoteDefinitionHash)
	})

	t.Run("failed update clears the hash", func(t *testing.T) {
		api.editRemote(func(m map[string]any) { m["name"] = "edited out of band" })
		api.failPuts.Store(true)
		failed := forceSync()
		assert.Empty(t, failed.Status.RemoteDefinitionHash)

		// The remote is unchanged since the failed update, but without a hash
		// the next force sync still updates.
		api.failPuts.Store(false)
		before := api.puts.Load()
		synced := forceSync()
		assert.Equal(t, before+1, api.puts.Load())
		assert.Equal(t, createdHash, synced.Status.RemoteDefinitionHash)
	})
}

func TestReconcileDatadogMonitor_CreateAppliesRestrictedRoles(t *testing.T) {
	r, api := newForceSyncTestReconciler(t)
	dm := genericDatadogMonitor(r.client)
	dm.Spec.RestrictedRoles = []string{"role-b", "role-a"}
	require.NoError(t, r.client.Update(context.TODO(), dm))

	// Add the finalizer, add the required tags, then create the monitor.
	for range 3 {
		require.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm))
		_, err := r.Reconcile(context.TODO(), dm)
		require.NoError(t, err)
	}
	api.editRemote(func(m map[string]any) {
		assert.ElementsMatch(t, []any{"role-a", "role-b"}, m["restricted_roles"], "create applies the restricted roles")
	})

	// The first force sync after a create is due immediately and finds the
	// monitor unchanged.
	require.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm))
	require.Nil(t, dm.Status.MonitorLastForceSyncTime)
	_, err := r.Reconcile(context.TODO(), dm)
	require.NoError(t, err)
	assert.Equal(t, int32(0), api.puts.Load())
}
