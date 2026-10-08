// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	k8sruntime "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/apimachinery/pkg/watch"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"
	"k8s.io/client-go/tools/cache"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/pkg/plugin/helm"
)

var gvrDaemonSets = schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}

const waitTimeout = 10 * time.Second

// testClock is a clock the test moves.
type testClock struct{ t atomic.Int64 }

func newTestClock() *testClock {
	c := &testClock{}
	c.set(testNow)
	return c
}

func (c *testClock) set(t time.Time) { c.t.Store(t.UnixNano()) }
func (c *testClock) now() time.Time  { return time.Unix(0, c.t.Load()).UTC() }

func startInformer(t *testing.T, clients ListClients, cfg InformerConfig) *InformerStore {
	t.Helper()
	if cfg.Now == nil {
		cfg.Now = func() time.Time { return testNow }
	}
	s := NewInformerStore(clients, cfg)
	require.NoError(t, s.Start(context.Background()))
	t.Cleanup(s.Close)
	require.NotNil(t, s.Changed())
	return s
}

// waitSnapshot waits until cond holds for the current snapshot and returns
// it.
func waitSnapshot(t *testing.T, s *InformerStore, what string, cond func(*Snapshot) bool) *Snapshot {
	t.Helper()
	var snap *Snapshot
	require.Eventually(t, func() bool {
		snap = s.Snapshot()
		return cond(snap)
	}, waitTimeout, 10*time.Millisecond, "waiting for %s", what)
	return snap
}

// settled reports whether every key is acquired (not Loading) and every
// release has a history.
func settled(keys []string, releases int) func(*Snapshot) bool {
	return func(snap *Snapshot) bool {
		for _, key := range keys {
			info, ok := snap.Sources[key]
			if !ok || info.State == SourceLoading {
				return false
			}
		}
		return len(snap.Helm) >= releases
	}
}

