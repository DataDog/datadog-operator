// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dashboard

import (
	"context"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-operator/pkg/rollout"
)

var update = flag.Bool("update", false, "rewrite the golden files of the dashboard tests")

// testNow is the fixed clock of every fixture.
var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

var goldenScenarios = []string{"steady", "profiles", "reconcile-errors", "helm", "summary", "stalled", "no-dap-crd", "dap-no-status", "stale-daps", "stale-warnings", "availability"}

func loadScenario(t *testing.T, name string) *Snapshot {
	t.Helper()
	store := NewFixtureStore(filepath.Join("testdata", name))
	require.NoError(t, store.Start(context.Background()))
	t.Cleanup(store.Close)
	assert.Nil(t, store.Changed())
	return store.Snapshot()
}

func buildScenario(t *testing.T, name string) View {
	t.Helper()
	v, err := Build(loadScenario(t, name), FixtureBuildConfig(name), NopObserver{}, testNow)
	require.NoError(t, err)
	return v
}

// TestGolden guards the View JSON schema: any change to the output shows
// up as a golden diff. Run with -update to rewrite.
func TestGolden(t *testing.T) {
	for _, name := range goldenScenarios {
		t.Run(name, func(t *testing.T) {
			v := buildScenario(t, name)
			got, err := json.MarshalIndent(v, "", "  ")
			require.NoError(t, err)
			got = append(got, '\n')

			path := filepath.Join("testdata", name, "view.golden.json")
			if *update {
				require.NoError(t, os.WriteFile(path, got, 0o644))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "missing golden file; run go test -run TestGolden -update")
			assert.Equal(t, string(want), string(got))
		})
	}
}

// TestGoldenVariants guards the View JSON of the fixture variants. Run with
// -update to rewrite.
func TestGoldenVariants(t *testing.T) {
	for _, fv := range FixtureVariants {
		t.Run(fv.Name, func(t *testing.T) {
			v, err := Build(loadScenario(t, fv.Scenario), fv.Config, NopObserver{}, testNow)
			require.NoError(t, err)
			got, err := json.MarshalIndent(v, "", "  ")
			require.NoError(t, err)
			got = append(got, '\n')

			path := filepath.Join("testdata", fv.Scenario, fv.Name+".golden.json")
			if *update {
				require.NoError(t, os.WriteFile(path, got, 0o644))
			}
			want, err := os.ReadFile(path)
			require.NoError(t, err, "missing golden file; run go test -run TestGoldenVariants -update")
			assert.Equal(t, string(want), string(got))
		})
	}
}

func TestBuildSelection(t *testing.T) {
	t.Run("multiple DDAs", func(t *testing.T) {
		v, err := Build(loadScenario(t, "multiple-ddas"), BuildConfig{}, nil, testNow)
		require.NoError(t, err, "the warning is in the View")
		assert.Equal(t, []string{"datadog/datadog-agent", "datadog/second"}, v.MultipleDDAs)
		assert.Empty(t, v.DDA.Name)
		err = v.SelectionError()
		require.ErrorIs(t, err, ErrMultipleDDAs)
		var multi *MultipleDDAsError
		require.ErrorAs(t, err, &multi)
		assert.Equal(t, v.MultipleDDAs, multi.Names)
	})
	t.Run("named DDA among several", func(t *testing.T) {
		v, err := Build(loadScenario(t, "multiple-ddas"), BuildConfig{DDAName: "datadog-agent"}, nil, testNow)
		require.NoError(t, err)
		require.NoError(t, v.SelectionError())
		assert.Equal(t, "datadog-agent", v.DDA.Name)
		require.NotNil(t, v.DDA.Default)
		assert.Equal(t, "datadog-agent", v.DDA.Default.Name)
		// The other DDA's DDAI and DaemonSet are neither attached nor
		// unattached.
		assert.Nil(t, v.Unattached)
	})
	t.Run("unknown name", func(t *testing.T) {
		_, err := Build(loadScenario(t, "multiple-ddas"), BuildConfig{DDAName: "nope"}, nil, testNow)
		require.ErrorIs(t, err, ErrNoDDA)
		assert.Contains(t, err.Error(), "nope")
	})
	t.Run("no DDA", func(t *testing.T) {
		_, err := Build(loadScenario(t, "no-dda"), BuildConfig{}, nil, testNow)
		require.ErrorIs(t, err, ErrNoDDA)
	})
	t.Run("unsupported operator", func(t *testing.T) {
		_, err := Build(loadScenario(t, "unsupported"), BuildConfig{}, nil, testNow)
		require.ErrorIs(t, err, ErrUnsupportedOperator)
	})
	t.Run("DDA source forbidden", func(t *testing.T) {
		s := loadScenario(t, "steady")
		s.Sources[SourceDDA] = SourceInfo{State: SourceForbidden, Err: "forbidden"}
		_, err := Build(s, BuildConfig{}, nil, testNow)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "Forbidden")
	})
}

