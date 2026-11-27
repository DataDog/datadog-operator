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
	"sync"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV2"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/datadogclient"
)

func TestOnCallReconcileLifecycle(t *testing.T) {
	for _, tc := range []struct {
		resourceType v1alpha1.SupportedResourcesType
		apiType      string
		apiPath      string
	}{
		{v1alpha1.OnCallSchedule, "schedules", "/api/v2/on-call/schedules"},
		{v1alpha1.OnCallEscalationPolicy, "policies", "/api/v2/on-call/escalation-policies"},
	} {
		t.Run(string(tc.resourceType), func(t *testing.T) {
			ctx := context.Background()
			var remote struct {
				sync.Mutex
				exists       bool
				id           string
				creates      int
				gets         int
				updates      int
				deletes      int
				deleteStatus int
				lastBody     map[string]any
			}
			remote.deleteStatus = http.StatusNoContent
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				remote.Lock()
				defer remote.Unlock()
				w.Header().Set("Content-Type", "application/json")
				if req.Method == http.MethodPost {
					assert.Equal(t, tc.apiPath, req.URL.Path)
				} else {
					assert.Equal(t, tc.apiPath+"/"+remote.id, req.URL.Path)
				}
				if req.Method == http.MethodPost || req.Method == http.MethodPut {
					if err := json.NewDecoder(req.Body).Decode(&remote.lastBody); !assert.NoError(t, err) {
						w.WriteHeader(http.StatusBadRequest)
						return
					}
				}
				writeError := func(status int) {
					w.WriteHeader(status)
					_, _ = w.Write([]byte(`{"errors":["On-Call request failed"]}`))
				}
				switch req.Method {
				case http.MethodPost:
					remote.creates++
					remote.id = fmt.Sprintf("on-call-%d", remote.creates)
					remote.exists = true
					w.WriteHeader(http.StatusCreated)
				case http.MethodGet:
					remote.gets++
					if !remote.exists {
						writeError(http.StatusNotFound)
						return
					}
				case http.MethodPut:
					remote.updates++
					if !remote.exists {
						writeError(http.StatusNotFound)
						return
					}
				case http.MethodDelete:
					remote.deletes++
					if remote.deleteStatus != http.StatusNoContent {
						writeError(remote.deleteStatus)
						return
					}
					remote.exists = false
					w.WriteHeader(http.StatusNoContent)
					return
				default:
					writeError(http.StatusMethodNotAllowed)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{
					"data": map[string]any{"id": remote.id, "type": tc.apiType},
				})
			}))
			defer server.Close()

			t.Setenv("DD_API_KEY", "DUMMY_API_KEY")
			t.Setenv("DD_APP_KEY", "DUMMY_APP_KEY")
			t.Setenv("DD_DD_URL", server.URL)
			t.Setenv("DD_URL", server.URL)

			s := runtime.NewScheme()
			require.NoError(t, v1alpha1.AddToScheme(s))
			c := fake.NewClientBuilder().WithScheme(s).
				WithStatusSubresource(&v1alpha1.DatadogGenericResource{}).Build()
			r := NewReconciler(c, config.NewCredentialManager(c), s, logr.Discard(), record.NewFakeRecorder(100))
			cfg := datadogapi.NewConfiguration()
			cfg.HTTPClient = server.Client()
			r.handlers = buildHandlers(&datadogclient.GenericClients{
				OnCallClient: datadogV2.NewOnCallApi(datadogapi.NewAPIClient(cfg)),
			})

			spec := func(name, team, suppliedID string) string {
				attributes := map[string]any{"name": name}
				if tc.resourceType == v1alpha1.OnCallSchedule {
					attributes["layers"] = []any{}
					attributes["time_zone"] = "UTC"
				} else {
					attributes["steps"] = []any{}
				}
				data := map[string]any{
					"type":       tc.apiType,
					"attributes": attributes,
					"relationships": map[string]any{
						"teams": map[string]any{"data": []any{map[string]any{"id": team, "type": "teams"}}},
					},
				}
				if suppliedID != "" {
					data["id"] = suppliedID
				}
				encoded, err := json.Marshal(map[string]any{"data": data})
				require.NoError(t, err)
				return string(encoded)
			}
			instance := &v1alpha1.DatadogGenericResource{
				ObjectMeta: metav1.ObjectMeta{Name: "on-call-lifecycle", Namespace: "default"},
				Spec: v1alpha1.DatadogGenericResourceSpec{
					Type: tc.resourceType, JsonSpec: spec("original", "team-original", ""),
				},
			}
			require.NoError(t, c.Create(ctx, instance))
			key := client.ObjectKeyFromObject(instance)
			load := func() {
				require.NoError(t, c.Get(ctx, key, instance))
			}
			reconcile := func() {
				load()
				_, err := r.Reconcile(ctx, instance)
				require.NoError(t, err)
				load()
			}
			assertSynced := func() {
				t.Helper()
				assert.Equal(t, v1alpha1.DatadogSyncStatusOK, instance.Status.SyncStatus)
				hash, err := comparison.GenerateMD5ForSpec(&instance.Spec)
				require.NoError(t, err)
				assert.Equal(t, hash, instance.Status.CurrentHash)
				require.NotNil(t, instance.Status.LastForceSyncTime)
			}
			assertRequest := func(name, team, id string) {
				t.Helper()
				remote.Lock()
				defer remote.Unlock()
				data, ok := remote.lastBody["data"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, tc.apiType, data["type"])
				if id != "" {
					assert.Equal(t, id, data["id"], "the persisted ID must override jsonSpec.data.id")
				}
				attributes, ok := data["attributes"].(map[string]any)
				require.True(t, ok)
				assert.Equal(t, name, attributes["name"])
				relationships, err := json.Marshal(data["relationships"])
				require.NoError(t, err)
				assert.JSONEq(t, fmt.Sprintf(`{"teams":{"data":[{"id":%q,"type":"teams"}]}}`, team), string(relationships))
			}
			expireForceSync := func() {
				t.Helper()
				instance.Status.LastForceSyncTime = &metav1.Time{Time: time.Now().Add(-2 * r.forceSyncPeriod)}
				require.NoError(t, c.Status().Update(ctx, instance))
			}

			reconcile() // Add the finalizer before making the first API request.
			assert.Contains(t, instance.Finalizers, datadogGenericResourceFinalizer)
			remote.Lock()
			assert.Zero(t, remote.creates)
			remote.Unlock()
			reconcile()
			assert.Equal(t, "on-call-1", instance.Status.Id)
			require.NotNil(t, instance.Status.Created)
			assertSynced()
			assertRequest("original", "team-original", "")

			instance.Spec.JsonSpec = spec("updated", "team-updated", "wrong-id")
			require.NoError(t, c.Update(ctx, instance))
			reconcile()
			assert.Equal(t, "on-call-1", instance.Status.Id)
			assertSynced()
			assertRequest("updated", "team-updated", "on-call-1")

			reconcile() // Idle requeues must not poll a live state for these types.
			assert.Empty(t, instance.Status.State)
			assert.Nil(t, instance.Status.StateLastUpdateTime)
			for _, cond := range instance.Status.Conditions {
				assert.NotEqual(t, "StateSynced", cond.Type)
			}
			remote.Lock()
			assert.Equal(t, 1, remote.creates)
			assert.Zero(t, remote.gets)
			assert.Equal(t, 1, remote.updates)
			remote.Unlock()

			expireForceSync()
			reconcile()
			assertSynced()
			assertRequest("updated", "team-updated", "on-call-1")
			remote.Lock()
			assert.Equal(t, 1, remote.gets)
			assert.Equal(t, 2, remote.updates)
			remote.exists = false
			remote.Unlock()

			expireForceSync()
			reconcile() // GET 404 recreates the remote resource.
			assert.Equal(t, "on-call-2", instance.Status.Id)
			assertSynced()

			remote.Lock()
			remote.exists = false
			remote.Unlock()
			instance.Spec.JsonSpec = spec("recreated-after-update", "team-updated", "")
			require.NoError(t, c.Update(ctx, instance))
			reconcile() // PUT 404 recreates immediately, even with a fresh force sync.
			assert.Equal(t, "on-call-3", instance.Status.Id)
			assertSynced()
			remote.Lock()
			assert.Equal(t, 3, remote.creates)
			assert.Equal(t, 2, remote.gets)
			assert.Equal(t, 3, remote.updates)
			remote.deleteStatus = http.StatusForbidden
			remote.Unlock()

			require.NoError(t, c.Delete(ctx, instance))
			load()
			_, err := r.Reconcile(ctx, instance)
			require.Error(t, err)
			load()
			assert.Contains(t, instance.Finalizers, datadogGenericResourceFinalizer, "failed cleanup must retain the CR")
			remote.Lock()
			remote.deleteStatus = http.StatusNotFound
			remote.Unlock()
			_, err = r.Reconcile(ctx, instance)
			require.NoError(t, err)
			err = c.Get(ctx, key, &v1alpha1.DatadogGenericResource{})
			assert.True(t, apierrors.IsNotFound(err), "404 cleanup must release the finalizer")
			remote.Lock()
			assert.Equal(t, 2, remote.deletes)
			remote.Unlock()
		})
	}
}