func sourceKeys(snap *Snapshot) []string {
	keys := make([]string, 0, len(snap.Sources))
	for key := range snap.Sources {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// comparable drops the parts of a View that depend on how sources are
// refreshed, and returns its JSON.
func comparableView(t *testing.T, v View) string {
	t.Helper()
	v.Sources = nil
	v.Header.NextPoll = nil
	out, err := json.MarshalIndent(v, "", "  ")
	require.NoError(t, err)
	return string(out)
}

// The InformerStore serves the same data as the ListStore.
func TestInformerStoreMatchesListStore(t *testing.T) {
	for _, pods := range []bool{false, true} {
		t.Run(fmt.Sprintf("pods=%v", pods), func(t *testing.T) {
			cfg := ListConfig{Namespace: "datadog", Pods: pods, Cluster: ClusterInfo{Context: "kind-ddtui", PluginVersion: "v1"}}
			want := startList(t, newListFixture(t, listObjects), cfg)

			f := newListFixture(t, listObjects)
			s := startInformer(t, f.clients(), InformerConfig{ListConfig: cfg})
			got := waitSnapshot(t, s, "all sources", settled(sourceKeys(want), len(want.Helm)))

			assert.Equal(t, sourceKeys(want), sourceKeys(got))
			assert.Equal(t, want.Cluster, got.Cluster)
			for key, info := range want.Sources {
				assert.Equal(t, info.State, got.Sources[key].State, key)
				assert.Equal(t, info.Notice, got.Sources[key].Notice, key)
				assert.Equal(t, names(want.Objects[key]), names(got.Objects[key]), key)
			}
			assert.Len(t, got.Pods, len(want.Pods))
			assert.Equal(t, want.Helm, got.Helm)

			// Watched and polled sources.
			assert.Equal(t, RefreshWatch, got.Sources[SourceDaemonSets].Mode)
			assert.True(t, got.Sources[SourceDaemonSets].NextPoll.IsZero())
			for _, key := range []string{SourceOperator, SourceLease, "sum/DatadogMonitor"} {
				assert.Equal(t, RefreshPoll, got.Sources[key].Mode, key)
				assert.Equal(t, testNow.Add(DefaultPollInterval), got.Sources[key].NextPoll, key)
			}

			cfgBuild := BuildConfig{Pods: pods}
			wantView, err := Build(want, cfgBuild, nil, testNow)
			require.NoError(t, err)
			gotView, err := Build(got, cfgBuild, nil, testNow)
			require.NoError(t, err)
			assert.Equal(t, comparableView(t, wantView), comparableView(t, gotView))
			require.NotNil(t, gotView.Header.NextPoll)
			assert.Equal(t, testNow.Add(DefaultPollInterval), *gotView.Header.NextPoll)
		})
	}
}

// fixtureClients serves the objects of a fixture snapshot through fake
// clients. Agent workloads and pods get the DDA label the operator sets, so
// that the label selectors match them.
func fixtureClients(t *testing.T, snap *Snapshot) (ListClients, ListConfig) {
	t.Helper()
	var ddaName, ddaNS string
	if ddas := snap.Objects[SourceDDA]; len(ddas) > 0 {
		ddaName, ddaNS = ddas[0].GetName(), ddas[0].GetNamespace()
	}
	label := func(obj metav1.Object) {
		labels := obj.GetLabels()
		if labels == nil {
			labels = map[string]string{}
		}
		if _, ok := labels[apicommon.AgentDeploymentNameLabelKey]; !ok {
			labels[apicommon.AgentDeploymentNameLabelKey] = ddaName
		}
		obj.SetLabels(labels)
	}

	listKinds := map[schema.GroupVersionResource]string{}
	resources := map[string][]metav1.APIResource{}
	var objs []k8sruntime.Object
	for _, src := range Sources() {
		listKinds[src.GVR] = src.Kind + "List"
		if info, ok := snap.Sources[src.Key]; !ok || info.State != SourceNotInstalled {
			gv := src.GVR.GroupVersion().String()
			if !slices.ContainsFunc(resources[gv], func(r metav1.APIResource) bool { return r.Name == src.GVR.Resource }) {
				resources[gv] = append(resources[gv], metav1.APIResource{Name: src.GVR.Resource, Kind: src.Kind, Namespaced: true})
			}
		}
		for _, o := range snap.Objects[src.Key] {
			o = o.DeepCopy()
			if src.Selector != nil && src.Key != SourceOperator {
				label(o)
			}
			objs = append(objs, o)
		}
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(k8sruntime.NewScheme(), listKinds, objs...)

	var pods []k8sruntime.Object
	for _, p := range snap.Pods {
		p = p.DeepCopy()
		label(p)
		pods = append(pods, p)
	}
	kube := kubefake.NewClientset(pods...)
	disco := kube.Discovery().(*fakediscovery.FakeDiscovery)
	disco.FakedServerVersion = &version.Info{GitVersion: snap.Cluster.ServerVersion}
	for gv, rs := range resources {
		disco.Resources = append(disco.Resources, &metav1.APIResourceList{GroupVersion: gv, APIResources: rs})
	}
	clients := ListClients{
		Dynamic: dyn,
		Kube:    kube,
		HelmHistory: func(_ context.Context, namespace, release string) ([]helm.Revision, error) {
			if revs, ok := snap.Helm[ReleaseKey(namespace, release)]; ok {
				return revs, nil
			}
			return nil, helm.ErrReleaseNotFound
		},
	}
	cfg := ListConfig{
		Namespace: ddaNS,
		Pods:      snap.Pods != nil,
		Cluster:   ClusterInfo{Context: snap.Cluster.Context, PluginVersion: snap.Cluster.PluginVersion},
	}
	return clients, cfg
}

// The Build assertions of the fixture scenarios hold for the InformerStore
// fed with the same objects (shared test table).
func TestInformerStoreFixtures(t *testing.T) {
	scenarios := []string{"steady", "profiles", "reconcile-errors", "helm", "stalled", "no-dap-crd", "dap-no-status", "stale-daps", "stale-warnings"}
	for _, name := range scenarios {
		t.Run(name, func(t *testing.T) {
			fixture, err := LoadFixture(filepath.Join("testdata", name))
			require.NoError(t, err)
			buildCfg := FixtureBuildConfig(name)
			want, err := Build(fixture, buildCfg, NopObserver{}, testNow)
			require.NoError(t, err)

			clients, cfg := fixtureClients(t, fixture)
			s := startInformer(t, clients, InformerConfig{ListConfig: cfg})
			keys := []string{SourceDDA, SourceDDAI, SourceDAP, SourceDaemonSets, SourceDeployments, SourceControllerRevisions, SourceReplicaSets, SourceOperator}
			if cfg.Pods {
				keys = append(keys, SourcePods)
			}
			snap := waitSnapshot(t, s, "all sources", settled(keys, len(fixture.Helm)))
			got, err := Build(snap, buildCfg, NopObserver{}, testNow)
			require.NoError(t, err)
			assert.Equal(t, comparableView(t, want), comparableView(t, got))
		})
	}
}

func TestInformerStoreTransforms(t *testing.T) {
	objs := listObjects + `
---
apiVersion: apps/v1
kind: ReplicaSet
metadata:
  name: datadog-agent-cluster-agent-abc
  namespace: datadog
  labels: {agent.datadoghq.com/name: datadog-agent}
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: '{"secret": "value"}'
  managedFields:
    - manager: kube-controller-manager
`
	f := newListFixture(t, objs)
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog", Pods: true}})
	snap := waitSnapshot(t, s, "pods and replica sets", settled([]string{SourcePods, SourceReplicaSets, SourceControllerRevisions, SourceDDA}, 0))

	assert.Nil(t, snap.Objects[SourceDDA][0].GetManagedFields())
	rs := snap.Objects[SourceReplicaSets][0]
	assert.Nil(t, rs.GetManagedFields())
	assert.NotContains(t, rs.GetAnnotations(), lastAppliedAnnotation)
	_, hasData := snap.Objects[SourceControllerRevisions][0].Object["data"]
	assert.False(t, hasData)

	require.Len(t, snap.Pods, 1)
	pod := snap.Pods[0]
	assert.Equal(t, "node-1", pod.Spec.NodeName)
	assert.Empty(t, pod.Spec.Containers, "pods are stripped")
	assert.Nil(t, pod.ManagedFields)
	assert.Equal(t, "datadog-agent", pod.Labels[apicommon.AgentDeploymentNameLabelKey])
}

// A burst of events wakes a reader only a few times: Changed holds at most
// one pending signal.
func TestInformerStoreCoalescesEvents(t *testing.T) {
	f := newListFixture(t, listObjects)
	burst := watch.NewFakeWithChanSize(1100, false)
	var served atomic.Bool
	f.dynamic.PrependWatchReactor("daemonsets", func(k8stesting.Action) (bool, watch.Interface, error) {
		if served.CompareAndSwap(false, true) {
			return true, burst, nil
		}
		return false, nil, nil
	})
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	waitSnapshot(t, s, "daemon sets", settled([]string{SourceDaemonSets}, 0))
	require.Eventually(t, served.Load, waitTimeout, 10*time.Millisecond)

	// A slow reader, as the TUI waits out a gap after each wake-up.
	var wakeups atomic.Int32
	done := make(chan struct{})
	readerDone := make(chan struct{})
	go func() {
		defer close(readerDone)
		for {
			select {
			case <-done:
				return
			case <-s.Changed():
				wakeups.Add(1)
				time.Sleep(20 * time.Millisecond)
			}
		}
	}()
	time.Sleep(50 * time.Millisecond)
	wakeups.Store(0)

	ds := decodeObjects(t, listObjects)[5]
	require.Equal(t, "DaemonSet", ds.GetKind())
	for i := range 1000 {
		obj := ds.DeepCopy()
		obj.SetResourceVersion(fmt.Sprint(i + 1))
		burst.Modify(obj)
	}
	last := ds.DeepCopy()
	last.SetName("burst-end")
	burst.Add(last)
	waitSnapshot(t, s, "the end of the burst", func(snap *Snapshot) bool {
		return slices.Contains(names(snap.Objects[SourceDaemonSets]), "datadog/burst-end")
	})
	time.Sleep(50 * time.Millisecond)
	close(done)
	<-readerDone
	t.Logf("%d wake-ups for 1001 events", wakeups.Load())
	assert.LessOrEqual(t, wakeups.Load(), int32(50), "1001 events must coalesce")
	assert.Positive(t, wakeups.Load())
	// With no reader, at most one signal is pending.
	pending := 0
	for {
		select {
		case <-s.Changed():
			pending++
			continue
		default:
		}
		break
	}
	assert.LessOrEqual(t, pending, 1)
}

// A source whose watch is forbidden but whose list is allowed is polled,
// with a countdown.
func TestInformerStoreWatchForbiddenPolls(t *testing.T) {
	f := newListFixture(t, listObjects)
	f.dynamic.PrependWatchReactor("daemonsets", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, nil, apierrors.NewForbidden(gvrDaemonSets.GroupResource(), "", errors.New("watch denied"))
	})
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	snap := waitSnapshot(t, s, "daemon sets polled", func(snap *Snapshot) bool {
		info := snap.Sources[SourceDaemonSets]
		return info.Mode == RefreshPoll && info.State == SourceOK
	})
	assert.Equal(t, testNow.Add(DefaultFallbackInterval), snap.Sources[SourceDaemonSets].NextPoll)
	assert.Equal(t, []string{"datadog/canary-agent", "datadog/datadog-agent-agent"}, names(snap.Objects[SourceDaemonSets]))
	assert.Equal(t, RefreshWatch, snap.Sources[SourceDeployments].Mode)
}

func TestInformerStoreForbiddenList(t *testing.T) {
	t.Run("cluster-wide DAP list falls back to the DDA namespace", func(t *testing.T) {
		f := newListFixture(t, listObjects)
		f.forbid(gvrDAP, true)
		s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
		snap := waitSnapshot(t, s, "DAPs in the DDA namespace", func(snap *Snapshot) bool {
			return snap.Sources[SourceDAP].State == SourceOK && snap.Sources[SourceDAP].Notice != ""
		})
		info := snap.Sources[SourceDAP]
		assert.Equal(t, RefreshWatch, info.Mode)
		assert.Equal(t, "DatadogAgentProfiles: namespace datadog only (cluster-wide list forbidden)", info.Notice)
		assert.Equal(t, []string{"datadog/local"}, names(snap.Objects[SourceDAP]))
	})
	t.Run("DAP list forbidden everywhere", func(t *testing.T) {
		f := newListFixture(t, listObjects)
		f.forbid(gvrDAP, false)
		s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
		snap := waitSnapshot(t, s, "DAPs forbidden", func(snap *Snapshot) bool {
			return snap.Sources[SourceDAP].State == SourceForbidden
		})
		assert.Contains(t, snap.Sources[SourceDAP].Err, "denied")
		assert.Empty(t, snap.Objects[SourceDAP])
		assert.Equal(t, SourceOK, waitSnapshot(t, s, "daemon sets", settled([]string{SourceDaemonSets}, 0)).Sources[SourceDaemonSets].State)
	})
	t.Run("forbidden DDA does not block Start", func(t *testing.T) {
		f := newListFixture(t, listObjects)
		f.forbid(gvrDDA, false)
		s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
		snap := waitSnapshot(t, s, "DDA forbidden", func(snap *Snapshot) bool {
			return snap.Sources[SourceDDA].State == SourceForbidden
		})
		_, err := Build(snap, BuildConfig{}, nil, testNow)
		require.ErrorContains(t, err, "Forbidden")
	})
	t.Run("forbidden summary kind", func(t *testing.T) {
		f := newListFixture(t, listObjects)
		f.forbid(gvrMonitor, false)
		s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
		snap := waitSnapshot(t, s, "monitors forbidden", func(snap *Snapshot) bool {
			return snap.Sources["sum/DatadogMonitor"].State == SourceForbidden
		})
		assert.Equal(t, RefreshPoll, snap.Sources["sum/DatadogMonitor"].Mode)
	})
}

// A failing watch marks a synced source Disconnected, keeping its objects
// and the time of its last event, until a request succeeds again.
func TestInformerStoreDisconnected(t *testing.T) {
	f := newListFixture(t, listObjects)
	first := watch.NewFake()
	var served, failing atomic.Bool
	f.dynamic.PrependWatchReactor("daemonsets", func(k8stesting.Action) (bool, watch.Interface, error) {
		switch {
		case failing.Load():
			return true, nil, errors.New("connection lost")
		case served.CompareAndSwap(false, true):
			return true, first, nil
		}
		return false, nil, nil
	})
	f.dynamic.PrependReactor("list", "daemonsets", func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		if failing.Load() {
			return true, nil, errors.New("connection lost")
		}
		return false, nil, nil
	})
	clock := newTestClock()
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog", Now: clock.now}})
	waitSnapshot(t, s, "daemon sets", settled([]string{SourceDaemonSets}, 0))
	require.Eventually(t, served.Load, waitTimeout, 10*time.Millisecond)

	// An event two hours after the sync advances LastOK.
	eventAt := testNow.Add(2 * time.Hour)
	clock.set(eventAt)
	ds := decodeObjects(t, listObjects)[5]
	require.Equal(t, "DaemonSet", ds.GetKind())
	ds.SetResourceVersion("100")
	first.Modify(ds)
	waitSnapshot(t, s, "the event", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].LastOK.Equal(eventAt)
	})

	clock.set(eventAt.Add(time.Minute))
	failing.Store(true)
	first.Stop()
	snap := waitSnapshot(t, s, "daemon sets disconnected", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].State == SourceDisconnected
	})
	assert.Contains(t, snap.Sources[SourceDaemonSets].Err, "connection lost")
	assert.Len(t, snap.Objects[SourceDaemonSets], 2, "stale objects are kept")
	assert.WithinDuration(t, eventAt, snap.Sources[SourceDaemonSets].LastOK, 0, "LastOK is the last event")

	failing.Store(false)
	snap = waitSnapshot(t, s, "daemon sets recovered", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].State == SourceOK
	})
	assert.Empty(t, snap.Sources[SourceDaemonSets].Err)
	assert.Equal(t, RefreshWatch, snap.Sources[SourceDaemonSets].Mode)
}

