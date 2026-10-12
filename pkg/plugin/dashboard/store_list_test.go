// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"context"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	utilyaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/apimachinery/pkg/version"
	fakediscovery "k8s.io/client-go/discovery/fake"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	kubefake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	"github.com/DataDog/datadog-operator/pkg/plugin/helm"
)

// listObjects are the cluster objects of the ListStore tests: a DDA in
// "datadog" with a default DDAI, a DAP in another namespace with its DDAI,
// labeled and unlabeled workloads, the operator in "dd-operator" and its
// Lease, and a monitor in an app namespace.
const listObjects = `
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: datadog-agent
  namespace: datadog
  uid: dda-uid
  creationTimestamp: "2026-10-01T09:00:00Z"
  annotations:
    meta.helm.sh/release-name: datadog
    meta.helm.sh/release-namespace: datadog
  labels:
    helm.sh/chart: datadog-3.1.0
  managedFields:
    - manager: kubectl
---
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentInternal
metadata:
  name: datadog-agent
  namespace: datadog
  ownerReferences:
    - {apiVersion: datadoghq.com/v2alpha1, kind: DatadogAgent, name: datadog-agent, uid: dda-uid, controller: true}
status:
  agent: {daemonsetName: datadog-agent-agent}
---
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentInternal
metadata:
  name: canary
  namespace: datadog
  labels:
    agent.datadoghq.com/datadogagentprofile: canary
  ownerReferences:
    - {apiVersion: datadoghq.com/v2alpha1, kind: DatadogAgent, name: datadog-agent, uid: dda-uid, controller: true}
status:
  agent: {daemonsetName: canary-agent}
---
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentProfile
metadata:
  name: canary
  namespace: team-a
status:
  valid: "True"
  applied: "True"
---
apiVersion: datadoghq.com/v1alpha1
kind: DatadogAgentProfile
metadata:
  name: local
  namespace: datadog
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: datadog-agent-agent
  namespace: datadog
  uid: ds-uid
  generation: 1
  labels: {agent.datadoghq.com/name: datadog-agent, agent.datadoghq.com/component: agent}
status: {observedGeneration: 1, desiredNumberScheduled: 2, updatedNumberScheduled: 2, numberAvailable: 2, numberReady: 2}
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: canary-agent
  namespace: datadog
  generation: 1
  labels: {agent.datadoghq.com/name: datadog-agent, agent.datadoghq.com/component: agent}
status: {observedGeneration: 1, desiredNumberScheduled: 1, updatedNumberScheduled: 1, numberAvailable: 1, numberReady: 1}
---
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: unrelated
  namespace: datadog
---
apiVersion: apps/v1
kind: ControllerRevision
metadata:
  name: datadog-agent-agent-abc
  namespace: datadog
  creationTimestamp: "2026-10-01T09:00:00Z"
  labels: {agent.datadoghq.com/name: datadog-agent, controller-revision-hash: abc}
  ownerReferences:
    - {apiVersion: apps/v1, kind: DaemonSet, name: datadog-agent-agent, uid: ds-uid, controller: true}
revision: 1
data: {spec: {template: {}}}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datadog-agent-cluster-agent
  namespace: datadog
  labels: {agent.datadoghq.com/name: datadog-agent, agent.datadoghq.com/component: cluster-agent}
spec: {replicas: 1}
status: {replicas: 1, updatedReplicas: 1, availableReplicas: 1, readyReplicas: 1}
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datadog-operator
  namespace: dd-operator
  labels: {app.kubernetes.io/name: datadog-operator}
spec:
  replicas: 1
  template:
    spec:
      containers:
        - {name: manager, image: "gcr.io/datadoghq/operator:1.22.0"}
status: {readyReplicas: 1}
---
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  name: datadog-operator-lock
  namespace: dd-operator
spec:
  holderIdentity: datadog-operator-0
---
apiVersion: datadoghq.com/v1alpha1
kind: DatadogMonitor
metadata:
  name: cpu
  namespace: app
status:
  monitorState: OK
  monitorStateSyncStatus: OK
`

const listPod = `
apiVersion: v1
kind: Pod
metadata:
  name: datadog-agent-agent-x
  namespace: datadog
  labels: {agent.datadoghq.com/name: datadog-agent}
  managedFields:
    - manager: kubelet
spec:
  nodeName: node-1
  containers:
    - {name: agent, image: agent:7, env: [{name: DD_API_KEY, value: secret}]}
`

