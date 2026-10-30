// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package monitorpoller

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	datadogapi "github.com/DataDog/datadog-api-client-go/v2/api/datadog"
	"github.com/DataDog/datadog-api-client-go/v2/api/datadogV1"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/workqueue"
	"sigs.k8s.io/controller-runtime/pkg/controller/priorityqueue"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	ctrutils "github.com/DataDog/datadog-operator/pkg/controller/utils"
)

// fakeDatadog serves the monitor list and get endpoints for several
// organizations, identified by API key.
type fakeDatadog struct {
	t      *testing.T
	server *httptest.Server

	mu sync.Mutex
	// monitors holds the IDs of the monitors of each organization.
	monitors map[string]map[int64]bool
	// unlisted monitors exist but are missing from list responses.
	unlisted map[int64]bool
	// failList makes list requests that include the monitor fail.
	failList map[int64]int
	// listRequests holds the requested monitor IDs of each list request, per organization.
	listRequests map[string][][]int64
	getRequests  []int64
}

func newFakeDatadog(t *testing.T) *fakeDatadog {
	f := &fakeDatadog{
		t:            t,
		monitors:     map[string]map[int64]bool{},
		unlisted:     map[int64]bool{},
		failList:     map[int64]int{},
		listRequests: map[string][][]int64{},
	}
	f.server = httptest.NewServer(http.HandlerFunc(f.serveHTTP))
	t.Cleanup(f.server.Close)
	return f
}

func (f *fakeDatadog) addMonitors(org string, ids ...int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.monitors[org] == nil {
		f.monitors[org] = map[int64]bool{}
	}
	for _, id := range ids {
		f.monitors[org][id] = true
	}
}