// Objects that appear or disappear reach the snapshot without a restart,
// and a second DDA shows the selection warning.
func TestInformerStoreLiveChanges(t *testing.T) {
	f := newListFixture(t, listObjects)
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{AllNamespaces: true}})
	waitSnapshot(t, s, "DAPs", settled([]string{SourceDAP}, 0))

	ctx := context.Background()
	dap := &unstructured.Unstructured{}
	dap.SetAPIVersion("datadoghq.com/v1alpha1")
	dap.SetKind("DatadogAgentProfile")
	dap.SetNamespace("team-b")
	dap.SetName("gpu")
	_, err := f.dynamic.Resource(gvrDAP).Namespace("team-b").Create(ctx, dap, metav1.CreateOptions{})
	require.NoError(t, err)
	waitSnapshot(t, s, "new DAP", func(snap *Snapshot) bool {
		return slices.Contains(names(snap.Objects[SourceDAP]), "team-b/gpu")
	})
	require.NoError(t, f.dynamic.Resource(gvrDAP).Namespace("team-b").Delete(ctx, "gpu", metav1.DeleteOptions{}))
	waitSnapshot(t, s, "deleted DAP", func(snap *Snapshot) bool {
		return !slices.Contains(names(snap.Objects[SourceDAP]), "team-b/gpu")
	})

	second := &unstructured.Unstructured{}
	second.SetAPIVersion("datadoghq.com/v2alpha1")
	second.SetKind("DatadogAgent")
	second.SetNamespace("other")
	second.SetName("second")
	_, err = f.dynamic.Resource(gvrDDA).Namespace("other").Create(ctx, second, metav1.CreateOptions{})
	require.NoError(t, err)
	snap := waitSnapshot(t, s, "second DDA", func(snap *Snapshot) bool { return len(snap.Objects[SourceDDA]) == 2 })
	v, err := Build(snap, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, []string{"datadog/datadog-agent", "other/second"}, v.MultipleDDAs)

	// Deleting the first DDA moves the selection, and the other sources
	// follow it to its namespace.
	require.NoError(t, f.dynamic.Resource(gvrDDA).Namespace("datadog").Delete(ctx, "datadog-agent", metav1.DeleteOptions{}))
	snap = waitSnapshot(t, s, "sources of the second DDA", func(snap *Snapshot) bool {
		return len(snap.Objects[SourceDDA]) == 1 && snap.Sources[SourceDaemonSets].State == SourceOK && len(snap.Objects[SourceDaemonSets]) == 0
	})
	v, err = Build(snap, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, "second", v.DDA.Name)
}