// Profiles are optional: neither a missing DAP CRD nor DAPs without status
// affect the DDA health.
func TestBuildOptionalProfiles(t *testing.T) {
	t.Run("no DAP CRD", func(t *testing.T) {
		v := buildScenario(t, "no-dap-crd")
		assert.Equal(t, BadgeHealthy, v.DDA.Health)
		assert.Empty(t, v.DDA.Profiles)
		assert.Empty(t, v.Issues)
		assert.Empty(t, v.Notices)
	})
	t.Run("DAPs without status", func(t *testing.T) {
		v := buildScenario(t, "dap-no-status")
		assert.Equal(t, BadgeHealthy, v.DDA.Health)
		require.Len(t, v.DDA.Profiles, 2)
		for _, p := range v.DDA.Profiles {
			assert.True(t, p.StatusUnknown, p.Name)
			assert.Equal(t, BadgeUnknown, p.Health, p.Name)
			assert.Nil(t, p.DDAI, p.Name)
		}
		assert.Empty(t, v.Issues)
	})
	t.Run("DAP without status with a DDAI", func(t *testing.T) {
		// Keep the default DDAI and the gpu profile only, and strip the gpu
		// DAP status: its DDAI is Healthy, the DAP itself Unknown.
		s := loadScenario(t, "profiles")
		keep := func(key string, names ...string) {
			s.Objects[key] = slices.DeleteFunc(s.Objects[key], func(u *unstructured.Unstructured) bool {
				return !slices.Contains(names, u.GetName())
			})
		}
		keep(SourceDAP, "gpu")
		keep(SourceDDAI, "datadog-agent", "gpu")
		keep(SourceDaemonSets, "datadog-agent-agent", "gpu-agent")
		keep(SourceControllerRevisions)
		require.Len(t, s.Objects[SourceDAP], 1)
		unstructured.RemoveNestedField(s.Objects[SourceDAP][0].Object, "status")

		v, err := Build(s, FixtureBuildConfig("profiles"), NopObserver{}, testNow)
		require.NoError(t, err)
		require.Len(t, v.DDA.Profiles, 1)
		gpu := v.DDA.Profiles[0]
		assert.True(t, gpu.StatusUnknown)
		assert.Equal(t, BadgeUnknown, gpu.Health)
		require.NotNil(t, gpu.DDAI)
		assert.Equal(t, BadgeHealthy, gpu.DDAI.Health)
		assert.Equal(t, BadgeHealthy, v.DDA.Health, "does not propagate")
		assert.Empty(t, v.Issues)
	})
	t.Run("DAP with status", func(t *testing.T) {
		for _, p := range buildScenario(t, "profiles").DDA.Profiles {
			assert.False(t, p.StatusUnknown, p.Name)
		}
	})
}