var (
	gvrDDA     = schema.GroupVersionResource{Group: "datadoghq.com", Version: "v2alpha1", Resource: "datadogagents"}
	gvrDAP     = schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogagentprofiles"}
	gvrMonitor = schema.GroupVersionResource{Group: "datadoghq.com", Version: "v1alpha1", Resource: "datadogmonitors"}
)

func decodeObjects(t *testing.T, docs string) []*unstructured.Unstructured {
	t.Helper()
	var out []*unstructured.Unstructured
	dec := utilyaml.NewYAMLOrJSONDecoder(strings.NewReader(docs), 4096)
	for {
		obj := &unstructured.Unstructured{}
		if err := dec.Decode(&obj.Object); err != nil {
			require.ErrorContains(t, err, "EOF")
			return out
		}
		if len(obj.Object) > 0 {
			out = append(out, obj)
		}
	}
}

// listFixture holds the fake clients of a ListStore test.
type listFixture struct {
	dynamic *dynamicfake.FakeDynamicClient
	kube    *kubefake.Clientset
	// helmCalls records the releases whose history was read.
	mu        sync.Mutex
	helmCalls []string
}

// newListFixture returns fake clients serving objects. Every resource of
// the source table is discovered, except those in notServed.
func newListFixture(t *testing.T, objects string, notServed ...string) *listFixture {
	t.Helper()
	listKinds := map[schema.GroupVersionResource]string{}
	resources := map[string][]metav1.APIResource{}
	for _, src := range Sources() {
		listKinds[src.GVR] = src.Kind + "List"
		if !slices.Contains(notServed, src.GVR.Resource) {
			gv := src.GVR.GroupVersion().String()
			resources[gv] = append(resources[gv], metav1.APIResource{Name: src.GVR.Resource, Kind: src.Kind, Namespaced: true})
		}
	}
	var objs []runtime.Object
	for _, o := range decodeObjects(t, objects) {
		objs = append(objs, o)
	}
	dyn := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), listKinds, objs...)

	var pods []runtime.Object
	for _, o := range decodeObjects(t, listPod) {
		pod := &corev1.Pod{}
		require.NoError(t, runtime.DefaultUnstructuredConverter.FromUnstructured(o.Object, pod))
		pods = append(pods, pod)
	}
	kube := kubefake.NewClientset(pods...)
	disco := kube.Discovery().(*fakediscovery.FakeDiscovery)
	disco.FakedServerVersion = &version.Info{GitVersion: "v1.35.1"}
	for gv, rs := range resources {
		disco.Resources = append(disco.Resources, &metav1.APIResourceList{GroupVersion: gv, APIResources: rs})
	}
	return &listFixture{dynamic: dyn, kube: kube}
}

func (f *listFixture) clients() ListClients {
	return ListClients{
		Dynamic: f.dynamic,
		Kube:    f.kube,
		HelmHistory: func(_ context.Context, namespace, release string) ([]helm.Revision, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.helmCalls = append(f.helmCalls, namespace+"/"+release)
			return []helm.Revision{
				{Chart: "datadog", ChartVersion: "3.2.0", Revision: 3},
				{Chart: "datadog", ChartVersion: "3.1.0", Revision: 2},
				{Chart: "datadog", ChartVersion: "3.0.0", Revision: 1},
			}, nil
		},
	}
}

// forbid makes the fake dynamic client deny the list of resource, in all
// namespaces or in the given one.
func (f *listFixture) forbid(gvr schema.GroupVersionResource, clusterWideOnly bool) {
	f.dynamic.PrependReactor("list", gvr.Resource, func(a k8stesting.Action) (bool, runtime.Object, error) {
		if clusterWideOnly && a.GetNamespace() != "" {
			return false, nil, nil
		}
		return true, nil, apierrors.NewForbidden(gvr.GroupResource(), "", errors.New("denied"))
	})
}

func startList(t *testing.T, f *listFixture, cfg ListConfig) *Snapshot {
	t.Helper()
	cfg.Now = func() time.Time { return testNow }
	s := NewListStore(f.clients(), cfg)
	require.NoError(t, s.Start(context.Background()))
	t.Cleanup(s.Close)
	assert.Nil(t, s.Changed())
	return s.Snapshot()
}

func names(objs []*unstructured.Unstructured) []string {
	out := make([]string, 0, len(objs))
	for _, o := range objs {
		out = append(out, o.GetNamespace()+"/"+o.GetName())
	}
	slices.Sort(out)
	return out
}

