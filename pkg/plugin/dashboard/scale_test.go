// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"encoding/json"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
)

// Scale budget.
const (
	scalePods     = 5000
	scaleProfiles = 50
	scaleHeap     = 100 << 20
	scaleBuild    = 50 * time.Millisecond
)

// scaleSnapshot returns the profiles scenario with scaleProfiles copies of
// the canary profile, its DDAI and DaemonSet, and scalePods stripped agent
// pods spread over the DaemonSets.
func scaleSnapshot(tb testing.TB) *Snapshot {
	tb.Helper()
	s, err := LoadFixture("testdata/profiles")
	require.NoError(tb, err)
	clone := func(key, name string, i int) {
		for _, o := range s.Objects[key] {
			if o.GetName() != name {
				continue
			}
			b, err := json.Marshal(o.Object)
			require.NoError(tb, err)
			c := &unstructured.Unstructured{}
			require.NoError(tb, c.UnmarshalJSON([]byte(strings.ReplaceAll(string(b), "canary", fmt.Sprintf("scale%02d", i)))))
			s.Objects[key] = append(s.Objects[key], c)
		}
	}
	for i := range scaleProfiles {
		clone(SourceDAP, "canary", i)
		clone(SourceDDAI, "canary", i)
		clone(SourceDaemonSets, "canary-agent", i)
		clone(SourceControllerRevisions, "canary-agent-c4n4ry", i)
	}
	var daemonSets []*unstructured.Unstructured
	for _, ds := range s.Objects[SourceDaemonSets] {
		if strings.HasPrefix(ds.GetName(), "scale") || ds.GetName() == "datadog-agent-agent" {
			daemonSets = append(daemonSets, ds)
		}
	}
	s.Pods = make([]*corev1.Pod, 0, scalePods)
	for i := range scalePods {
		ds := daemonSets[i%len(daemonSets)]
		stripped, err := StripPod(fullPod(ds, i))
		require.NoError(tb, err)
		s.Pods = append(s.Pods, stripped.(*corev1.Pod))
	}
	s.Sources[SourcePods] = SourceInfo{State: SourceOK, Mode: RefreshWatch}
	return s
}

// fullPod is an agent pod as the API server returns it.
func fullPod(ds *unstructured.Unstructured, i int) *corev1.Pod {
	created := metav1.NewTime(testNow.Add(-time.Hour))
	env := make([]corev1.EnvVar, 40)
	for j := range env {
		env[j] = corev1.EnvVar{Name: fmt.Sprintf("DD_SETTING_%d", j), Value: strings.Repeat("v", 64)}
	}
	containers := make([]corev1.Container, 4)
	statuses := make([]corev1.ContainerStatus, 4)
	for j := range containers {
		name := fmt.Sprintf("container-%d", j)
		containers[j] = corev1.Container{Name: name, Image: "gcr.io/datadoghq/agent:7.84.1", Env: env, Args: []string{"run", "--config", "/etc/datadog-agent"}}
		statuses[j] = corev1.ContainerStatus{
			Name: name, Ready: true, Image: containers[j].Image, ImageID: strings.Repeat("sha256:", 10),
			State: corev1.ContainerState{Running: &corev1.ContainerStateRunning{StartedAt: created}},
		}
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name: fmt.Sprintf("%s-%05d", ds.GetName(), i), Namespace: ds.GetNamespace(), UID: types.UID(fmt.Sprintf("pod-uid-%05d", i)),
			CreationTimestamp: created,
			Labels:            map[string]string{"agent.datadoghq.com/name": "datadog-agent", "controller-revision-hash": "c4n4ry"},
			Annotations:       map[string]string{lastAppliedAnnotation: strings.Repeat("x", 2048)},
			OwnerReferences:   []metav1.OwnerReference{{APIVersion: "apps/v1", Kind: "DaemonSet", Name: ds.GetName(), UID: ds.GetUID(), Controller: new(true)}},
			ManagedFields:     []metav1.ManagedFieldsEntry{{Manager: "kubelet", FieldsV1: &metav1.FieldsV1{Raw: []byte(strings.Repeat("f", 4096))}}},
		},
		Spec: corev1.PodSpec{NodeName: fmt.Sprintf("node-%05d", i), Containers: containers},
		Status: corev1.PodStatus{
			Phase:             corev1.PodRunning,
			Conditions:        []corev1.PodCondition{{Type: corev1.PodReady, Status: corev1.ConditionTrue, LastTransitionTime: created}},
			ContainerStatuses: statuses,
		},
	}
}

func heapInUse() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// TestScaleBudget checks the memory target and the Build time with
// 5k stripped pods and 50 profiles.
func TestScaleBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("scale test")
	}
	before := heapInUse()
	s := scaleSnapshot(t)
	cfg := BuildConfig{Pods: true, StallAfter: 10 * time.Minute}
	obs := NewMemoryObserver()
	v, err := Build(s, cfg, obs, testNow)
	require.NoError(t, err)
	require.Len(t, v.DDA.Profiles, 3+scaleProfiles)

	best := time.Duration(1 << 62)
	for range 5 {
		start := time.Now()
		_, err := Build(s, cfg, obs, testNow)
		require.NoError(t, err)
		best = min(best, time.Since(start))
	}
	heap := heapInUse() - before
	runtime.KeepAlive(s)
	runtime.KeepAlive(v)
	t.Logf("heap %.1f MB, Build %v", float64(heap)/(1<<20), best)
	assert.Less(t, heap, uint64(scaleHeap), "heap budget")
	if !raceEnabled {
		assert.Less(t, best, scaleBuild, "Build time")
	}
}

func BenchmarkBuildScale(b *testing.B) {
	s := scaleSnapshot(b)
	cfg := BuildConfig{Pods: true, StallAfter: 10 * time.Minute}
	obs := NewMemoryObserver()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := Build(s, cfg, obs, testNow); err != nil {
			b.Fatal(err)
		}
	}
}