func TestBuildTree(t *testing.T) {
	v := buildScenario(t, "profiles")

	require.NotNil(t, v.DDA.Default)
	assert.Equal(t, "datadog-agent-agent", v.DDA.Default.Agent.Name)
	assert.Nil(t, v.DDA.Default.ClusterAgent, "disabled component is nil")

	require.Len(t, v.DDA.Profiles, 3)
	canary, dup, gpu := v.DDA.Profiles[0], v.DDA.Profiles[1], v.DDA.Profiles[2]

	// DAP -> DDAI -> DS, DS found by owner reference.
	assert.Equal(t, "canary", canary.Name)
	require.NotNil(t, canary.DDAI)
	assert.Equal(t, "canary", canary.DDAI.Profile)
	require.NotNil(t, canary.DDAI.Agent)
	assert.Equal(t, "canary-agent", canary.DDAI.Agent.Name)
	assert.Equal(t, "Rolling", canary.DDAI.Agent.Rollout.Phase)
	assert.Equal(t, BadgeProgressing, canary.Health)

	// Conflicting DAP: no DDAI, its condition shown once.
	assert.Equal(t, "dup", dup.Name)
	assert.Nil(t, dup.DDAI)
	assert.Equal(t, "False", dup.Applied)
	require.Len(t, dup.Conditions, 1)
	assert.Equal(t, "Conflict", dup.Conditions[0].Reason)
	assert.Equal(t, SeverityWarning, dup.Conditions[0].Severity)
	assert.Equal(t, BadgeDegraded, dup.Health)
	assert.Len(t, v.Issues, 1)

	// DAP in another namespace, DDAI in the DDA namespace.
	assert.Equal(t, "team-a", gpu.Namespace)
	require.NotNil(t, gpu.DDAI)
	assert.Equal(t, "OnDelete", gpu.DDAI.Agent.Strategy)

	// Unattached.
	require.NotNil(t, v.Unattached)
	require.Len(t, v.Unattached.DDAIs, 1)
	assert.Equal(t, "ghost", v.Unattached.DDAIs[0].Name)
	assert.Equal(t, "ghost-agent", v.Unattached.DDAIs[0].Agent.Name)
	require.Len(t, v.Unattached.Workloads, 1)
	assert.Equal(t, "stray-agent", v.Unattached.Workloads[0].Name)

	// Totals over the DDA's DaemonSets only.
	assert.Equal(t, Counts{Desired: 7, Ready: 6, UpToDate: 4, Available: 6, Unavailable: 1}, v.DDA.Agents)
	assert.Equal(t, BadgeDegraded, v.DDA.Health)
}

func TestBuildProvider(t *testing.T) {
	v := buildScenario(t, "steady")
	assert.Equal(t, ProviderView{Name: "eks", Source: ProviderDetected, Reason: "ProviderDetected"}, v.Header.Provider)

	v = buildScenario(t, "profiles")
	assert.Equal(t, ProviderView{Name: "gke-cos", Source: ProviderUserSet}, v.Header.Provider)
	require.NotNil(t, v.DDA.Profiles[0].Provider, "DAP provider")
	assert.Equal(t, ProviderView{Name: "gke-cos", Source: ProviderUserSet}, *v.DDA.Profiles[0].Provider)
	assert.Nil(t, v.DDA.Profiles[1].Provider)

	v = buildScenario(t, "helm")
	assert.Equal(t, ProviderView{Source: ProviderDetected, Reason: "NoProviderDetected"}, v.Header.Provider)
}

func TestBuildHeader(t *testing.T) {
	v := buildScenario(t, "steady")
	assert.Equal(t, SchemaVersion, v.SchemaVersion)
	assert.Equal(t, "kind-ddtui", v.Header.Context)
	assert.Equal(t, "v1.35.1", v.Header.ServerVersion)
	assert.Equal(t, "v1.22.0-test", v.Header.PluginVersion)
	assert.Equal(t, "datadog", v.Header.Namespace)
	require.NotNil(t, v.Header.Operator)
	assert.Equal(t, "1.22.0", v.Header.Operator.Version)
	assert.Equal(t, "datadog-operator-7c9d_1a2b", v.Header.Operator.LeaseHolder)
	assert.Nil(t, v.Header.NextPoll)

	v = buildScenario(t, "summary")
	require.NotNil(t, v.Header.NextPoll, "poll countdown")
	assert.Equal(t, time.Date(2026, 10, 5, 12, 0, 20, 0, time.UTC), *v.Header.NextPoll)
	assert.Nil(t, v.Header.Operator, "no operator found")
	assert.Equal(t, []string{"DatadogAgentProfiles: listed in the DDA namespace only"}, v.Notices)
}

