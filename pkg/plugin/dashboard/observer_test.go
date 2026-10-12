// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-operator/pkg/rollout"
)

func TestMemoryObserver(t *testing.T) {
	o := NewMemoryObserver()
	t0 := testNow
	at := func(d time.Duration) time.Time { return t0.Add(d) }
	w := rollout.Workload{CurrentRevision: "a", Desired: 3, Updated: 1, Available: 2}

	// First observation: FirstSeen is now, no progress seen yet.
	assert.Equal(t, Observation{FirstSeen: t0}, o.Observe("ds", w, t0))
	// Same counts: nothing moves.
	assert.Equal(t, Observation{FirstSeen: t0}, o.Observe("ds", w, at(time.Minute)))

	// Only updated and available count as progress.
	w.Unavailable, w.Desired, w.ObservedGeneration = 1, 4, 2
	assert.Equal(t, Observation{FirstSeen: t0}, o.Observe("ds", w, at(2*time.Minute)))
	w.Updated = 2
	assert.Equal(t, Observation{FirstSeen: t0, LastProgress: at(3 * time.Minute)}, o.Observe("ds", w, at(3*time.Minute)))
	w.Available = 3
	assert.Equal(t, Observation{FirstSeen: t0, LastProgress: at(4 * time.Minute)}, o.Observe("ds", w, at(4*time.Minute)))

	// An unresolved revision keeps FirstSeen; a new revision resets it.
	w.CurrentRevision = ""
	assert.Equal(t, t0, o.Observe("ds", w, at(5*time.Minute)).FirstSeen)
	w.CurrentRevision = "a"
	assert.Equal(t, t0, o.Observe("ds", w, at(6*time.Minute)).FirstSeen)
	w.CurrentRevision = "b"
	assert.Equal(t, Observation{FirstSeen: at(7 * time.Minute), LastProgress: at(4 * time.Minute)}, o.Observe("ds", w, at(7*time.Minute)))

	// Workloads are independent.
	assert.Equal(t, Observation{FirstSeen: at(8 * time.Minute)}, o.Observe("deploy", w, at(8*time.Minute)))

	// A workload never seen for observerTTL is forgotten.
	o.Observe("deploy", w, at(8*time.Minute+observerTTL+time.Minute))
	assert.Len(t, o.entries, 1)
}

func TestMemoryObserverConvergedSince(t *testing.T) {
	o := NewMemoryObserver()
	at := func(d time.Duration) time.Time { return testNow.Add(d) }
	w := rollout.Workload{CurrentRevision: "a", Generation: 2, ObservedGeneration: 2, Desired: 3, Updated: 3, Available: 2}

	// Converged from the first observation: unknown, the start bounds it.
	assert.Zero(t, o.Observe("ds", w, at(0)).ConvergedSince)
	assert.Zero(t, o.Observe("ds", w, at(time.Minute)).ConvergedSince)

	// Not converged clears it; converging again sets it, once.
	w.Generation = 3
	assert.Zero(t, o.Observe("ds", w, at(2*time.Minute)).ConvergedSince)
	w.ObservedGeneration, w.Updated = 3, 1
	assert.Zero(t, o.Observe("ds", w, at(3*time.Minute)).ConvergedSince)
	w.Updated = 3
	assert.Equal(t, at(4*time.Minute), o.Observe("ds", w, at(4*time.Minute)).ConvergedSince)
	w.Available = 3
	assert.Equal(t, at(4*time.Minute), o.Observe("ds", w, at(5*time.Minute)).ConvergedSince)

	// A new revision seen already converged sets it too.
	w.CurrentRevision = "b"
	assert.Equal(t, at(6*time.Minute), o.Observe("ds", w, at(6*time.Minute)).ConvergedSince)
}

