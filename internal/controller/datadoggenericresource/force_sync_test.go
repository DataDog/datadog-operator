// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"crypto/md5"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
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
	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
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
	mu                 sync.Mutex
	monitor            map[string]any
	reads              int
	readsWithoutAssets atomic.Int32
	puts               atomic.Int32

	etags       bool
	ifNoneMatch []string
	notModified int
}

func (f *fakeMonitorAPI) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	defer f.mu.Unlock()

	var body map[string]any
	if data, _ := io.ReadAll(r.Body); len(data) > 0 {
		_ = json.Unmarshal(data, &body)
	}
	switch r.Method {
	case http.MethodPost:
		f.monitor = body
		f.monitor["id"] = 1001
		f.monitor["created"] = time.Now().Format(time.RFC3339)
		f.monitor["creator"] = map[string]any{"handle": "test@example.com"}
		f.monitor["overall_state"] = "No Data"
	case http.MethodPut:
		f.puts.Add(1)
		maps.Copy(f.monitor, body)
	case http.MethodGet:
		f.reads++
		if r.URL.Query().Get("with_assets") != "true" {
			f.readsWithoutAssets.Add(1)
		}
		if f.etags {
			f.ifNoneMatch = append(f.ifNoneMatch, r.Header.Get("If-None-Match"))
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
		}
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

// newMonitorTestReconciler returns a reconciler whose monitor handler talks to
// a fakeMonitorAPI.
func newMonitorTestReconciler(t *testing.T) (*Reconciler, *fakeMonitorAPI) {
	t.Helper()
	api := &fakeMonitorAPI{}
	server := httptest.NewServer(api)
	t.Cleanup(server.Close)

	t.Setenv("DD_URL", server.URL)
	t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
	t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")

	s := scheme.Scheme
	s.AddKnownTypes(datadoghqv1alpha1.GroupVersion, &datadoghqv1alpha1.DatadogGenericResource{})
	r := NewReconciler(
		fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(&datadoghqv1alpha1.DatadogGenericResource{}).Build(),
		config.NewCredentialManager(fake.NewClientBuilder().Build()),
		s,
		logf.Log.WithName(t.Name()),
		record.NewBroadcaster().NewRecorder(scheme.Scheme, corev1.EventSource{Component: t.Name()}),
	)
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = &http.Client{Transport: datadogclient.NewETagTransport(server.Client().Transport)}
	r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
		datadoghqv1alpha1.Monitor: &MonitorHandler{client: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))},
	}
	return r, api
}

func TestReconcileGenericResource_MonitorForceSyncSkipsUpdateWhileRemoteUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name        string
		jsonSpec    string
		futureField bool
		// wantSkip is whether an unchanged remote monitor skips the update.
		wantSkip bool
	}{
		{
			name:     "monitor",
			jsonSpec: `{"name":"m","type":"metric alert","query":"avg(last_5m):avg:system.load.1{*} > 1","message":"msg","tags":["b:2","a:1"],"options":{"thresholds":{"critical":1}}}`,
			wantSkip: true,
		},
		{
			name:        "monitor with a field unknown to the SDK",
			jsonSpec:    `{"name":"m","type":"metric alert","query":"avg(last_5m):avg:system.load.1{*} > 1","message":"msg","future_setting":{"enabled":true}}`,
			futureField: true,
			wantSkip:    true,
		},
		{
			name:     "monitor with restriction policy always updates",
			jsonSpec: `{"name":"m","type":"metric alert","query":"avg(last_5m):avg:system.load.1{*} > 1","message":"msg","restriction_policy":{"bindings":[]}}`,
			wantSkip: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r, api := newMonitorTestReconciler(t)

			obj := mockGenericResource()
			obj.Spec.Type = datadoghqv1alpha1.Monitor
			obj.Spec.JsonSpec = tt.jsonSpec
			require.NoError(t, r.client.Create(context.TODO(), obj))
			req := newRequest(resourcesNamespace, resourcesName)
			get := func() *datadoghqv1alpha1.DatadogGenericResource {
				t.Helper()
				require.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj))
				return obj
			}
			// forceSync makes the force sync due and reconciles once.
			forceSync := func() *datadoghqv1alpha1.DatadogGenericResource {
				t.Helper()
				obj := get()
				stale := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod))
				obj.Status.LastForceSyncTime = &stale
				require.NoError(t, r.client.Status().Update(context.TODO(), obj))
				_, err := reconcileRequest(r, context.TODO(), req)
				require.NoError(t, err)
				return get()
			}

			// Add the finalizer, then create the monitor.
			for range 2 {
				_, err := reconcileRequest(r, context.TODO(), req)
				require.NoError(t, err)
			}
			createdHash := get().Status.RemoteDefinitionHash
			if !tt.wantSkip {
				assert.Empty(t, createdHash)
				before := api.puts.Load()
				forceSync()
				assert.Equal(t, before+1, api.puts.Load())
				return
			}
			require.NotEmpty(t, createdHash)

			before := api.puts.Load()
			synced := forceSync()
			assert.Equal(t, before, api.puts.Load(), "an unchanged monitor is not updated")
			assert.WithinDuration(t, time.Now(), synced.Status.LastForceSyncTime.Time, time.Minute)
			assert.Equal(t, int32(0), api.readsWithoutAssets.Load(), "force-sync reads request assets")

			api.editRemote(func(m map[string]any) { m["priority"] = 3 })
			synced = forceSync()
			assert.Equal(t, before+1, api.puts.Load(), "an edited monitor is updated")
			// The update only sends the spec's fields, so the out-of-band priority
			// stays and becomes part of the recorded definition.
			assert.NotEqual(t, createdHash, synced.Status.RemoteDefinitionHash)

			forceSync()
			assert.Equal(t, before+1, api.puts.Load(), "the next force sync is skipped again")

			if tt.futureField {
				before = api.puts.Load()
				api.editRemote(func(m map[string]any) {
					m["future_setting"] = map[string]any{"enabled": false}
				})
				forceSync()
				assert.Equal(t, before+1, api.puts.Load(), "drift in an SDK-unknown field triggers an update")
				api.editRemote(func(m map[string]any) {
					assert.Equal(t, map[string]any{"enabled": true}, m["future_setting"], "the SDK sends the desired field on update")
				})
				forceSync()
				assert.Equal(t, before+1, api.puts.Load(), "the repaired monitor is skipped again")
			}
		})
	}
}