func TestBuildHelm(t *testing.T) {
	v := buildScenario(t, "helm")
	assert.Equal(t, &HelmView{
		Release: "datadog", Namespace: "datadog", Chart: "datadog", Version: "3.70.0", Revision: 5,
		PreviousVersion: "3.69.0", PreviousRevision: 4,
	}, v.DDA.Helm, "old -> new while rolling")
	assert.Equal(t, &HelmView{Release: "profiles", Namespace: "datadog", Chart: "datadog-profiles", Version: "0.4.1"},
		v.DDA.Profiles[0].Helm, "label fallback without history")

	// Rolling, but the release was deployed long before the rollout started
	// (e.g. kubectl patch): current version only.
	s := loadScenario(t, "helm")
	s.Helm[ReleaseKey("datadog", "datadog")][0].LastDeployed = testNow.Add(-time.Hour)
	v, err := Build(s, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, "Rolling", v.DDA.Rollout.Phase)
	assert.Equal(t, &HelmView{Release: "datadog", Namespace: "datadog", Chart: "datadog", Version: "3.70.0", Revision: 5}, v.DDA.Helm)

	// Not rolling: current version only.
	s = loadScenario(t, "helm")
	for _, ds := range s.Objects[SourceDaemonSets] {
		desired, _, _ := unstructured.NestedInt64(ds.Object, "status", "desiredNumberScheduled")
		require.NoError(t, unstructured.SetNestedField(ds.Object, desired, "status", "updatedNumberScheduled"))
		require.NoError(t, unstructured.SetNestedField(ds.Object, int64(0), "status", "numberUnavailable"))
	}
	v, err = Build(s, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, "Complete", v.DDA.Rollout.Phase)
	assert.Equal(t, &HelmView{Release: "datadog", Namespace: "datadog", Chart: "datadog", Version: "3.70.0", Revision: 5}, v.DDA.Helm)

	// Not Helm-managed.
	assert.Nil(t, buildScenario(t, "steady").DDA.Helm)
}

func TestBuildRollout(t *testing.T) {
	v := buildScenario(t, "stalled")
	def := v.DDA.Default
	require.NotNil(t, def)

	ds := def.Agent
	assert.Equal(t, "Stalled", ds.Rollout.Phase)
	assert.Equal(t, "NoProgress: 1 pod CrashLoopBackOff", ds.Rollout.Reason)
	assert.Equal(t, rolloutSource, ds.Rollout.Source)
	assert.Equal(t, []PodView{{Name: "datadog-agent-agent-a1", Node: "worker-1", Reason: "CrashLoopBackOff"}}, ds.FailingPods)
	assert.Equal(t, BadgeDegraded, ds.Health)

	dca := def.ClusterAgent
	assert.Equal(t, "Failed", dca.Rollout.Phase)
	assert.Equal(t, BadgeError, dca.Health)

	onDelete := v.DDA.Profiles[0].DDAI.Agent
	assert.Equal(t, "Rolling", onDelete.Rollout.Phase, "OnDelete never Stalled")
	assert.Equal(t, []PodView{{Name: "legacy-nodes-agent-z9", Reason: "Unschedulable"}}, onDelete.FailingPods)

	assert.Equal(t, "Failed", v.DDA.Rollout.Phase)
	assert.Equal(t, BadgeError, v.DDA.Health)
	require.NotNil(t, v.DDA.Experiment)
	assert.Equal(t, "running", v.DDA.Experiment.Phase)

	// Without --pods: never Stalled, no failing pods.
	v, err := Build(loadScenario(t, "stalled"), BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, "Rolling", v.DDA.Default.Agent.Rollout.Phase)
	assert.Empty(t, v.DDA.Default.Agent.FailingPods)
	assert.Nil(t, v.DDA.Default.Agent.Rollout.LastProgress)
}