func TestListStoreReadsSources(t *testing.T) {
	f := newListFixture(t, listObjects)
	snap := startList(t, f, ListConfig{Namespace: "datadog", Cluster: ClusterInfo{Context: "kind-ddtui", PluginVersion: "v1"}})

	assert.Equal(t, testNow, snap.Taken)
	assert.Equal(t, ClusterInfo{Context: "kind-ddtui", ServerVersion: "v1.35.1", PluginVersion: "v1"}, snap.Cluster)
	for _, src := range Sources() {
		if src.Key == SourcePods {
			continue
		}
		assert.Equal(t, SourceOK, snap.Sources[src.Key].State, src.Key)
		assert.Equal(t, RefreshOnce, snap.Sources[src.Key].Mode, src.Key)
	}
	// DAPs and summary kinds are listed cluster-wide; workloads by the
	// DDA label; the operator in another namespace.
	assert.Equal(t, []string{"datadog/local", "team-a/canary"}, names(snap.Objects[SourceDAP]))
	assert.Equal(t, []string{"datadog/canary-agent", "datadog/datadog-agent-agent"}, names(snap.Objects[SourceDaemonSets]))
	assert.Equal(t, []string{"datadog/datadog-agent-cluster-agent"}, names(snap.Objects[SourceDeployments]))
	assert.Equal(t, []string{"dd-operator/datadog-operator"}, names(snap.Objects[SourceOperator]))
	assert.Equal(t, []string{"dd-operator/datadog-operator-lock"}, names(snap.Objects[SourceLease]))
	assert.Equal(t, []string{"app/cpu"}, names(snap.Objects["sum/DatadogMonitor"]))

	// Strip transforms ran.
	assert.Nil(t, snap.Objects[SourceDDA][0].GetManagedFields())
	_, hasData := snap.Objects[SourceControllerRevisions][0].Object["data"]
	assert.False(t, hasData)

	// Pods are only listed with --pods.
	_, hasPods := snap.Sources[SourcePods]
	assert.False(t, hasPods)
	assert.Nil(t, snap.Pods)

	// The Helm release of the DDA is read, keeping the two newest.
	assert.Equal(t, []string{"datadog/datadog"}, f.helmCalls)
	require.Len(t, snap.Helm["datadog/datadog"], 2)
	assert.Equal(t, "3.2.0", snap.Helm["datadog/datadog"][0].ChartVersion)

	v, err := Build(snap, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	require.NotNil(t, v.DDA.Default)
	require.NotNil(t, v.DDA.Default.Agent)
	assert.Equal(t, "datadog-agent-agent", v.DDA.Default.Agent.Name)
	require.NotNil(t, v.Header.Operator)
	assert.Equal(t, "1.22.0", v.Header.Operator.Version)
	assert.Equal(t, "datadog-operator-0", v.Header.Operator.LeaseHolder)
	require.Len(t, v.DDA.Profiles, 2)
	assert.Empty(t, v.Notices)
}

func TestListStorePods(t *testing.T) {
	f := newListFixture(t, listObjects)
	snap := startList(t, f, ListConfig{Namespace: "datadog", Pods: true})

	assert.Equal(t, SourceOK, snap.Sources[SourcePods].State)
	require.Len(t, snap.Pods, 1)
	pod := snap.Pods[0]
	assert.Equal(t, "node-1", pod.Spec.NodeName)
	assert.Empty(t, pod.Spec.Containers, "pods are stripped")
	assert.Nil(t, pod.ManagedFields)
}

func TestListStoreNoHelm(t *testing.T) {
	f := newListFixture(t, listObjects)
	clients := f.clients()
	clients.HelmHistory = nil
	s := NewListStore(clients, ListConfig{Namespace: "datadog"})
	require.NoError(t, s.Start(context.Background()))
	assert.Empty(t, s.Snapshot().Helm)
	assert.Empty(t, f.helmCalls)
}

// A forbidden source degrades only itself.
func TestListStoreForbiddenSource(t *testing.T) {
	f := newListFixture(t, listObjects)
	f.forbid(gvrMonitor, false)
	snap := startList(t, f, ListConfig{Namespace: "datadog"})

	info := snap.Sources["sum/DatadogMonitor"]
	assert.Equal(t, SourceForbidden, info.State)
	assert.Contains(t, info.Err, "denied")
	for _, src := range Sources() {
		if src.Key != "sum/DatadogMonitor" && src.Key != SourcePods {
			assert.Equal(t, SourceOK, snap.Sources[src.Key].State, src.Key)
		}
	}

	v, err := Build(snap, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	require.NotNil(t, v.DDA.Default)
	assert.Equal(t, SourceForbidden, v.Summary[0].State)
	assert.Equal(t, "Monitors", v.Summary[0].Label)
}

// A cluster-wide DAP list that is forbidden falls back to the DDA
// namespace with a notice.
func TestListStoreClusterWideFallback(t *testing.T) {
	f := newListFixture(t, listObjects)
	f.forbid(gvrDAP, true)
	f.forbid(gvrMonitor, true)
	snap := startList(t, f, ListConfig{Namespace: "datadog"})

	info := snap.Sources[SourceDAP]
	assert.Equal(t, SourceOK, info.State)
	assert.Equal(t, "DatadogAgentProfiles: namespace datadog only (cluster-wide list forbidden)", info.Notice)
	assert.Equal(t, []string{"datadog/local"}, names(snap.Objects[SourceDAP]))
	assert.Equal(t, SourceOK, snap.Sources["sum/DatadogMonitor"].State)
	assert.Empty(t, snap.Objects["sum/DatadogMonitor"])

	v, err := Build(snap, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"DatadogAgentProfiles: namespace datadog only (cluster-wide list forbidden)",
		"Other resources: namespace datadog only (cluster-wide list forbidden)",
	}, v.Notices)
}

func TestListStoreFallbackForbiddenToo(t *testing.T) {
	f := newListFixture(t, listObjects)
	f.forbid(gvrDAP, false)
	snap := startList(t, f, ListConfig{Namespace: "datadog"})
	assert.Equal(t, SourceForbidden, snap.Sources[SourceDAP].State)
}

func TestListStoreOperatorInDDANamespace(t *testing.T) {
	objs := listObjects + `
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: datadog-operator
  namespace: datadog
  labels: {app.kubernetes.io/name: datadog-operator}
`
	f := newListFixture(t, objs)
	snap := startList(t, f, ListConfig{Namespace: "datadog"})
	assert.Equal(t, []string{"datadog/datadog-operator"}, names(snap.Objects[SourceOperator]))
	// The Lease is looked up in the operator namespace, where it is absent.
	assert.Equal(t, SourceOK, snap.Sources[SourceLease].State)
	assert.Empty(t, snap.Objects[SourceLease])
}

func TestListStoreOperatorNotListable(t *testing.T) {
	f := newListFixture(t, listObjects)
	f.dynamic.PrependReactor("list", "deployments", func(a k8stesting.Action) (bool, runtime.Object, error) {
		if a.GetNamespace() == "" {
			return true, nil, apierrors.NewForbidden(schema.GroupResource{Group: "apps", Resource: "deployments"}, "", errors.New("denied"))
		}
		return false, nil, nil
	})
	snap := startList(t, f, ListConfig{Namespace: "datadog"})
	info := snap.Sources[SourceOperator]
	assert.Equal(t, SourceOK, info.State)
	assert.Contains(t, info.Notice, "not found in namespace datadog")
	assert.Equal(t, SourceOK, snap.Sources[SourceDeployments].State)
}

func TestListStoreUnsupportedOperator(t *testing.T) {
	for _, missing := range []string{"datadogagentinternals", "datadogagents"} {
		t.Run(missing, func(t *testing.T) {
			f := newListFixture(t, listObjects, missing)
			s := NewListStore(f.clients(), ListConfig{Namespace: "datadog"})
			require.ErrorIs(t, s.Start(context.Background()), ErrUnsupportedOperator)
			_, err := Build(s.Snapshot(), BuildConfig{}, nil, testNow)
			require.ErrorIs(t, err, ErrUnsupportedOperator)
			// Nothing is listed before the CRDs are checked.
			for _, a := range f.dynamic.Actions() {
				t.Errorf("unexpected %s %s", a.GetVerb(), a.GetResource().Resource)
			}
		})
	}
}

// The DAP CRD is optional: without it the DDA tree is built with no DAPs.
func TestListStoreNoDAPCRD(t *testing.T) {
	f := newListFixture(t, listObjects, "datadogagentprofiles")
	snap := startList(t, f, ListConfig{Namespace: "datadog"})
	assert.Equal(t, SourceNotInstalled, snap.Sources[SourceDAP].State)
	assert.Empty(t, snap.Objects[SourceDAP])
	assert.Equal(t, SourceOK, snap.Sources[SourceDDAI].State)
	for _, a := range f.dynamic.Actions() {
		assert.NotEqual(t, "datadogagentprofiles", a.GetResource().Resource, "the DAP CRD is never listed")
	}
	v, err := Build(snap, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	require.NotNil(t, v.DDA.Default)
	assert.Equal(t, "datadog-agent", v.DDA.Default.Name)
	assert.Empty(t, v.DDA.Profiles)
	// The profile DDAI has no DAP to attach to.
	require.NotNil(t, v.Unattached)
	assert.Equal(t, "canary", v.Unattached.DDAIs[0].Name)
}

// Summary kinds that are not installed are recorded so and never listed.
func TestListStoreSummaryNotInstalled(t *testing.T) {
	f := newListFixture(t, listObjects, "datadogcsidrivers")
	snap := startList(t, f, ListConfig{Namespace: "datadog"})
	assert.Equal(t, SourceNotInstalled, snap.Sources["sum/DatadogCSIDriver"].State)
	for _, a := range f.dynamic.Actions() {
		assert.NotEqual(t, "datadogcsidrivers", a.GetResource().Resource)
	}
}

func TestListStoreDDASelection(t *testing.T) {
	second := listObjects + `
---
apiVersion: datadoghq.com/v2alpha1
kind: DatadogAgent
metadata:
  name: second
  namespace: other
`
	t.Run("multiple DDAs across namespaces", func(t *testing.T) {
		f := newListFixture(t, second)
		snap := startList(t, f, ListConfig{AllNamespaces: true})
		assert.Len(t, snap.Objects[SourceDDA], 2)
		// Nothing else is read until a DDA is selected.
		assert.Len(t, snap.Sources, 1+countNotInstalled(snap))
		v, err := Build(snap, BuildConfig{}, nil, testNow)
		require.NoError(t, err)
		assert.Equal(t, []string{"datadog/datadog-agent", "other/second"}, v.MultipleDDAs)
	})
	t.Run("named DDA", func(t *testing.T) {
		f := newListFixture(t, second)
		snap := startList(t, f, ListConfig{AllNamespaces: true, DDAName: "second"})
		assert.Equal(t, SourceOK, snap.Sources[SourceDaemonSets].State)
		for _, a := range f.dynamic.Actions() {
			if a.GetResource().Resource == "daemonsets" {
				assert.Equal(t, "other", a.GetNamespace())
			}
		}
	})
	t.Run("namespace scope", func(t *testing.T) {
		f := newListFixture(t, second)
		snap := startList(t, f, ListConfig{Namespace: "other"})
		assert.Equal(t, []string{"other/second"}, names(snap.Objects[SourceDDA]))
	})
	t.Run("no DDA", func(t *testing.T) {
		f := newListFixture(t, second)
		snap := startList(t, f, ListConfig{Namespace: "empty"})
		_, err := Build(snap, BuildConfig{}, nil, testNow)
		require.ErrorIs(t, err, ErrNoDDA)
	})
	t.Run("DDA forbidden", func(t *testing.T) {
		f := newListFixture(t, second)
		f.forbid(gvrDDA, false)
		snap := startList(t, f, ListConfig{Namespace: "datadog"})
		assert.Equal(t, SourceForbidden, snap.Sources[SourceDDA].State)
		_, err := Build(snap, BuildConfig{}, nil, testNow)
		require.ErrorContains(t, err, "Forbidden")
	})
}

func countNotInstalled(s *Snapshot) int {
	n := 0
	for _, info := range s.Sources {
		if info.State == SourceNotInstalled {
			n++
		}
	}
	return n
}

func TestListStoreDiscoveryError(t *testing.T) {
	f := newListFixture(t, listObjects)
	f.kube.PrependReactor("get", "version", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})
	s := NewListStore(f.clients(), ListConfig{Namespace: "datadog"})
	require.ErrorContains(t, s.Start(context.Background()), "connection refused")
}

func TestListStoreCanceled(t *testing.T) {
	f := newListFixture(t, listObjects)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewListStore(f.clients(), ListConfig{Namespace: "datadog"})
	require.ErrorIs(t, s.Start(ctx), context.Canceled)
}

func TestClassify(t *testing.T) {
	gr := schema.GroupResource{Resource: "x"}
	assert.Equal(t, SourceForbidden, classify(apierrors.NewForbidden(gr, "", errors.New("no"))))
	assert.Equal(t, SourceForbidden, classify(apierrors.NewUnauthorized("no")))
	assert.Equal(t, SourceNotInstalled, classify(apierrors.NewNotFound(gr, "")))
	assert.Equal(t, SourceError, classify(errors.New("boom")))
}
