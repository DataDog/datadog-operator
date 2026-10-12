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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/DataDog/datadog-operator/pkg/rollout"
)

func profileNames(v View) []string {
	names := make([]string, len(v.DDA.Profiles))
	for i, p := range v.DDA.Profiles {
		names[i] = p.Name
	}
	return names
}

func buildStale(t *testing.T, cfg BuildConfig) View {
	t.Helper()
	v, err := Build(loadScenario(t, "stale-daps"), cfg, NopObserver{}, testNow)
	require.NoError(t, err)
	return v
}

// --hide-empty hides only the healthy profiles with an empty agent
// DaemonSet; problems never disappear and the totals are unchanged.
func TestBuildHideEmpty(t *testing.T) {
	all := buildStale(t, BuildConfig{})
	assert.Equal(t, []string{"alpha", "beta", "broken", "conflict", "delta", "gamma", "nostatus", "pending", "rolling", "zeta"}, profileNames(all))
	assert.Zero(t, all.DDA.HiddenProfiles)
	assert.Nil(t, all.DDA.HiddenProfileNames)

	v := buildStale(t, BuildConfig{HideEmpty: true})
	// Kept: reconcile error (broken), conflict without DDAI, no status,
	// DaemonSet not found yet (pending), rollout in progress (rolling).
	assert.Equal(t, []string{"beta", "broken", "conflict", "gamma", "nostatus", "pending", "rolling", "zeta"}, profileNames(v))
	assert.Equal(t, 2, v.DDA.HiddenProfiles)
	assert.Equal(t, []string{"datadog/alpha", "datadog/delta"}, v.DDA.HiddenProfileNames)

	require.NotNil(t, v.DDA.Default, "the default DDAI is never hidden")
	assert.Equal(t, all.DDA.Default, v.DDA.Default)
	assert.Equal(t, all.DDA.Agents, v.DDA.Agents)
	assert.Equal(t, Counts{Desired: 12, Ready: 12, UpToDate: 12, Available: 12}, v.DDA.Agents)
	assert.Equal(t, all.DDA.Rollout, v.DDA.Rollout)
	assert.Equal(t, all.DDA.Health, v.DDA.Health)
	assert.Equal(t, all.Issues, v.Issues)
	assert.Equal(t, all.Unattached, v.Unattached)
}

// The default DDAI with 0 desired pods is still shown: only profiles hide.
func TestBuildHideEmptyKeepsDefault(t *testing.T) {
	snap := loadScenario(t, "stale-daps")
	for _, u := range snap.Objects[SourceDaemonSets] {
		if u.GetName() == "datadog-agent-agent" {
			setDesired(t, u, 0)
		}
	}
	v, err := Build(snap, BuildConfig{HideEmpty: true}, NopObserver{}, testNow)
	require.NoError(t, err)
	require.NotNil(t, v.DDA.Default)
	require.NotNil(t, v.DDA.Default.Agent)
	assert.Zero(t, v.DDA.Default.Agent.Counts.Desired)
}

// Each rebuild applies --hide-empty again: a profile that scales up shows
// again, one that scales down to 0 hides (live mode).
func TestBuildHideEmptyFollowsScaling(t *testing.T) {
	snap := loadScenario(t, "stale-daps")
	for _, u := range snap.Objects[SourceDaemonSets] {
		switch u.GetName() {
		case "alpha-agent":
			setDesired(t, u, 3)
		case "beta-agent":
			setDesired(t, u, 0)
		}
	}
	v, err := Build(snap, BuildConfig{HideEmpty: true}, NopObserver{}, testNow)
	require.NoError(t, err)
	assert.Equal(t, []string{"alpha", "broken", "conflict", "gamma", "nostatus", "pending", "rolling", "zeta"}, profileNames(v))
	assert.Equal(t, []string{"datadog/beta", "datadog/delta"}, v.DDA.HiddenProfileNames)
}

// --sort=desired orders by desired pods, most first; ties keep the name
// order and profiles without a DaemonSet come last.
func TestBuildSortDesired(t *testing.T) {
	v := buildStale(t, BuildConfig{Sort: SortDesired})
	assert.Equal(t, []string{"beta", "gamma", "zeta", "alpha", "broken", "delta", "rolling", "conflict", "nostatus", "pending"}, profileNames(v))
	assert.Equal(t, "datadog-agent", v.DDA.Default.Name, "the default DDAI stays first")

	byName := buildStale(t, BuildConfig{Sort: SortName})
	assert.Equal(t, profileNames(buildStale(t, BuildConfig{})), profileNames(byName))
	assert.Equal(t, byName.DDA.Agents, v.DDA.Agents)

	both := buildStale(t, BuildConfig{Sort: SortDesired, HideEmpty: true})
	assert.Equal(t, []string{"beta", "gamma", "zeta", "broken", "rolling", "conflict", "nostatus", "pending"}, profileNames(both))
}

