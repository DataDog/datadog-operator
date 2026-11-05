// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogmonitor

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
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
)

func TestReconcileMonitorForceSync(t *testing.T) {
	const remote = `{"id":12345,"name":"CPU","message":"CPU high","query":"avg(last_5m):avg:system.cpu.user{*} > 80",
		"restricted_roles":["role"],"type":"query alert","priority":null,"overall_state":"Alert","tags":["team:a","generated:kubernetes"],
		"created":"2026-01-01T00:00:00Z","creator":{"email":"user@example.com"},
		"options":{"thresholds":{"critical":80.0,"warning":70},"include_tags":true,"new_host_delay":300,"notify_audit":false,"notify_by":["host"],"renotify_statuses":["alert"]}}`
	tests := []struct {
		name                      string
		mutate                    func(*v1alpha1.DatadogMonitor)
		getStatus                 int
		remotePriority            string
		wantValidate, wantUpdates int
		wantCreates               int
		wantSync                  bool
	}{
		{name: "no-op with server defaults and normalized values", wantSync: true},
		{name: "no-op with omitted priority set remotely", wantSync: true, mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Options.Thresholds = nil }},
		{name: "name drift", mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Name = "desired name" }, wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "explicit false drift", mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Options.IncludeTags = ptr.To(false) }, wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "explicit zero drift", mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Options.NewGroupDelay = ptr.To(int64(0)) }, wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "explicit zero matches canonical null", mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Priority = ptr.To(int64(0)) }, wantSync: true},
		{name: "explicit zero clears remote priority", mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Priority = ptr.To(int64(0)) }, remotePriority: "3", wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "omitted to empty roles is a spec change", wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "omitted to empty notify_by is a spec change", wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "omitted to empty renotify_statuses is a spec change", wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "omitted to empty tags is a spec change", mutate: func(dm *v1alpha1.DatadogMonitor) {
			dm.Spec.Tags = nil
			dm.Spec.ControllerOptions.DisableRequiredTags = ptr.To(true)
		}, wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "omitted to zero is a spec change", remotePriority: "3", wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "priority drift", mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Priority = ptr.To(int64(1)) }, wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "spec change still updates", wantValidate: 1, wantUpdates: 1, wantSync: true},
		{name: "remote deletion recreates", getStatus: http.StatusNotFound, wantValidate: 1, wantCreates: 1},
		{name: "GET failure does not mark synced", getStatus: http.StatusInternalServerError},
		{name: "missing required tag updates CR", mutate: func(dm *v1alpha1.DatadogMonitor) { dm.Spec.Tags = nil }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var gets, validates, updates, creates int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.Method {
				case http.MethodGet:
					gets++
					if tt.getStatus != 0 {
						w.WriteHeader(tt.getStatus)
						fmt.Fprint(w, `{"errors":["remote error"]}`)
						return
					}
				case http.MethodPut:
					if tt.name == "explicit zero clears remote priority" || tt.name == "omitted to zero is a spec change" {
						var payload map[string]any
						assert.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
						assert.Equal(t, float64(0), payload["priority"])
					}
					updates++
				case http.MethodPost:
					if r.URL.Path == "/api/v1/monitor/validate" {
						validates++
						fmt.Fprint(w, `{}`)
						return
					}
					creates++
				}
				body := remote
				if tt.remotePriority != "" {
					body = strings.Replace(body, `"priority":null`, `"priority":`+tt.remotePriority, 1)
				}
				if tt.name == "no-op with omitted priority set remotely" {
					body = strings.Replace(body, `"priority":null`, `"priority":3`, 1)
				}
				fmt.Fprint(w, body)
			}))
			defer server.Close()
			t.Setenv("DD_URL", server.URL)
			t.Setenv("DD_API_KEY", "test-api-key")
			t.Setenv("DD_APP_KEY", "test-app-key")
			t.Setenv(DDMonitorForceSyncPeriodEnvVar, "60")
			s := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(s))
			now := metav1.NewTime(time.Now().Truncate(time.Second))
			stale := metav1.NewTime(now.Add(-2 * defaultForceSyncPeriod))
			dm := &v1alpha1.DatadogMonitor{
				ObjectMeta: metav1.ObjectMeta{Name: "cpu", Namespace: "default", Finalizers: []string{datadogMonitorFinalizer}},
				Spec: v1alpha1.DatadogMonitorSpec{
					Name: "CPU", Message: "CPU high\n", Query: "avg(last_5m):avg:system.cpu.user{*} > 80", Type: v1alpha1.DatadogMonitorTypeMetric,
					Tags:    []string{"generated:kubernetes", "team:a"},
					Options: v1alpha1.DatadogMonitorOptions{Thresholds: &v1alpha1.DatadogMonitorOptionsThresholds{Critical: ptr.To("80")}},
				},
				Status: v1alpha1.DatadogMonitorStatus{
					ID: 12345, MonitorLastForceSyncTime: &stale, MonitorStateLastUpdateTime: &now,
					MonitorState: v1alpha1.DatadogMonitorStateOK, MonitorStateSyncStatus: v1alpha1.MonitorStateSyncStatusUpdateError,
				},
			}
			if tt.mutate != nil {
				tt.mutate(dm)
			}
			var err error
			dm.Status.CurrentHash, err = comparison.GenerateMD5ForSpec(&dm.Spec)
			require.NoError(t, err)
			switch tt.name {
			case "omitted to zero is a spec change":
				dm.Spec.Priority = ptr.To(int64(0))
			case "omitted to empty roles is a spec change":
				dm.Spec.RestrictedRoles = []string{}
			case "omitted to empty notify_by is a spec change":
				dm.Spec.Options.NotifyBy = []string{}
			case "omitted to empty renotify_statuses is a spec change":
				dm.Spec.Options.RenotifyStatuses = []datadogV1.MonitorRenotifyStatusType{}
			case "omitted to empty tags is a spec change":
				dm.Spec.Tags = []string{}
			}
			if strings.HasPrefix(tt.name, "omitted to ") {
				// Prove edits trigger immediate updates rather than waiting for
				// the next force sync or state poll.
				dm.Status.MonitorLastForceSyncTime = &now
			}
			if tt.name == "spec change still updates" {
				dm.Status.CurrentHash = "old hash"
			}
			c := fake.NewClientBuilder().WithScheme(s).WithStatusSubresource(dm).WithObjects(dm).Build()
			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			r := &Reconciler{
				client: c, apiReader: c, datadogClient: datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg)),
				credsManager: config.NewCredentialManager(c), recorder: record.NewFakeRecorder(10), requeuePeriod: time.Minute,
			}
			_, err = r.Reconcile(context.Background(), dm)
			require.NoError(t, err)
			require.NoError(t, c.Get(context.Background(), client.ObjectKeyFromObject(dm), dm))
			assert.Equal(t, 1, gets)
			assert.Equal(t, tt.wantValidate, validates)
			assert.Equal(t, tt.wantUpdates, updates)
			assert.Equal(t, tt.wantCreates, creates)
			if tt.wantSync {
				require.NotNil(t, dm.Status.MonitorLastForceSyncTime)
				assert.True(t, dm.Status.MonitorLastForceSyncTime.After(stale.Time))
				assert.Equal(t, v1alpha1.MonitorStateSyncStatusOK, dm.Status.MonitorStateSyncStatus)
				// A successful force sync must not immediately fetch and write again.
				_, err = r.Reconcile(context.Background(), dm)
				require.NoError(t, err)
				assert.Equal(t, 1, gets)
				assert.Equal(t, tt.wantUpdates, updates)
			} else if tt.wantCreates == 0 {
				assert.True(t, stale.Equal(dm.Status.MonitorLastForceSyncTime))
			}
			assert.Equal(t, v1alpha1.DatadogMonitorStateOK, dm.Status.MonitorState, "force sync preserves state reporting")
			if tt.name == "missing required tag updates CR" {
				assert.Contains(t, dm.Spec.Tags, requiredTag)
			}
		})
	}
}