// In live mode, a workload seen converging is Settling for the settling
// period after the convergence, even when its rollout started long ago.
func TestBuildSettlingFromObserver(t *testing.T) {
	obs := NewMemoryObserver()
	agent := func(v View) *WorkloadView { return v.DDA.Default.Agent }
	build := func(updated int64, now time.Time) View {
		s := loadScenario(t, "availability")
		for _, ds := range s.Objects[SourceDaemonSets] {
			if ds.GetName() == "datadog-agent-agent" {
				require.NoError(t, unstructured.SetNestedField(ds.Object, updated, "status", "updatedNumberScheduled"))
			}
		}
		v, err := Build(s, BuildConfig{}, obs, now)
		require.NoError(t, err)
		return v
	}
	assert.Equal(t, "Rolling", agent(build(1, testNow)).Rollout.Phase)
	v := build(2, testNow.Add(time.Minute))
	assert.Equal(t, "Settling", agent(v).Rollout.Phase)
	assert.Equal(t, BadgeProgressing, agent(v).Health)
	require.NotNil(t, agent(v).Rollout.SettlingUntil)
	assert.Equal(t, testNow.Add(time.Minute+rollout.DefaultSettlingPeriod), *agent(v).Rollout.SettlingUntil)
	v = build(2, testNow.Add(time.Minute+rollout.DefaultSettlingPeriod))
	assert.Equal(t, "Complete", agent(v).Rollout.Phase)
	assert.Equal(t, BadgeDegraded, agent(v).Health)
}

func TestMemoryObserverUnresolvedFirst(t *testing.T) {
	o := NewMemoryObserver()
	o.Observe("ds", rollout.Workload{}, testNow)
	got := o.Observe("ds", rollout.Workload{CurrentRevision: "a"}, testNow.Add(time.Minute))
	assert.Equal(t, testNow, got.FirstSeen, "the revision may be the one seen first")
}

// recordingEvaluator records the inputs it is given.
type recordingEvaluator struct {
	inputs map[string]rollout.Input
}

func (recordingEvaluator) Name() string { return "recording" }

func (e recordingEvaluator) Evaluate(in rollout.Input) (rollout.Result, bool) {
	e.inputs[in.Workload.Name] = in
	return rollout.HeuristicEvaluator{}.Evaluate(in)
}

// Build gets FirstSeen for every workload from the observer and
// LastProgress once counts change.
func TestBuildWithMemoryObserver(t *testing.T) {
	s := loadScenario(t, "steady")
	obs := NewMemoryObserver()
	eval := recordingEvaluator{inputs: map[string]rollout.Input{}}
	cfg := BuildConfig{Evaluator: eval}

	_, err := Build(s, cfg, obs, testNow)
	require.NoError(t, err)
	require.Len(t, eval.inputs, 3)
	for name, in := range eval.inputs {
		assert.Equal(t, testNow, in.FirstSeen, name)
		assert.True(t, in.LastProgress.IsZero(), name)
	}

	later := testNow.Add(time.Minute)
	v, err := Build(s, cfg, obs, later)
	require.NoError(t, err)
	assert.Nil(t, v.DDA.Default.Agent.Rollout.LastProgress)

	// The DaemonSet loses an updated pod: the counts changed.
	var ds *unstructured.Unstructured
	for _, o := range s.Objects[SourceDaemonSets] {
		if o.GetName() == "datadog-agent-agent" {
			ds = o
		}
	}
	require.NotNil(t, ds)
	updated, _, err := unstructured.NestedInt64(ds.Object, "status", "updatedNumberScheduled")
	require.NoError(t, err)
	require.NoError(t, unstructured.SetNestedField(ds.Object, updated-1, "status", "updatedNumberScheduled"))
	evenLater := testNow.Add(2 * time.Minute)
	v, err = Build(s, cfg, obs, evenLater)
	require.NoError(t, err)
	require.NotNil(t, v.DDA.Default.Agent.Rollout.LastProgress)
	assert.Equal(t, evenLater, *v.DDA.Default.Agent.Rollout.LastProgress)
	assert.Equal(t, testNow, eval.inputs["datadog-agent-agent"].FirstSeen)
}