func TestParseProfileSort(t *testing.T) {
	for in, want := range map[string]ProfileSort{"name": SortName, "desired": SortDesired} {
		got, err := ParseProfileSort(in)
		require.NoError(t, err, in)
		assert.Equal(t, want, got, in)
	}
	for _, in := range []string{"", "Desired", "ready", " name"} {
		_, err := ParseProfileSort(in)
		assert.Error(t, err, in)
	}
	_, err := Build(loadScenario(t, "stale-daps"), BuildConfig{Sort: "bogus"}, NopObserver{}, testNow)
	assert.ErrorContains(t, err, `unsupported profile sort "bogus"`)
}

func setDesired(t *testing.T, ds *unstructured.Unstructured, desired int64) {
	t.Helper()
	for _, f := range []string{"desiredNumberScheduled", "numberReady", "numberAvailable", "updatedNumberScheduled"} {
		require.NoError(t, unstructured.SetNestedField(ds.Object, desired, "status", f))
	}
}

func issueObjects(v View) []string {
	out := make([]string, len(v.Issues))
	for i, c := range v.Issues {
		out[i] = c.Object.String()
	}
	return out
}

// --hide-empty-force also hides the empty profiles with warnings and
// removes their issues; health and totals are computed before hiding.
func TestBuildHideEmptyForce(t *testing.T) {
	build := func(cfg BuildConfig) View {
		v, err := Build(loadScenario(t, "stale-warnings"), cfg, NopObserver{}, testNow)
		require.NoError(t, err)
		return v
	}
	all := build(BuildConfig{})
	assert.Equal(t, []string{"batch", "cache", "dup", "edge", "gpu", "ingest", "legacy", "spot", "web"}, profileNames(all))
	assert.Len(t, all.Issues, 7)

	plain := build(BuildConfig{HideEmpty: true})
	assert.Equal(t, []string{"batch", "cache", "dup", "edge", "gpu", "legacy", "web"}, profileNames(plain))
	assert.Equal(t, FilterHideEmpty, plain.DDA.HiddenBy)
	assert.Equal(t, 2, plain.DDA.HiddenProfiles)
	assert.Equal(t, 2, plain.DDA.HiddenProfilesWithoutWarnings)
	assert.Zero(t, plain.DDA.HiddenProfilesWithWarnings)
	assert.Zero(t, plain.DDA.HiddenIssues)
	assert.Equal(t, all.Issues, plain.Issues)

	for _, cfg := range []BuildConfig{{HideEmptyForce: true}, {HideEmptyForce: true, HideEmpty: true}} {
		v := build(cfg)
		assert.Equal(t, []string{"dup", "web"}, profileNames(v))
		assert.Equal(t, FilterHideEmptyForce, v.DDA.HiddenBy)
		assert.Equal(t, 7, v.DDA.HiddenProfiles)
		assert.Equal(t, []string{"datadog/batch", "datadog/cache", "datadog/edge", "datadog/gpu", "datadog/ingest", "datadog/legacy", "datadog/spot"}, v.DDA.HiddenProfileNames)
		assert.Equal(t, 2, v.DDA.HiddenProfilesWithoutWarnings)
		assert.Equal(t, 5, v.DDA.HiddenProfilesWithWarnings)
		assert.Equal(t, 5, v.DDA.HiddenIssues)
		assert.Equal(t, []string{"DatadogAgentInternal/datadog/web", "DatadogAgentProfile/datadog/dup"}, issueObjects(v))
		assert.Len(t, all.Issues, len(v.Issues)+v.DDA.HiddenIssues)

		assert.Equal(t, BadgeDegraded, v.DDA.Health, "the hidden warnings still count")
		assert.Equal(t, all.DDA.Health, v.DDA.Health)
		assert.Equal(t, all.DDA.Agents, v.DDA.Agents)
		assert.Equal(t, all.DDA.Rollout, v.DDA.Rollout)
		assert.Equal(t, all.DDA.Default, v.DDA.Default)
		assert.Equal(t, all.DDA.Conditions, v.DDA.Conditions)
	}
}

// --hide-empty-force keeps the default DDAI and the profiles with no DDAI,
// no status, no DaemonSet yet or a rollout in progress; a 0-desired profile
// with an error is hidden with its issue.
func TestBuildHideEmptyForceNeverHides(t *testing.T) {
	all := buildStale(t, BuildConfig{})
	v := buildStale(t, BuildConfig{HideEmptyForce: true})
	assert.Equal(t, []string{"beta", "conflict", "gamma", "nostatus", "pending", "rolling", "zeta"}, profileNames(v))
	assert.Equal(t, []string{"datadog/alpha", "datadog/broken", "datadog/delta"}, v.DDA.HiddenProfileNames)
	assert.Equal(t, 2, v.DDA.HiddenProfilesWithoutWarnings)
	assert.Equal(t, 1, v.DDA.HiddenProfilesWithWarnings)
	assert.Equal(t, 1, v.DDA.HiddenIssues)
	assert.NotContains(t, issueObjects(v), "DatadogAgentInternal/datadog/broken")
	assert.Contains(t, issueObjects(all), "DatadogAgentInternal/datadog/broken")
	assert.Contains(t, issueObjects(v), "DatadogAgentProfile/datadog/conflict")
	assert.Equal(t, BadgeError, v.DDA.Health, "the hidden error still counts")
	assert.Equal(t, all.DDA.Agents, v.DDA.Agents)
	require.NotNil(t, v.DDA.Default)

	snap := loadScenario(t, "stale-daps")
	for _, u := range snap.Objects[SourceDaemonSets] {
		if u.GetName() == "datadog-agent-agent" {
			setDesired(t, u, 0)
		}
	}
	v, err := Build(snap, BuildConfig{HideEmptyForce: true}, NopObserver{}, testNow)
	require.NoError(t, err)
	require.NotNil(t, v.DDA.Default, "the default DDAI is never hidden")
	assert.Zero(t, v.DDA.Default.Agent.Counts.Desired)
}