const rolloutSource = "heuristic"

// fakeAuthoritative decides every DaemonSet, standing in for an operator
// status evaluator.
type fakeAuthoritative struct{}

func (fakeAuthoritative) Name() string { return "operator-status" }

func (fakeAuthoritative) Evaluate(in rollout.Input) (rollout.Result, bool) {
	if in.Workload.Kind != rollout.KindDaemonSet {
		return rollout.Result{}, false
	}
	return rollout.Result{Phase: rollout.PhaseBaking, Updated: in.Workload.Updated, Desired: in.Workload.Desired}, true
}

// recordingObserver returns a fixed history and records the observed keys.
type recordingObserver struct {
	keys []string
	obs  Observation
}

func (r *recordingObserver) Observe(key string, _ rollout.Workload, _ time.Time) Observation {
	r.keys = append(r.keys, key)
	return r.obs
}

// Unavailable pods of a converged workload are Settling for the settling
// period after the convergence, then a Warning issue with a Degraded badge,
// and only above the --max-unavailable threshold.
func TestBuildUnavailableAfterUpdate(t *testing.T) {
	for name, tc := range map[string]struct {
		age         time.Duration
		unavailable int64
		approx      bool
		max         rollout.MaxUnavailable
		eval        rollout.Evaluator
		phase       string
		reason      string
		health      Badge
		warning     string
	}{
		"just updated":           {age: time.Minute, unavailable: 1, phase: "Settling", reason: rollout.ReasonPodsUnavailable, health: BadgeProgressing},
		"settling period over":   {age: rollout.DefaultSettlingPeriod, unavailable: 1, phase: "Complete", reason: rollout.ReasonAvailabilityBelowThreshold, health: BadgeDegraded, warning: "1 unavailable (> 0 allowed)"},
		"updated long ago":       {age: 20 * time.Minute, unavailable: 1, phase: "Complete", reason: rollout.ReasonAvailabilityBelowThreshold, health: BadgeDegraded, warning: "1 unavailable (> 0 allowed)"},
		"approximate start":      {age: time.Minute, approx: true, unavailable: 1, phase: "Complete", reason: rollout.ReasonAvailabilityBelowThreshold, health: BadgeDegraded, warning: "1 unavailable (> 0 allowed)"},
		"within count threshold": {age: 20 * time.Minute, unavailable: 1, max: rollout.MaxUnavailable{Value: 1}, phase: "Complete", health: BadgeHealthy},
		"above count threshold":  {age: 20 * time.Minute, unavailable: 2, max: rollout.MaxUnavailable{Value: 1}, phase: "Complete", reason: rollout.ReasonAvailabilityBelowThreshold, health: BadgeDegraded, warning: "2 unavailable (> 1 allowed)"},
		"percent rounds down":    {age: 20 * time.Minute, unavailable: 1, max: rollout.MaxUnavailable{Value: 1, Percent: true}, phase: "Complete", reason: rollout.ReasonAvailabilityBelowThreshold, health: BadgeDegraded, warning: "1 unavailable (> 0 allowed)"},
		"within percent":         {age: 20 * time.Minute, unavailable: 1, max: rollout.MaxUnavailable{Value: 50, Percent: true}, phase: "Complete", health: BadgeHealthy},
		"stalled":                {age: 20 * time.Minute, unavailable: 1, eval: stalledEvaluator{}, phase: "Stalled", health: BadgeDegraded},
	} {
		t.Run(name, func(t *testing.T) {
			s := loadScenario(t, "steady")
			for _, ds := range s.Objects[SourceDaemonSets] {
				if ds.GetName() == "datadog-agent-agent" {
					require.NoError(t, unstructured.SetNestedField(ds.Object, tc.unavailable, "status", "numberUnavailable"))
				}
			}
			for _, cr := range s.Objects[SourceControllerRevisions] {
				cr.SetCreationTimestamp(metav1.NewTime(testNow.Add(-tc.age)))
			}
			if tc.approx {
				s.Objects[SourceControllerRevisions] = append(s.Objects[SourceControllerRevisions], reusedRevision(t, s, "datadog-agent-agent", testNow.Add(-tc.age+time.Second)))
			}
			v, err := Build(s, BuildConfig{StallAfter: 5 * time.Minute, MaxUnavailable: tc.max, Evaluator: tc.eval}, nil, testNow)
			require.NoError(t, err)
			ds := v.DDA.Default.Agent
			assert.Equal(t, tc.phase, ds.Rollout.Phase)
			assert.Equal(t, tc.health, ds.Health)
			if tc.eval == nil {
				assert.Equal(t, tc.reason, ds.Rollout.Reason)
			}
			var warning string
			for _, c := range ds.Conditions {
				if c.Type == condUnavailable {
					assert.Equal(t, SeverityWarning, c.Severity)
					warning = c.Message
				}
			}
			assert.Equal(t, tc.warning, warning)
			require.NotNil(t, ds.Rollout.AvailabilityThreshold)
			assert.Equal(t, tc.max.Resolve(ds.Counts.Desired), *ds.Rollout.AvailabilityThreshold)
			if tc.phase == "Settling" {
				require.NotNil(t, ds.Rollout.SettlingUntil)
				assert.Equal(t, testNow.Add(-tc.age+rollout.DefaultSettlingPeriod), *ds.Rollout.SettlingUntil)
			} else {
				assert.Nil(t, ds.Rollout.SettlingUntil)
			}
		})
	}
}