func TestMonitorPriorityPresence(t *testing.T) {
	var omitted, zero v1alpha1.DatadogMonitor
	require.NoError(t, json.Unmarshal([]byte(`{"spec":{}}`), &omitted))
	require.NoError(t, json.Unmarshal([]byte(`{"spec":{"priority":0}}`), &zero))
	require.Nil(t, omitted.Spec.Priority)
	require.NotNil(t, zero.Spec.Priority)
	hashOmitted, err := comparison.GenerateMD5ForSpec(&omitted.Spec)
	require.NoError(t, err)
	hashZero, err := comparison.GenerateMD5ForSpec(&zero.Spec)
	require.NoError(t, err)
	assert.NotEqual(t, hashOmitted, hashZero, "adding explicit zero must trigger a spec update")
	copy := zero.DeepCopy()
	*copy.Spec.Priority = 3
	assert.Zero(t, *zero.Spec.Priority, "a copied CR must own its priority pointer")
}

func TestMonitorEmptySetPresence(t *testing.T) {
	omitted := &v1alpha1.DatadogMonitor{}
	omittedHash, err := comparison.GenerateMD5ForSpec(&omitted.Spec)
	require.NoError(t, err)
	for _, input := range []string{
		`{"spec":{"restrictedRoles":[]}}`,
		`{"spec":{"tags":[]}}`,
		`{"spec":{"options":{"notifyBy":[]}}}`,
		`{"spec":{"options":{"renotifyStatuses":[]}}}`,
	} {
		t.Run(input, func(t *testing.T) {
			var dm v1alpha1.DatadogMonitor
			require.NoError(t, json.Unmarshal([]byte(input), &dm))
			hash, err := comparison.GenerateMD5ForSpec(&dm.Spec)
			require.NoError(t, err)
			assert.NotEqual(t, omittedHash, hash, "explicit [] must affect the spec hash")
			// API reads/writes and fake clients must preserve that same ownership,
			// including when a required-tag update writes the typed resource.
			data, err := json.Marshal(&dm)
			require.NoError(t, err)
			var roundTrip v1alpha1.DatadogMonitor
			require.NoError(t, json.Unmarshal(data, &roundTrip))
			assert.Equal(t, dm.Spec, roundTrip.Spec)
		})
	}
}