// Helm histories are read once, and again only while a rollout is in
// progress or when the chart changes.
func TestInformerStoreHelmPolling(t *testing.T) {
	f := newListFixture(t, listObjects)
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}, HelmInterval: 20 * time.Millisecond})
	waitSnapshot(t, s, "Helm history", settled([]string{SourceDaemonSets, SourceDeployments}, 1))
	helmCalls := func() int {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.helmCalls)
	}
	require.Never(t, func() bool { return helmCalls() > 1 }, 200*time.Millisecond, 10*time.Millisecond, "steady state: no Helm polling")

	ds, err := f.dynamic.Resource(gvrDaemonSets).Namespace("datadog").Get(context.Background(), "datadog-agent-agent", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(ds.Object, int64(1), "status", "updatedNumberScheduled"))
	_, err = f.dynamic.Resource(gvrDaemonSets).Namespace("datadog").Update(context.Background(), ds, metav1.UpdateOptions{})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return helmCalls() >= 3 }, waitTimeout, 10*time.Millisecond, "Helm polled while rolling")
}

// An unavailable agent pod alone is not a rollout: Helm is not polled.
func TestInformerStoreHelmNotPolledForUnavailablePods(t *testing.T) {
	f := newListFixture(t, listObjects)
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}, HelmInterval: 20 * time.Millisecond})
	waitSnapshot(t, s, "Helm history", settled([]string{SourceDaemonSets, SourceDeployments}, 1))
	ds, err := f.dynamic.Resource(gvrDaemonSets).Namespace("datadog").Get(context.Background(), "datadog-agent-agent", metav1.GetOptions{})
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(ds.Object, int64(1), "status", "numberUnavailable"))
	_, err = f.dynamic.Resource(gvrDaemonSets).Namespace("datadog").Update(context.Background(), ds, metav1.UpdateOptions{})
	require.NoError(t, err)
	waitSnapshot(t, s, "the unavailable pod", func(snap *Snapshot) bool {
		for _, d := range snap.Objects[SourceDaemonSets] {
			if field(d, "status", "numberUnavailable") == 1 {
				return true
			}
		}
		return false
	})
	f.mu.Lock()
	calls := len(f.helmCalls)
	f.mu.Unlock()
	require.Never(t, func() bool {
		f.mu.Lock()
		defer f.mu.Unlock()
		return len(f.helmCalls) > calls
	}, 200*time.Millisecond, 10*time.Millisecond)
}