// reusedRevision returns a copy of the newest ControllerRevision of the
// DaemonSet name, with an older revision number and created at created, so
// that the newest revision start is approximate.
func reusedRevision(t *testing.T, s *Snapshot, name string, created time.Time) *unstructured.Unstructured {
	t.Helper()
	for _, cr := range s.Objects[SourceControllerRevisions] {
		for _, r := range cr.GetOwnerReferences() {
			if r.Kind == kindDaemonSet && r.Name == name {
				old := cr.DeepCopy()
				old.SetName(cr.GetName() + "-old")
				old.SetLabels(map[string]string{controllerRevisionHashLabel: "old"})
				require.NoError(t, unstructured.SetNestedField(old.Object, controllerRevisionNumber(cr)-1, "revision"))
				old.SetCreationTimestamp(metav1.NewTime(created))
				return old
			}
		}
	}
	t.Fatalf("no ControllerRevision for %s", name)
	return nil
}

// stalledEvaluator reports every rollout Stalled.
type stalledEvaluator struct{}

func (stalledEvaluator) Name() string { return "stalled" }

func (stalledEvaluator) Evaluate(in rollout.Input) (rollout.Result, bool) {
	return rollout.Result{Phase: rollout.PhaseStalled, Updated: in.Workload.Updated, Desired: in.Workload.Desired}, true
}

func TestBuildEvaluatorAndObserver(t *testing.T) {
	lastProgress := testNow.Add(-2 * time.Minute)
	obs := &recordingObserver{obs: Observation{LastProgress: lastProgress}}
	cfg := BuildConfig{Evaluator: rollout.Chain{fakeAuthoritative{}, rollout.HeuristicEvaluator{}}}
	v, err := Build(loadScenario(t, "steady"), cfg, obs, testNow)
	require.NoError(t, err)

	assert.Equal(t, "Baking", v.DDA.Default.Agent.Rollout.Phase)
	assert.Equal(t, "operator-status", v.DDA.Default.Agent.Rollout.Source)
	assert.Equal(t, "heuristic", v.DDA.Default.ClusterAgent.Rollout.Source)
	assert.Equal(t, BadgeProgressing, v.DDA.Health)

	// The in-memory last progress reaches the view.
	require.NotNil(t, v.DDA.Default.ClusterAgent.Rollout.LastProgress)
	assert.Equal(t, lastProgress, *v.DDA.Default.ClusterAgent.Rollout.LastProgress)
	assert.ElementsMatch(t, []string{
		"DaemonSet/datadog/datadog-agent-agent",
		"Deployment/datadog/datadog-agent-cluster-agent",
		"Deployment/datadog/datadog-agent-cluster-checks-runner",
	}, obs.keys)
}