func TestReconcileGenericResource_MonitorETags(t *testing.T) {
	r, api := newMonitorTestReconciler(t)
	api.etags = true

	obj := mockGenericResource()
	obj.Spec.Type = datadoghqv1alpha1.Monitor
	obj.Spec.JsonSpec = `{"name":"m","type":"metric alert","query":"avg(last_5m):avg:system.load.1{*} > 1","message":"msg","tags":["b:2","a:1"],"options":{"thresholds":{"critical":1}}}`
	require.NoError(t, r.client.Create(context.TODO(), obj))
	req := newRequest(resourcesNamespace, resourcesName)
	get := func() *datadoghqv1alpha1.DatadogGenericResource {
		t.Helper()
		require.NoError(t, r.client.Get(context.TODO(), client.ObjectKeyFromObject(obj), obj))
		return obj
	}
	// reconcile makes the force sync or the state poll due and reconciles once.
	reconcile := func(forceSync bool) *datadoghqv1alpha1.DatadogGenericResource {
		t.Helper()
		obj := get()
		now := metav1.Now()
		stale := metav1.NewTime(time.Now().Add(-2 * defaultForceSyncPeriod))
		obj.Status.LastForceSyncTime = &now
		obj.Status.StateLastUpdateTime = &stale
		if forceSync {
			obj.Status.LastForceSyncTime = &stale
		}
		require.NoError(t, r.client.Status().Update(context.TODO(), obj))
		_, err := reconcileRequest(r, context.TODO(), req)
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

	// Add the finalizer, then create the monitor.
	for range 2 {
		_, err := reconcileRequest(r, context.TODO(), req)
		require.NoError(t, err)
	}
	require.NotEmpty(t, get().Status.RemoteDefinitionHash)
	assert.Empty(t, get().Status.Etag, "write responses carry no ETag")

	// The first force sync reads the whole monitor and records its ETag.
	synced := reconcile(true)
	assert.Empty(t, lastIfNoneMatch())
	assert.Equal(t, int32(0), api.puts.Load())
	etag := synced.Status.Etag
	require.NotEmpty(t, etag)
	assert.Equal(t, "No Data", synced.Status.State, "the force sync records the state that its ETag covers")

	t.Run("force sync of an unchanged monitor gets Not Modified", func(t *testing.T) {
		synced := reconcile(true)
		assert.Equal(t, etag, lastIfNoneMatch())
		assert.Equal(t, 1, api.notModified)
		assert.Equal(t, int32(0), api.puts.Load())
		assert.Equal(t, etag, synced.Status.Etag)
		assert.WithinDuration(t, time.Now(), synced.Status.LastForceSyncTime.Time, time.Minute)
	})

	t.Run("state poll of an unchanged monitor keeps its state", func(t *testing.T) {
		before := get().Status.State
		polled := reconcile(false)
		assert.Equal(t, 2, api.notModified)
		assert.Equal(t, before, polled.Status.State)
		assert.WithinDuration(t, time.Now(), polled.Status.StateLastUpdateTime.Time, time.Minute)
		assert.Equal(t, etag, polled.Status.Etag)
	})

	t.Run("state change is read and its ETag recorded", func(t *testing.T) {
		api.editRemote(func(m map[string]any) { m["overall_state"] = "Alert" })
		polled := reconcile(false)
		assert.Equal(t, etag, lastIfNoneMatch())
		assert.Equal(t, 2, api.notModified)
		assert.Equal(t, "Alert", polled.Status.State)
		assert.NotEqual(t, etag, polled.Status.Etag)
		assert.NotEmpty(t, polled.Status.Etag)

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
		reconcile(true)
		assert.Equal(t, 4, api.notModified)
		assert.Equal(t, int32(1), api.puts.Load())
	})

	t.Run("state change read by a force sync is recorded", func(t *testing.T) {
		api.editRemote(func(m map[string]any) { m["overall_state"] = "Warn" })
		synced := reconcile(true)
		assert.Equal(t, "Warn", synced.Status.State)
		before := api.notModified
		polled := reconcile(false)
		assert.Equal(t, before+1, api.notModified)
		assert.Equal(t, "Warn", polled.Status.State)
	})

	t.Run("state poll without a polled state does not send the ETag", func(t *testing.T) {
		obj := get()
		obj.Status.State = ctrutils.StatePollingDisabledState
		require.NoError(t, r.client.Status().Update(context.TODO(), obj))
		api.editRemote(func(m map[string]any) { m["overall_state"] = "OK" })
		before := api.notModified
		polled := reconcile(false)
		assert.Empty(t, lastIfNoneMatch())
		assert.Equal(t, before, api.notModified)
		assert.Equal(t, "OK", polled.Status.State)
	})

	assert.Equal(t, int32(0), api.readsWithoutAssets.Load(), "every read requests assets, so their ETags match")
}
