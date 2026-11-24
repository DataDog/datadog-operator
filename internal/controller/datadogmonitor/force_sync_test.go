// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
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
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

// fakeMonitorAPI stores a single monitor. Like the Datadog API, an update
// replaces each top-level field it sends and keeps the others, and every read
// returns fresh state fields that are not part of the definition.
//
// With etags set, reads instead return the stored state, an ETag computed from
// the response body like the Datadog API does, and 304 Not Modified when the
// request sends that ETag in If-None-Match.
type fakeMonitorAPI struct {
	mu        sync.Mutex
	monitor   map[string]any
	reads     int
	puts      atomic.Int32
	validates atomic.Int32
	failPuts  atomic.Bool

	etags        bool
	ifNoneMatch  []string
	notModified  int
	lastGetQuery string
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
		f.monitor["overall_state"] = "No Data"
	case r.Method == http.MethodPut:
		f.puts.Add(1)
		if f.failPuts.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"errors":["boom"]}`))
			return
		}
		maps.Copy(f.monitor, body)
	case r.Method == http.MethodGet && f.etags:
		f.reads++
		f.ifNoneMatch = append(f.ifNoneMatch, r.Header.Get("If-None-Match"))
		f.lastGetQuery = r.URL.RawQuery
		body, _ := json.Marshal(f.monitor)
		etag := fmt.Sprintf(`"%x"`, md5.Sum(body))
		w.Header().Set("ETag", etag)
		if r.Header.Get("If-None-Match") == etag {
			f.notModified++
			w.WriteHeader(http.StatusNotModified)
			return
		}
		_, _ = w.Write(body)
		return
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
	cfg.HTTPClient = &http.Client{Transport: datadogclient.NewETagTransport(server.Client().Transport)}
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

func TestReconcileDatadogMonitor_ETags(t *testing.T) {
	r, api := newForceSyncTestReconciler(t)
	api.etags = true
	dm := genericDatadogMonitor(r.client)
	get := func() *datadoghqv1alpha1.DatadogMonitor {
		t.Helper()
		require.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(dm), dm))
		return dm
	}
	// reconcile makes the force sync or the state poll due and reconciles once.
	reconcile := func(forceSync bool) *datadoghqv1alpha1.DatadogMonitor {
		t.Helper()
		dm := get()
		now := metav1.Now()
		stale := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod))
		dm.Status.MonitorLastForceSyncTime = &now
		dm.Status.MonitorStateLastUpdateTime = &stale
		if forceSync {
			dm.Status.MonitorLastForceSyncTime = &stale
		}
		require.NoError(t, r.client.Status().Update(context.TODO(), dm))
		_, err := r.Reconcile(context.TODO(), get())
		require.NoError(t, err)
		return get()
	}
	lastIfNoneMatch := func() string {
		t.Helper()
		api.mu.Lock()
		defer api.mu.Unlock()
		require.NotEmpty(t, api.ifNoneMatch)
		return api.ifNoneMatch[len(api.ifNoneMatch)-1]
	}

	// Add the finalizer, add the required tags, then create the monitor.
	for range 3 {
		_, err := r.Reconcile(context.TODO(), get())
		require.NoError(t, err)
	}
	require.NotZero(t, get().Status.ID)
	assert.Empty(t, get().Status.Etag, "write responses carry no ETag")

	// The first force sync reads the whole monitor and records its ETag.
	synced := reconcile(true)
	assert.Empty(t, lastIfNoneMatch())
	assert.Equal(t, "group_states=all", api.lastGetQuery)
	assert.Equal(t, int32(0), api.puts.Load())
	etag := synced.Status.Etag
	require.NotEmpty(t, etag)
	assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateNoData, synced.Status.MonitorState, "the force sync records the state that its ETag covers")

	t.Run("force sync of an unchanged monitor gets Not Modified", func(t *testing.T) {
		synced := reconcile(true)
		assert.Equal(t, etag, lastIfNoneMatch())
		assert.Equal(t, 1, api.notModified)
		assert.Equal(t, int32(0), api.puts.Load())
		assert.Equal(t, etag, synced.Status.Etag)
		assert.WithinDuration(t, time.Now(), synced.Status.MonitorLastForceSyncTime.Time, time.Minute)
	})

	t.Run("state poll of an unchanged monitor keeps its state", func(t *testing.T) {
		before := get().Status.MonitorState
		polled := reconcile(false)
		assert.Equal(t, 2, api.notModified)
		assert.Equal(t, before, polled.Status.MonitorState)
		assert.Equal(t, datadoghqv1alpha1.MonitorStateSyncStatusOK, polled.Status.MonitorStateSyncStatus)
		assert.WithinDuration(t, time.Now(), polled.Status.MonitorStateLastUpdateTime.Time, time.Minute)
		assert.Equal(t, etag, polled.Status.Etag)
	})

	t.Run("state change is read and its ETag recorded", func(t *testing.T) {
		api.editRemote(func(m map[string]any) { m["overall_state"] = "Alert" })
		polled := reconcile(false)
		assert.Equal(t, etag, lastIfNoneMatch())
		assert.Equal(t, 2, api.notModified)
		assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateAlert, polled.Status.MonitorState)
		assert.NotEqual(t, etag, polled.Status.Etag)
		assert.NotEmpty(t, polled.Status.Etag)
		etag = polled.Status.Etag

		reconcile(true)
		assert.Equal(t, 3, api.notModified, "a force sync can use the ETag of a state poll")
		assert.Equal(t, int32(0), api.puts.Load())
	})

	t.Run("out-of-band edit seen by a state poll is still reverted", func(t *testing.T) {
		api.editRemote(func(m map[string]any) { m["message"] = "edited out of band" })
		polled := reconcile(false)
		assert.Empty(t, polled.Status.Etag, "the ETag of a drifted definition is not kept")

		synced := reconcile(true)
		assert.Empty(t, lastIfNoneMatch())
		assert.Equal(t, int32(1), api.puts.Load())
		assert.Empty(t, synced.Status.Etag, "an update clears the ETag")

		synced = reconcile(true)
		assert.NotEmpty(t, synced.Status.Etag)
		assert.Equal(t, int32(1), api.puts.Load())
		synced = reconcile(true)
		assert.Equal(t, 4, api.notModified)
		assert.Equal(t, int32(1), api.puts.Load())
	})

	t.Run("state change read by a force sync is recorded", func(t *testing.T) {
		api.editRemote(func(m map[string]any) { m["overall_state"] = "Warn" })
		synced := reconcile(true)
		assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateWarn, synced.Status.MonitorState)
		before := api.notModified
		polled := reconcile(false)
		assert.Equal(t, before+1, api.notModified)
		assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateWarn, polled.Status.MonitorState)
	})

	t.Run("state poll without a polled state does not send the ETag", func(t *testing.T) {
		// For example, the first poll after polling is re-enabled.
		dm := get()
		dm.Status.MonitorState = ""
		require.NoError(t, r.client.Status().Update(context.TODO(), dm))
		api.editRemote(func(m map[string]any) { m["overall_state"] = "OK" })
		before := api.notModified
		polled := reconcile(false)
		assert.Empty(t, lastIfNoneMatch())
		assert.Equal(t, before, api.notModified)
		assert.Equal(t, datadoghqv1alpha1.DatadogMonitorStateOK, polled.Status.MonitorState)
	})

	t.Run("spec change updates despite Not Modified", func(t *testing.T) {
		reconcile(true) // records an ETag
		before, puts := api.notModified, api.puts.Load()
		dm := get()
		require.NotEmpty(t, dm.Status.Etag)
		dm.Spec.Message = "new message"
		require.NoError(t, r.client.Update(context.TODO(), dm))
		_, err := r.Reconcile(context.TODO(), get())
		require.NoError(t, err)
		assert.Equal(t, before+1, api.notModified, "the existence check gets Not Modified")
		assert.Equal(t, puts+1, api.puts.Load())
		assert.Empty(t, get().Status.Etag)
	})
}