// A Helm read that fails is retried even while steady.
func TestInformerStoreHelmRetriesFailedRead(t *testing.T) {
	f := newListFixture(t, listObjects)
	clients := f.clients()
	var calls atomic.Int32
	read := clients.HelmHistory
	clients.HelmHistory = func(ctx context.Context, namespace, release string) ([]helm.Revision, error) {
		if calls.Add(1) == 1 {
			return nil, errors.New("connection lost")
		}
		return read(ctx, namespace, release)
	}
	s := startInformer(t, clients, InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}, HelmInterval: 20 * time.Millisecond})
	waitSnapshot(t, s, "Helm history after a failed read", settled([]string{SourceDDA, SourceDAP}, 1))
	assert.GreaterOrEqual(t, calls.Load(), int32(2))
}

// Close does not wait for a Helm read that ignores ctx, as the Helm
// storage drivers do while the API server is unreachable.
func TestInformerStoreCloseDoesNotWaitForHelm(t *testing.T) {
	f := newListFixture(t, listObjects)
	clients := f.clients()
	started, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	clients.HelmHistory = func(context.Context, string, string) ([]helm.Revision, error) {
		once.Do(func() { close(started) })
		<-release
		return nil, errors.New("connection lost")
	}
	defer close(release)
	s := NewInformerStore(clients, InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	require.NoError(t, s.Start(context.Background()))
	select {
	case <-started:
	case <-time.After(waitTimeout):
		t.Fatal("the Helm history was not read")
	}

	closed := make(chan struct{})
	go func() {
		s.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(time.Second):
		t.Fatal("Close waited for the Helm read")
	}
}

// operatorListFails makes the operator list fail while failing is set.
func operatorListFails(f *listFixture, failing *atomic.Bool) {
	f.dynamic.PrependReactor("list", "deployments", func(a k8stesting.Action) (bool, k8sruntime.Object, error) {
		la, ok := a.(k8stesting.ListAction)
		if ok && failing.Load() && strings.Contains(la.GetListRestrictions().Labels.String(), "datadog-operator") {
			return true, nil, errors.New("connection lost")
		}
		return false, nil, nil
	})
}

// When the operator poll fails after a success, the Lease is Disconnected
// too, keeps its objects and its countdown moves on.
func TestInformerStoreLeaseFollowsOperatorFailure(t *testing.T) {
	f := newListFixture(t, listObjects)
	var failing atomic.Bool
	operatorListFails(f, &failing)
	clock := newTestClock()
	interval := 30 * time.Millisecond
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog", Now: clock.now}, PollInterval: interval})
	waitSnapshot(t, s, "Lease", func(snap *Snapshot) bool {
		return snap.Sources[SourceLease].State == SourceOK && len(snap.Objects[SourceLease]) == 1
	})

	later := testNow.Add(time.Hour)
	clock.set(later)
	failing.Store(true)
	snap := waitSnapshot(t, s, "Lease disconnected", func(snap *Snapshot) bool {
		return snap.Sources[SourceLease].State == SourceDisconnected && snap.Sources[SourceOperator].State == SourceDisconnected
	})
	lease := snap.Sources[SourceLease]
	assert.Contains(t, lease.Err, "connection lost")
	assert.Equal(t, testNow, lease.LastOK)
	assert.Equal(t, later.Add(interval), lease.NextPoll, "the countdown is not stuck in the past")
	assert.Len(t, snap.Objects[SourceLease], 1, "stale Lease kept")

	failing.Store(false)
	waitSnapshot(t, s, "Lease recovered", func(snap *Snapshot) bool {
		return snap.Sources[SourceLease].State == SourceOK && snap.Sources[SourceLease].LastOK.Equal(later)
	})
}

// A watch that is forbidden while its probe fails for another reason
// retries the probe instead of polling for good.
func TestInformerStoreForbiddenWatchProbeRetried(t *testing.T) {
	f := newListFixture(t, listObjects)
	var watchForbidden, probeFails atomic.Bool
	watchForbidden.Store(true)
	probeFails.Store(true)
	f.dynamic.PrependWatchReactor("daemonsets", func(k8stesting.Action) (bool, watch.Interface, error) {
		if watchForbidden.Load() {
			return true, nil, apierrors.NewForbidden(gvrDaemonSets.GroupResource(), "", errors.New("watch denied"))
		}
		return false, nil, nil
	})
	f.dynamic.PrependReactor("list", "daemonsets", func(a k8stesting.Action) (bool, k8sruntime.Object, error) {
		if probeFails.Load() && isProbe(a) {
			return true, nil, errors.New("timeout")
		}
		return false, nil, nil
	})
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}, FallbackInterval: 20 * time.Millisecond})
	snap := waitSnapshot(t, s, "probe error", func(snap *Snapshot) bool {
		return strings.Contains(snap.Sources[SourceDaemonSets].Err, "timeout")
	})
	assert.Contains(t, []SourceState{SourceError, SourceDisconnected}, snap.Sources[SourceDaemonSets].State)
	assert.Equal(t, RefreshWatch, snap.Sources[SourceDaemonSets].Mode)

	probeFails.Store(false)
	waitSnapshot(t, s, "daemon sets polled", func(snap *Snapshot) bool {
		info := snap.Sources[SourceDaemonSets]
		return info.Mode == RefreshPoll && info.State == SourceOK
	})
}

