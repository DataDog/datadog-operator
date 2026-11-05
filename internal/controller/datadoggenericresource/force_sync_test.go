// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadoggenericresource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
)

func TestReconcileGenericMonitorForceSync(t *testing.T) {
	const remote = `{"id":12345,"name":"CPU","message":"CPU high","query":"avg(last_5m):avg:system.cpu.user{*} > 80",
		"type":"query alert","priority":null,"overall_state":"Alert","tags":["team:a"],
		"created":"2026-01-01T00:00:00Z","creator":{"handle":"user@example.com"},
		"options":{"thresholds":{"critical":80.0,"warning":70},"include_tags":true,"new_host_delay":300,"notify_audit":false}}`
	const spec = `{"name":"CPU","message":"CPU high\n","query":"avg(last_5m):avg:system.cpu.user{*} > 80","type":"metric alert","options":{"thresholds":{"critical":80}}}`
	tests := []struct {
		name, spec               string
		getStatus                int
		remotePriority           string
		wantUpdates, wantCreates int
		wantSync                 bool
		wantPolicyRead           bool
	}{
		{name: "no-op with server defaults", spec: spec, wantSync: true},
		{name: "zero priority matches canonical null", spec: `{"priority":0}`, wantSync: true},
		{name: "explicit zero clears priority", spec: `{"priority":0}`, remotePriority: "3", wantUpdates: 1, wantSync: true},
		{name: "explicit null clears priority", spec: `{"priority":null}`, remotePriority: "3", wantUpdates: 1, wantSync: true},
		{name: "SDK omits null tags", spec: `{"tags":null}`, wantSync: true},
		{name: "SDK omits null option", spec: `{"options":{"include_tags":null}}`, wantSync: true},
		{name: "nullable explicit null still clears priority", spec: `{"priority":null}`, wantSync: true},
		{name: "restriction policy matches", spec: `{"restriction_policy":{"bindings":[{"relation":"editor","principals":["role:managed"]}]}}`, wantSync: true, wantPolicyRead: true},
		{name: "restriction policy drifts", spec: `{"restriction_policy":{"bindings":[{"relation":"editor","principals":["role:other"]}]}}`, wantUpdates: 1, wantSync: true, wantPolicyRead: true},
		{name: "ignored policy does not hide name drift", spec: `{"name":"Desired CPU","restriction_policy":null}`, wantUpdates: 1, wantSync: true},
		{name: "API ignores null policy", spec: `{"restriction_policy":null}`, wantSync: true},
		{name: "empty policy bindings clear existing policy", spec: `{"restriction_policy":{"bindings":[]}}`, wantUpdates: 1, wantSync: true, wantPolicyRead: true},
		{name: "assets match", spec: `{"assets":[{"category":"runbook","name":"Runbook","url":"https://example.com/runbook"}]}`, wantSync: true},
		{name: "assets drift", spec: `{"assets":[{"category":"runbook","name":"Runbook","url":"https://example.com/new-runbook"}]}`, wantUpdates: 1, wantSync: true},
		{name: "nested option drift", spec: `{"name":"CPU","options":{"include_tags":false}}`, wantUpdates: 1, wantSync: true},
		{name: "explicit zero drift", spec: `{"options":{"new_host_delay":0}}`, wantUpdates: 1, wantSync: true},
		{name: "explicit empty set drift", spec: `{"tags":[]}`, wantUpdates: 1, wantSync: true},
		{name: "explicit threshold drift", spec: `{"options":{"thresholds":{"critical":90}}}`, wantUpdates: 1, wantSync: true},
		{name: "spec change still updates", spec: spec, wantUpdates: 1, wantSync: true},
		{name: "remote deletion recreates", spec: spec, getStatus: http.StatusNotFound, wantCreates: 1, wantSync: true},
		{name: "GET failure does not mark synced", spec: spec, getStatus: http.StatusInternalServerError},
		{name: "invalid spec retains update error handling", spec: `{`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gets, updates, creates int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodGet:
					assert.Equal(t, "true", r.URL.Query().Get("with_assets"))
					if tt.wantPolicyRead {
						assert.Equal(t, "true", r.URL.Query().Get("with_restriction_policy"))
					} else {
						assert.Empty(t, r.URL.Query().Get("with_restriction_policy"))
					}
					gets++
					if tt.getStatus != 0 {
						w.WriteHeader(tt.getStatus)
						fmt.Fprint(w, `{"errors":["remote error"]}`)
						return
					}
				case http.MethodPut:
					assert.Empty(t, r.URL.RawQuery, "comparison options must not leak into writes")
					if tt.remotePriority != "" {
						var payload map[string]any
						assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
						value, present := payload["priority"]
						assert.True(t, present, "priority clearing must be sent")
						if tt.name == "explicit null clears priority" {
							assert.Nil(t, value)
						} else {
							assert.Equal(t, float64(0), value)
						}
					}
					updates++
				case http.MethodPost:
					creates++
				}
				body := remote
				if tt.remotePriority != "" {
					body = strings.Replace(body, `"priority":null`, `"priority":`+tt.remotePriority, 1)
				}
				if r.URL.Query().Get("with_restriction_policy") == "true" {
					body = strings.Replace(body, `"id":12345`, `"id":12345,"restriction_policy":{"bindings":[{"relation":"editor","principals":["role:managed"]}]}`, 1)
				}
				if r.URL.Query().Get("with_assets") == "true" {
					body = strings.Replace(body, `"id":12345`, `"id":12345,"assets":[{"category":"runbook","name":"Runbook","url":"https://example.com/runbook"}]`, 1)
				}
				if r.Method == http.MethodPost {
					body = strings.Replace(body, "2026-01-01T00:00:00Z", time.Now().UTC().Format(time.RFC3339), 1)
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			t.Setenv("DD_URL", server.URL)
			t.Setenv("DD_API_KEY", "test-api-key")
			t.Setenv("DD_APP_KEY", "test-app-key")
			s := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(s))
			now := metav1.NewTime(time.Now().Truncate(time.Second))
			stale := metav1.NewTime(now.Add(-2 * defaultForceSyncPeriod))
			instance := &v1alpha1.DatadogGenericResource{
				ObjectMeta: metav1.ObjectMeta{Name: "cpu", Namespace: "default", Finalizers: []string{datadogGenericResourceFinalizer}},
				Spec:       v1alpha1.DatadogGenericResourceSpec{Type: v1alpha1.Monitor, JsonSpec: tt.spec},
				Status: v1alpha1.DatadogGenericResourceStatus{
					Id: "12345", LastForceSyncTime: &stale, StateLastUpdateTime: &now, State: "OK", SyncStatus: v1alpha1.DatadogSyncStatusUpdateError,
				},
			}
			condition.UpdateStatusConditions(&instance.Status.Conditions, now, condition.DatadogConditionTypeError, metav1.ConditionTrue, "UpdateError", "previous error")
			var err error
			instance.Status.CurrentHash, err = comparison.GenerateMD5ForSpec(&instance.Spec)
			require.NoError(t, err)
			if tt.name == "spec change still updates" {
				instance.Status.CurrentHash = "old hash"
			}
			c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(instance).WithObjects(instance).Build()
			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			r := &Reconciler{
				client: c, apiReader: c, credsManager: config.NewCredentialManager(c), recorder: record.NewFakeRecorder(10),
				requeuePeriod: time.Minute, forceSyncPeriod: defaultForceSyncPeriod,
				handlers: map[v1alpha1.SupportedResourcesType]ResourceHandler{v1alpha1.Monitor: &MonitorHandler{client: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg))}},
			}
			result, err := r.Reconcile(context.Background(), instance)
			require.NoError(t, err)
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(instance), instance))
			wantGets := 1
			if tt.name == "spec change still updates" {
				wantGets = 0
			}
			assert.Equal(t, wantGets, gets, "comparison reuses the existence GET")
			assert.Equal(t, tt.wantUpdates, updates)
			assert.Equal(t, tt.wantCreates, creates)
			assert.Equal(t, "OK", instance.Status.State, "force sync preserves state reporting")
			if tt.wantSync {
				require.NotNil(t, instance.Status.LastForceSyncTime)
				assert.True(t, instance.Status.LastForceSyncTime.After(stale.Time))
				assert.Equal(t, v1alpha1.DatadogSyncStatusOK, instance.Status.SyncStatus)
				for _, c := range instance.Status.Conditions {
					assert.NotEqual(t, condition.DatadogConditionTypeError, c.Type)
				}
				_, err = r.Reconcile(context.Background(), instance)
				require.NoError(t, err)
				assert.Equal(t, wantGets, gets)
				assert.Equal(t, tt.wantUpdates, updates)
			} else {
				assert.True(t, stale.Equal(instance.Status.LastForceSyncTime))
				if tt.name == "invalid spec retains update error handling" {
					assert.Equal(t, v1alpha1.DatadogSyncStatusUpdateError, instance.Status.SyncStatus)
					assert.Equal(t, defaultForceSyncPeriod, result.RequeueAfter)
				} else {
					assert.Equal(t, v1alpha1.DatadogSyncStatusGetError, instance.Status.SyncStatus)
				}
			}
		})
	}
}