func (f *fakeDatadog) serveHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	org := r.Header.Get("DD-API-KEY")
	assert.Equal(f.t, http.MethodGet, r.Method)
	assert.Equal(f.t, "alert,warn,no data", r.URL.Query().Get("group_states"))

	if r.URL.Path == "/api/v1/monitor" {
		query := r.URL.Query()
		var ids []int64
		for _, id := range strings.Split(query.Get("monitor_ids"), ",") {
			parsed, err := strconv.ParseInt(id, 10, 64)
			require.NoError(f.t, err)
			ids = append(ids, parsed)
		}
		assert.Equal(f.t, "0", query.Get("page"))
		assert.Equal(f.t, strconv.Itoa(len(ids)), query.Get("page_size"))
		f.listRequests[org] = append(f.listRequests[org], ids)

		for _, id := range ids {
			if status, ok := f.failList[id]; ok {
				w.WriteHeader(status)
				_, _ = w.Write([]byte(`{"errors":["injected failure"]}`))
				return
			}
		}
		monitors := []map[string]any{}
		for _, id := range ids {
			if f.monitors[org][id] && !f.unlisted[id] {
				monitors = append(monitors, monitorJSON(id))
			}
		}
		require.NoError(f.t, json.NewEncoder(w).Encode(monitors))
		return
	}

	id, err := strconv.ParseInt(strings.TrimPrefix(r.URL.Path, "/api/v1/monitor/"), 10, 64)
	require.NoError(f.t, err)
	f.getRequests = append(f.getRequests, id)
	if !f.monitors[org][id] {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"errors":["Monitor not found"]}`))
		return
	}
	require.NoError(f.t, json.NewEncoder(w).Encode(monitorJSON(id)))
}

func monitorJSON(id int64) map[string]any {
	return map[string]any{
		"id":            id,
		"name":          "monitor " + strconv.FormatInt(id, 10),
		"query":         "avg(last_5m):avg:system.cpu.user{*} by {host} > 80",
		"type":          "query alert",
		"overall_state": "Alert",
		"state": map[string]any{
			"groups": map[string]any{
				"host:a": map[string]any{"name": "host:a", "status": "Alert", "last_triggered_ts": 1700000000},
			},
		},
	}
}

func (f *fakeDatadog) auth(org string) context.Context {
	serverURL, err := url.Parse(f.server.URL)
	require.NoError(f.t, err)
	auth := context.WithValue(context.Background(), datadogapi.ContextAPIKeys, map[string]datadogapi.APIKey{
		"apiKeyAuth": {Key: org},
		"appKeyAuth": {Key: org + "-app"},
	})
	auth = context.WithValue(auth, datadogapi.ContextServerIndex, 1)
	return context.WithValue(auth, datadogapi.ContextServerVariables, map[string]string{
		"name":     serverURL.Host,
		"protocol": serverURL.Scheme,
	})
}

func (f *fakeDatadog) listedIDs(org string) [][]int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.listRequests[org])
}

// testPoller returns a poller on a fake clock that records notifications.
type testPoller struct {
	*Poller
	clock    time.Time
	notified map[types.NamespacedName]int
}

func newTestPoller(t *testing.T, f *fakeDatadog, opts Options) *testPoller {
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = f.server.Client()
	tp := &testPoller{
		Poller:   New(datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg)), logr.Discard(), opts),
		clock:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		notified: map[types.NamespacedName]int{},
	}
	tp.now = func() time.Time { return tp.clock }
	tp.notifiers["test"] = func(key types.NamespacedName) { tp.notified[key]++ }
	return tp
}

func (tp *testPoller) advance(d time.Duration) {
	tp.clock = tp.clock.Add(d)
}

func subscriber(id int64) Subscriber {
	return Subscriber{Consumer: "test", NamespacedName: types.NamespacedName{Namespace: "default", Name: "monitor-" + strconv.FormatInt(id, 10)}}
}

func TestPoller_PollsMonitorsInBatches(t *testing.T) {
	f := newFakeDatadog(t)
	tp := newTestPoller(t, f, Options{Interval: time.Minute})
	auth := f.auth("org-a")

	var ids []int64
	for id := int64(1); id <= 2500; id++ {
		ids = append(ids, id)
		tp.Register(auth, subscriber(id), id)
	}
	f.addMonitors("org-a", ids...)

	// Newly registered monitors wait briefly for more registrations to batch with.
	assert.Equal(t, tp.newMonitorDelay, tp.poll(context.Background()))
	assert.Empty(t, f.listedIDs("org-a"))

	tp.advance(tp.newMonitorDelay)
	tp.poll(context.Background())

	requests := f.listedIDs("org-a")
	require.Equal(t, 3, len(requests), "2500 monitors need three requests of at most 1000 monitors")
	assert.Equal(t, ids[:1000], requests[0])
	assert.Equal(t, ids[1000:2000], requests[1])
	assert.Equal(t, ids[2000:], requests[2])
	assert.Empty(t, f.getRequests)

	for _, id := range ids {
		result, ok := tp.Result(auth, id)
		require.True(t, ok)
		require.NoError(t, result.Err)
		assert.Equal(t, id, result.Monitor.GetId())
		assert.Equal(t, datadogV1.MONITOROVERALLSTATES_ALERT, result.Monitor.GetOverallState())
		assert.Equal(t, tp.clock, result.PolledAt)
		assert.Equal(t, 1, tp.notified[subscriber(id).NamespacedName])
	}

	// Every registered monitor is polled again one interval after the first
	// registration.
	tp.advance(time.Minute - tp.newMonitorDelay - time.Second)
	assert.Equal(t, time.Second, tp.poll(context.Background()))
	assert.Equal(t, 3, len(f.listedIDs("org-a")))
	tp.advance(time.Second)
	assert.Equal(t, time.Minute, tp.poll(context.Background()))
	assert.Equal(t, 6, len(f.listedIDs("org-a")))
	assert.Equal(t, 2, tp.notified[subscriber(1).NamespacedName])
}

func TestPoller_PartialFailures(t *testing.T) {
	f := newFakeDatadog(t)
	tp := newTestPoller(t, f, Options{BatchSize: 2})
	auth := f.auth("org-a")

	// Monitors 1 and 2 share a batch whose request fails; monitor 4 is missing
	// from the list response but exists, and monitor 3 was deleted.
	f.addMonitors("org-a", 1, 2, 4)
	f.failList[1] = http.StatusNotFound
	f.unlisted[4] = true
	for id := int64(1); id <= 4; id++ {
		tp.Register(auth, subscriber(id), id)
	}
	tp.advance(tp.newMonitorDelay)
	tp.poll(context.Background())

	for _, id := range []int64{1, 2} {
		result, ok := tp.Result(auth, id)
		require.True(t, ok)
		assert.ErrorContains(t, result.Err, "error listing monitors: HTTP status 404")
		assert.NotContains(t, result.Err.Error(), ctrutils.NotFoundString, "a failed list must not read as a deleted monitor")
		assert.True(t, ctrutils.IsPermanentAPIError(result.Err))
	}

	assert.Equal(t, []int64{3, 4}, f.getRequests, "monitors missing from the list are confirmed individually")
	deleted, ok := tp.Result(auth, 3)
	require.True(t, ok)
	assert.ErrorContains(t, deleted.Err, ctrutils.NotFoundString)
	unlisted, ok := tp.Result(auth, 4)
	require.True(t, ok)
	require.NoError(t, unlisted.Err)
	assert.Equal(t, int64(4), unlisted.Monitor.GetId())

	for id := int64(1); id <= 4; id++ {
		assert.Equal(t, 1, tp.notified[subscriber(id).NamespacedName], "subscribers are notified of failures too")
	}
}

func TestPoller_SeparatesCredentialScopes(t *testing.T) {
	f := newFakeDatadog(t)
	tp := newTestPoller(t, f, Options{})
	authA, authB := f.auth("org-a"), f.auth("org-b")

	f.addMonitors("org-a", 1, 2)
	f.addMonitors("org-b", 1, 3)
	tp.Register(authA, subscriber(1), 1)
	tp.Register(authA, subscriber(2), 2)
	otherOrg := Subscriber{Consumer: "test", NamespacedName: types.NamespacedName{Namespace: "other", Name: "monitor-1"}}
	tp.Register(authB, otherOrg, 1)
	tp.Register(authB, subscriber(3), 3)

	tp.advance(tp.newMonitorDelay)
	tp.poll(context.Background())

	assert.Equal(t, [][]int64{{1, 2}}, f.listedIDs("org-a"))
	assert.Equal(t, [][]int64{{1, 3}}, f.listedIDs("org-b"))
	_, ok := tp.Result(authA, 3)
	assert.False(t, ok, "results are scoped to the credentials that polled them")
	_, ok = tp.Result(authB, 2)
	assert.False(t, ok)
	for _, auth := range []context.Context{authA, authB} {
		result, ok := tp.Result(auth, 1)
		require.True(t, ok)
		assert.NoError(t, result.Err)
	}

	// A subscriber whose credentials change moves to the new scope.
	tp.Register(authB, subscriber(2), 2)
	assert.Len(t, tp.scopes, 2)
	_, ok = tp.Result(authA, 2)
	assert.False(t, ok)
	tp.Unregister(subscriber(1))
	assert.Len(t, tp.scopes, 1, "a scope without subscribers is dropped")
}

func TestPoller_Registrations(t *testing.T) {
	f := newFakeDatadog(t)
	tp := newTestPoller(t, f, Options{Interval: time.Minute})
	auth := f.auth("org-a")
	f.addMonitors("org-a", 1, 2, 3)

	tp.Register(auth, subscriber(1), 1)
	tp.advance(tp.newMonitorDelay)
	tp.poll(context.Background())
	assert.Equal(t, [][]int64{{1}}, f.listedIDs("org-a"))

	// Refreshing a registration does not poll again; a new monitor is polled
	// on its own, ahead of the next full poll.
	tp.Register(auth, subscriber(1), 1)
	tp.Register(auth, subscriber(2), 2)
	tp.advance(tp.newMonitorDelay)
	tp.poll(context.Background())
	assert.Equal(t, [][]int64{{1}, {2}}, f.listedIDs("org-a"))

	// Registering another monitor replaces the previous one.
	tp.Register(auth, subscriber(2), 3)
	_, ok := tp.Result(auth, 2)
	assert.False(t, ok, "monitors without subscribers are dropped")
	tp.advance(tp.newMonitorDelay)
	tp.poll(context.Background())
	assert.Equal(t, [][]int64{{1}, {2}, {3}}, f.listedIDs("org-a"))

	tp.Unregister(subscriber(1))
	tp.advance(time.Minute)
	tp.poll(context.Background())
	assert.Equal(t, []int64{3}, f.listedIDs("org-a")[3])

	// Registrations that are not refreshed expire.
	tp.advance(registrationTTLIntervals * time.Minute)
	tp.poll(context.Background())
	assert.Empty(t, tp.registrations)
	assert.Empty(t, tp.scopes)
	assert.Len(t, f.listedIDs("org-a"), 4)
}

func TestPoller_SourceEnqueuesAtLowPriority(t *testing.T) {
	f := newFakeDatadog(t)
	tp := newTestPoller(t, f, Options{})
	auth := f.auth("org-a")
	f.addMonitors("org-a", 1)

	queue := priorityqueue.New[reconcile.Request]("monitor-poller-test")
	defer queue.ShutDown()
	require.NoError(t, tp.Source("priority").Start(context.Background(), queue))
	plainQueue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer plainQueue.ShutDown()
	require.NoError(t, tp.Source("plain").Start(context.Background(), plainQueue))

	key := types.NamespacedName{Namespace: "default", Name: "monitor"}
	tp.Register(auth, Subscriber{Consumer: "priority", NamespacedName: key}, 1)
	tp.Register(auth, Subscriber{Consumer: "plain", NamespacedName: key}, 1)
	tp.advance(tp.newMonitorDelay)
	tp.poll(context.Background())

	request, priority, _ := queue.GetWithPriority()
	assert.Equal(t, reconcile.Request{NamespacedName: key}, request)
	assert.Equal(t, handler.LowPriority, priority)
	request, _ = plainQueue.Get()
	assert.Equal(t, reconcile.Request{NamespacedName: key}, request)
}

func TestPoller_Start(t *testing.T) {
	f := newFakeDatadog(t)
	cfg := datadogapi.NewConfiguration()
	cfg.HTTPClient = f.server.Client()
	p := New(datadogV1.NewMonitorsApi(datadogapi.NewAPIClient(cfg)), logr.Discard(), Options{Interval: time.Hour, NewMonitorDelay: 10 * time.Millisecond})
	queue := workqueue.NewTypedRateLimitingQueue(workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]())
	defer queue.ShutDown()
	require.NoError(t, p.Source("test").Start(context.Background(), queue))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error)
	go func() { done <- p.Start(ctx) }()

	// A registration wakes the worker, which polls the new monitor without
	// waiting for the hour-long interval.
	f.addMonitors("org-a", 1)
	p.Register(f.auth("org-a"), subscriber(1), 1)
	request, _ := queue.Get()
	assert.Equal(t, reconcile.Request{NamespacedName: subscriber(1).NamespacedName}, request)
	result, ok := p.Result(f.auth("org-a"), 1)
	require.True(t, ok)
	assert.NoError(t, result.Err)

	cancel()
	assert.NoError(t, <-done)
}