// isProbe reports whether a list action is the one-object probe.
func isProbe(a k8stesting.Action) bool {
	la, ok := a.(k8stesting.ListActionImpl)
	return ok && la.ListOptions.Limit == 1
}

// An Unauthorized watch is an error the informer retries, not Forbidden.
func TestInformerStoreUnauthorizedRetried(t *testing.T) {
	f := newListFixture(t, listObjects)
	var failing atomic.Bool
	failing.Store(true)
	f.dynamic.PrependWatchReactor("daemonsets", func(k8stesting.Action) (bool, watch.Interface, error) {
		if failing.Load() {
			return true, nil, apierrors.NewUnauthorized("token expired")
		}
		return false, nil, nil
	})
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	snap := waitSnapshot(t, s, "daemon sets disconnected", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].State == SourceDisconnected
	})
	assert.Len(t, snap.Objects[SourceDaemonSets], 2)
	failing.Store(false)
	snap = waitSnapshot(t, s, "daemon sets recovered", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].State == SourceOK
	})
	assert.Equal(t, RefreshWatch, snap.Sources[SourceDaemonSets].Mode)
}

// A watched source whose collection is not found recovers once its
// requests succeed again.
func TestInformerStoreNotInstalledRecovers(t *testing.T) {
	f := newListFixture(t, listObjects)
	var missing atomic.Bool
	missing.Store(true)
	notFound := func(k8stesting.Action) (bool, k8sruntime.Object, error) {
		if missing.Load() {
			return true, nil, apierrors.NewNotFound(gvrDaemonSets.GroupResource(), "")
		}
		return false, nil, nil
	}
	f.dynamic.PrependReactor("list", "daemonsets", notFound)
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	waitSnapshot(t, s, "daemon sets not installed", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].State == SourceNotInstalled
	})
	missing.Store(false)
	snap := waitSnapshot(t, s, "daemon sets recovered", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].State == SourceOK
	})
	assert.Len(t, snap.Objects[SourceDaemonSets], 2)
}