func TestBuildDedup(t *testing.T) {
	v := buildScenario(t, "reconcile-errors")

	find := func(kind, name, typ string) []Cond {
		var out []Cond
		for _, c := range v.Issues {
			if c.Object.Kind == kind && c.Object.Name == name && c.Type == typ {
				out = append(out, c)
			}
		}
		return out
	}
	// (a) + (c): the default DDAI error is shown once, on the DDAI.
	got := find(kindDDAI, "datadog-agent", condDDAReconcileError)
	require.Len(t, got, 1)
	assert.Equal(t, []ObjectRef{{Kind: kindDDA, Namespace: "datadog", Name: "datadog-agent"}}, got[0].AlsoReportedBy)
	assert.Empty(t, find(kindDDA, "datadog-agent", condDDAIReconcileError))
	assert.Empty(t, find(kindDDA, "datadog-agent", condDDAReconcileError))

	// (b): the profile DDAI error is shown once, on the DDAI.
	got = find(kindDDAI, "canary", condDDAReconcileError)
	require.Len(t, got, 1)
	assert.Equal(t, []ObjectRef{{Kind: kindDAP, Namespace: "datadog", Name: "canary"}}, got[0].AlsoReportedBy)
	assert.Empty(t, find(kindDAP, "canary", condDDAIReconcileError))
	canary := v.DDA.Profiles[1]
	require.Equal(t, "canary", canary.Name)
	assert.Empty(t, canary.Conditions)
	assert.Equal(t, BadgeError, canary.Health, "parent rollup keeps the severity")

	// Unknown mirror format: falls through, both shown.
	assert.Len(t, find(kindDDAI, "legacy", condDDAReconcileError), 1)
	assert.Len(t, find(kindDAP, "legacy", condDDAIReconcileError), 1)

	// Exact dedup: kept on the lowest object.
	got = find(kindDDAI, "datadog-agent", condOverrideReconcileConflict)
	require.Len(t, got, 1)
	assert.Equal(t, SeverityWarning, got[0].Severity)
	assert.Empty(t, find(kindDDA, "datadog-agent", condOverrideReconcileConflict))

	// DDA-only conditions stay on the DDA.
	require.Len(t, v.DDA.Conditions, 1)
	assert.Equal(t, condDeprecatedConfigInUse, v.DDA.Conditions[0].Type)

	// Invalid DAP is an Error.
	assert.Equal(t, SeverityError, find(kindDAP, "badspec", condValid)[0].Severity)
	assert.Equal(t, SeverityWarning, find(kindDAP, "badspec", condApplied)[0].Severity)

	// Sorted by severity, then object.
	for i := 1; i < len(v.Issues); i++ {
		assert.GreaterOrEqual(t, v.Issues[i-1].Severity.rank(), v.Issues[i].Severity.rank())
	}
	assert.Equal(t, BadgeError, v.DDA.Health)
}

func TestBuildSummary(t *testing.T) {
	v := buildScenario(t, "summary")
	byKind := map[string]SummaryView{}
	for _, row := range v.Summary {
		byKind[row.Kind] = row
	}
	assert.NotContains(t, byKind, "DatadogBYOCCluster", "not installed kinds are hidden")
	assert.Equal(t, SourceForbidden, byKind["DatadogSLO"].State, "no access")

	mon := byKind["DatadogMonitor"]
	assert.Equal(t, [4]int{5, 3, 1, 1}, [4]int{mon.Total, mon.OK, mon.Errors, mon.Unknown})
	assert.Equal(t, map[string]int{"OK": 1, "Alert": 1, "Other": 1}, mon.Breakdown, "monitor state breakdown")
	assert.Equal(t, []SummaryFailure{{Namespace: "app", Name: "m-error", Message: "400 Bad Request: invalid query"}}, mon.Failures)

	gr := byKind["DatadogGenericResource"]
	assert.Equal(t, map[string]int{"monitor": 1, "downtime": 1}, gr.Breakdown)
	assert.Equal(t, 1, gr.Errors)
	assert.Equal(t, 1, byKind["DatadogDashboard"].Errors)
	assert.Equal(t, 1, byKind["DatadogMetric"].Errors)
	assert.Equal(t, [2]int{1, 1}, [2]int{byKind["DatadogInstrumentation"].OK, byKind["DatadogInstrumentation"].Unknown})
	assert.Equal(t, 1, byKind["DatadogCSIDriver"].OK)

	// Display order follows SummaryKinds.
	var kinds []string
	for _, row := range v.Summary {
		kinds = append(kinds, row.Kind)
	}
	assert.Equal(t, []string{"DatadogMonitor", "DatadogDashboard", "DatadogSLO", "DatadogGenericResource", "DatadogMetric", "DatadogInstrumentation", "DatadogCSIDriver"}, kinds)
}

