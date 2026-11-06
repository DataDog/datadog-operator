// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"encoding/json"
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
)

// fakeMonitorAPI stores a single monitor. Like the Datadog API, an update
// replaces each top-level field it sends and keeps the others, and every read
// returns fresh state fields that are not part of the definition.
type fakeMonitorAPI struct {
	mu                 sync.Mutex
	monitor            map[string]any
	reads              int
	readsWithoutAssets atomic.Int32
	puts               atomic.Int32
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
	case http.MethodPut:
		f.puts.Add(1)
		maps.Copy(f.monitor, body)
	case http.MethodGet:
		f.reads++
		if r.URL.Query().Get("with_assets") != "true" {
			f.readsWithoutAssets.Add(1)
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

func TestReconcileGenericResource_MonitorForceSyncSkipsUpdateWhileRemoteUnchanged(t *testing.T) {
	for _, tt := range []struct {
		name     string
		jsonSpec string
		// wantSkip is whether an unchanged remote monitor skips the update.
		wantSkip bool
	}{
		{
			name:     "monitor",
			jsonSpec: `{"name":"m","type":"metric alert","query":"avg(last_5m):avg:system.load.1{*} > 1","message":"msg","tags":["b:2","a:1"],"options":{"thresholds":{"critical":1}}}`,
			wantSkip: true,
		},
		{
			name:     "monitor with restriction policy always updates",
			jsonSpec: `{"name":"m","type":"metric alert","query":"avg(last_5m):avg:system.load.1{*} > 1","message":"msg","restriction_policy":{"bindings":[]}}`,
			wantSkip: false,
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			api := &fakeMonitorAPI{}
			server := httptest.NewServer(api)
			defer server.Close()

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
			cfg.HTTPClient = server.Client()
			r.handlers = map[datadoghqv1alpha1.SupportedResourcesType]ResourceHandler{
				datadoghqv1alpha1.Monitor: &MonitorHandler{client: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))},
			}

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
		})
	}
}