// Close stops every goroutine of the store, across selection changes, the
// forbidden-watch fallback, pollers and the Helm loop.
func TestInformerStoreCloseStopsGoroutines(t *testing.T) {
	f := newListFixture(t, listObjects)
	f.dynamic.PrependWatchReactor("daemonsets", func(k8stesting.Action) (bool, watch.Interface, error) {
		return true, nil, apierrors.NewForbidden(gvrDaemonSets.GroupResource(), "", errors.New("watch denied"))
	})
	s := NewInformerStore(f.clients(), InformerConfig{
		ListConfig:   ListConfig{AllNamespaces: true, Pods: true, Now: func() time.Time { return testNow }},
		PollInterval: 10 * time.Millisecond, FallbackInterval: 10 * time.Millisecond, HelmInterval: 10 * time.Millisecond,
	})
	require.NoError(t, s.Start(context.Background()))
	waitSnapshot(t, s, "daemon sets polled", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].Mode == RefreshPoll && snap.Sources[SourceLease].State == SourceOK
	})

	// Move the selection to another DDA.
	ctx := context.Background()
	second := &unstructured.Unstructured{}
	second.SetAPIVersion("datadoghq.com/v2alpha1")
	second.SetKind("DatadogAgent")
	second.SetNamespace("other")
	second.SetName("second")
	_, err := f.dynamic.Resource(gvrDDA).Namespace("other").Create(ctx, second, metav1.CreateOptions{})
	require.NoError(t, err)
	require.NoError(t, f.dynamic.Resource(gvrDDA).Namespace("datadog").Delete(ctx, "datadog-agent", metav1.DeleteOptions{}))
	waitSnapshot(t, s, "sources of the second DDA", func(snap *Snapshot) bool {
		return len(snap.Objects[SourceDDA]) == 1 && snap.Sources[SourceDaemonSets].Mode == RefreshPoll && snap.Sources[SourceDaemonSets].State == SourceOK && len(snap.Objects[SourceDaemonSets]) == 0
	})

	s.Close()
	var stacks string
	leaked := func() bool {
		buf := make([]byte, 1<<20)
		stacks = string(buf[:runtime.Stack(buf, true)])
		return strings.Contains(stacks, "dashboard.(*InformerStore)") || strings.Contains(stacks, "dashboard.(*sourceRun)") || strings.Contains(stacks, "client-go/tools/cache")
	}
	if !assert.Eventually(t, func() bool { return !leaked() }, waitTimeout, 10*time.Millisecond) {
		t.Fatalf("goroutines left after Close:\n%s", stacks)
	}
}