// TestViewHasNoSecrets checks that nothing outside the View model leaks into
// the JSON: stripped fields, operator status tokens and secret references.
func TestViewHasNoSecrets(t *testing.T) {
	for _, name := range goldenScenarios {
		out, err := json.Marshal(buildScenario(t, name))
		require.NoError(t, err)
		for _, s := range []string{"must-never-be-copied", "must be stripped", "datadog-secret", "managedFields"} {
			assert.NotContains(t, string(out), s, name)
		}
	}
}

// A Disconnected source keeps the objects of its last read: the live view
// shows them, with the same health, rather than an error.
func TestBuildDisconnectedSources(t *testing.T) {
	want := buildScenario(t, "steady")
	s := loadScenario(t, "steady")
	for _, key := range []string{SourceDDA, SourceDDAI, SourceDaemonSets, SourceDeployments, SourceControllerRevisions, SourceReplicaSets} {
		info := s.Sources[key]
		info.State, info.Err = SourceDisconnected, "connection refused"
		s.Sources[key] = info
	}
	v, err := Build(s, BuildConfig{}, nil, testNow)
	require.NoError(t, err)
	assert.Equal(t, want.DDA, v.DDA)
}

// Stale pods or workloads are shown, but no Stalled verdict comes from them.
func TestBuildDisconnectedNotStalled(t *testing.T) {
	for _, key := range []string{SourcePods, SourceDaemonSets} {
		t.Run(key, func(t *testing.T) {
			s := loadScenario(t, "stalled")
			info := s.Sources[key]
			info.State, info.Err = SourceDisconnected, "connection refused"
			s.Sources[key] = info
			v, err := Build(s, FixtureBuildConfig("stalled"), NopObserver{}, testNow)
			require.NoError(t, err)
			ds := v.DDA.Default.Agent
			assert.Equal(t, "Rolling", ds.Rollout.Phase)
			assert.Equal(t, []PodView{{Name: "datadog-agent-agent-a1", Node: "worker-1", Reason: "CrashLoopBackOff"}}, ds.FailingPods, "stale pods are still shown")
		})
	}
}

func TestBuildPartialData(t *testing.T) {
	s := loadScenario(t, "steady")
	// The DDAI names a Deployment that is gone, and DaemonSets can't be
	// listed.
	s.Objects[SourceDeployments] = s.Objects[SourceDeployments][:1]
	s.Objects[SourceDaemonSets] = nil
	s.Sources[SourceDaemonSets] = SourceInfo{State: SourceForbidden, Err: "forbidden"}
	v, err := Build(s, BuildConfig{}, nil, testNow)
	require.NoError(t, err)

	def := v.DDA.Default
	require.NotNil(t, def.Agent)
	assert.True(t, def.Agent.Missing)
	assert.Equal(t, BadgeUnknown, def.Agent.Health)
	require.NotNil(t, def.ClusterChecksRunner)
	assert.True(t, def.ClusterChecksRunner.Missing)
	assert.False(t, def.ClusterAgent.Missing)
	assert.Equal(t, BadgeUnknown, v.DDA.Health)
}