// A profile with a 0-desired agent DaemonSet but another workload with
// desired pods is never hidden.
func TestForceEmptyProfileWorkloads(t *testing.T) {
	d := &DAPView{Health: BadgeDegraded, DDAI: &DDAIView{
		Health:  BadgeDegraded,
		Rollout: Rollout{Phase: "Complete"},
		Agent:   &WorkloadView{Counts: Counts{}},
	}}
	assert.True(t, forceEmptyProfile(d))
	assert.False(t, emptyProfile(d))
	d.DDAI.ClusterChecksRunner = &WorkloadView{Counts: Counts{Desired: 1}}
	assert.False(t, forceEmptyProfile(d))
	d.DDAI.ClusterChecksRunner = &WorkloadView{Missing: true}
	assert.False(t, forceEmptyProfile(d))
	d.DDAI.ClusterChecksRunner = nil
	d.DDAI.Rollout.Phase = "Unknown"
	assert.False(t, forceEmptyProfile(d))
	d.DDAI.Rollout.Phase = "Complete"
	d.StatusUnknown = true
	assert.False(t, forceEmptyProfile(d))
}

// --hide-empty-force requires a Complete rollout: a profile Settling is
// shown, one Complete with unavailable pods above the threshold is
// hidden with its warning; --hide-empty keeps it, as it is Degraded.
func TestBuildHideEmptyAvailability(t *testing.T) {
	build := func(revisionAge time.Duration, cfg BuildConfig) View {
		snap := loadScenario(t, "availability")
		for _, u := range snap.Objects[SourceDaemonSets] {
			if u.GetName() == "large-agent" {
				// A contrived status: no desired pod, one still unavailable.
				setDesired(t, u, 0)
				require.NoError(t, unstructured.SetNestedField(u.Object, int64(1), "status", "numberUnavailable"))
			}
		}
		for _, u := range snap.Objects[SourceControllerRevisions] {
			if u.GetName() == "large-agent-d4e5f6" {
				u.SetCreationTimestamp(metav1.NewTime(testNow.Add(-revisionAge)))
			}
		}
		v, err := Build(snap, cfg, NopObserver{}, testNow)
		require.NoError(t, err)
		return v
	}

	settling := build(time.Minute, BuildConfig{HideEmptyForce: true})
	assert.Equal(t, []string{"large", "small"}, profileNames(settling))
	assert.Equal(t, "Settling", settling.DDA.Profiles[0].DDAI.Rollout.Phase)
	assert.Equal(t, BadgeProgressing, settling.DDA.Profiles[0].Health)

	degraded := build(time.Hour, BuildConfig{HideEmpty: true})
	assert.Equal(t, []string{"large", "small"}, profileNames(degraded))
	assert.Equal(t, "Complete", degraded.DDA.Profiles[0].DDAI.Rollout.Phase)
	assert.Equal(t, BadgeDegraded, degraded.DDA.Profiles[0].Health)
	assert.Contains(t, issueObjects(degraded), "DaemonSet/datadog/large-agent")

	hidden := build(time.Hour, BuildConfig{HideEmptyForce: true})
	assert.Equal(t, []string{"small"}, profileNames(hidden))
	assert.Equal(t, []string{"datadog/large"}, hidden.DDA.HiddenProfileNames)
	assert.Equal(t, 1, hidden.DDA.HiddenProfilesWithWarnings)
	assert.NotContains(t, issueObjects(hidden), "DaemonSet/datadog/large-agent")
}

func TestForceEmptyProfilePhases(t *testing.T) {
	d := &DAPView{DDAI: &DDAIView{Agent: &WorkloadView{}}}
	for phase, want := range map[rollout.Phase]bool{
		rollout.PhaseComplete: true,
		rollout.PhaseSettling: false,
		rollout.PhaseRolling:  false,
		rollout.PhaseStalled:  false,
	} {
		d.DDAI.Rollout = Rollout{Phase: string(phase)}
		assert.Equal(t, want, forceEmptyProfile(d), phase)
	}
	d.DDAI.Rollout = Rollout{Phase: string(rollout.PhaseComplete), Reason: rollout.ReasonAvailabilityBelowThreshold}
	assert.True(t, forceEmptyProfile(d))
}