func TestInformerStoreUnsupportedOperator(t *testing.T) {
	f := newListFixture(t, listObjects, "datadogagentinternals")
	s := NewInformerStore(f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	require.ErrorIs(t, s.Start(context.Background()), ErrUnsupportedOperator)
	s.Close()
	_, err := Build(s.Snapshot(), BuildConfig{}, nil, testNow)
	require.ErrorIs(t, err, ErrUnsupportedOperator)
	assert.Empty(t, f.dynamic.Actions())
}

func TestInformerStoreNoDAPCRD(t *testing.T) {
	f := newListFixture(t, listObjects, "datadogagentprofiles")
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	snap := waitSnapshot(t, s, "sources", settled([]string{SourceDDAI, SourceDaemonSets}, 1))
	assert.Equal(t, SourceNotInstalled, snap.Sources[SourceDAP].State)
	v, err := Build(snap, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Empty(t, v.DDA.Profiles)
}

// The store only reads.
func TestInformerStoreReadOnly(t *testing.T) {
	f := newListFixture(t, listObjects)
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog", Pods: true}})
	waitSnapshot(t, s, "all sources", settled([]string{SourceDDA, SourceDAP, SourcePods, SourceLease, "sum/DatadogMonitor"}, 1))
	s.Close()
	actions := slices.Concat(f.dynamic.Actions(), f.kube.Actions())
	require.NotEmpty(t, actions)
	for _, a := range actions {
		assert.Contains(t, []string{"get", "list", "watch"}, a.GetVerb(), a.GetResource().Resource)
	}
}

func TestInformerStoreSnapshotCached(t *testing.T) {
	f := newListFixture(t, listObjects)
	s := startInformer(t, f.clients(), InformerConfig{ListConfig: ListConfig{Namespace: "datadog"}})
	waitSnapshot(t, s, "all sources", settled([]string{SourceDDA, SourceDAP, SourceOperator, SourceLease}, 1))
	// Pollers may still record their first reads; wait for a quiet period.
	require.Eventually(t, func() bool { return s.Snapshot() == s.Snapshot() }, waitTimeout, 10*time.Millisecond)
	s.Close()
	assert.Same(t, s.Snapshot(), s.Snapshot())
	s.Close() // idempotent
}

func TestInformerStoreCloseBeforeStart(t *testing.T) {
	s := NewInformerStore(ListClients{}, InformerConfig{})
	s.Close()
	assert.Empty(t, s.Snapshot().Sources)
}

func TestStripPodOnInformerTypes(t *testing.T) {
	// Tombstones and other values pass through the transforms unchanged.
	tomb := cache.DeletedFinalStateUnknown{Key: "datadog/x", Obj: &corev1.Pod{}}
	out, err := StripPod(tomb)
	require.NoError(t, err)
	assert.Equal(t, tomb, out)
	out, err = StripObject(tomb)
	require.NoError(t, err)
	assert.Equal(t, tomb, out)
	_, ok := out.(*corev1.Pod)
	assert.False(t, ok)
}

// InformerConfig.Go starts every goroutine of the store, and they end with
// Close.
func TestInformerStoreGoHook(t *testing.T) {
	f := newListFixture(t, listObjects)
	var started, running atomic.Int32
	s := NewInformerStore(f.clients(), InformerConfig{
		ListConfig: ListConfig{AllNamespaces: true, Now: func() time.Time { return testNow }},
		Go: func(fn func()) {
			started.Add(1)
			running.Add(1)
			go func() {
				defer running.Add(-1)
				fn()
			}()
		},
	})
	require.NoError(t, s.Start(context.Background()))
	waitSnapshot(t, s, "sources read", func(snap *Snapshot) bool {
		return snap.Sources[SourceDaemonSets].State == SourceOK
	})
	assert.Greater(t, started.Load(), int32(5))
	s.Close()
	require.Eventually(t, func() bool { return running.Load() == 0 }, time.Second, time.Millisecond, "every goroutine started by Go ends with Close")
}
