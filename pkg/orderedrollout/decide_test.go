// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orderedrollout

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

var testNow = time.Date(2026, 10, 5, 12, 0, 0, 0, time.UTC)

type stepState string

const (
	stUnchanged        stepState = "unchanged"
	stNotStarted       stepState = "not-started"
	stMissing          stepState = "missing"
	stWrittenUnacked   stepState = "written-unacked"
	stReconcileError   stepState = "reconcile-error"
	stRolling          stepState = "rolling"
	stBaking           stepState = "baking"
	stCompleted        stepState = "completed-latched"
	stRegressed        stepState = "completed-regressed"
	stTimedOutHalt     stepState = "timed-out-halt"
	stTimedOutContinue stepState = "timed-out-continue"
)

var allStates = []stepState{stUnchanged, stNotStarted, stMissing, stWrittenUnacked, stReconcileError, stRolling, stBaking, stCompleted, stRegressed, stTimedOutHalt, stTimedOutContinue}

func healthyDS() *DaemonSetFacts {
	return &DaemonSetFacts{Generation: 2, ObservedGeneration: 2, DesiredNumberScheduled: 3, UpdatedNumberScheduled: 3}
}

func unhealthyDS() *DaemonSetFacts {
	return &DaemonSetFacts{Generation: 2, ObservedGeneration: 2, DesiredNumberScheduled: 3, UpdatedNumberScheduled: 3, NumberUnavailable: 3}
}

func ago(d time.Duration) *metav1.Time {
	t := metav1.NewTime(testNow.Add(-d))
	return &t
}

// buildStep returns the facts and the previous status entry for a step in state st.
func buildStep(kind v2alpha1.RolloutStepKind, name string, priority int32, st stepState) (StepFacts, *v2alpha1.RolloutStepStatus) {
	ns := ""
	if kind == v2alpha1.RolloutStepKindProfile {
		ns = "ns"
	}
	target := "T-" + name
	s := StepFacts{Kind: kind, Name: name, Namespace: ns, TargetHash: target, Config: DefaultConfig()}
	s.Config.Priority = priority
	acked := &LiveFacts{TargetHash: target, Generation: 2, ObservedGeneration: 2, HasAgent: true, DaemonSet: healthyDS()}
	started := &v2alpha1.RolloutStepStatus{Kind: kind, Name: name, Namespace: ns, TargetHash: target, Phase: v2alpha1.RolloutPhaseInProgress, Reason: ReasonRolling, StartedAt: ago(time.Minute)}

	switch st {
	case stUnchanged:
		// Unhealthy on purpose: unchanged steps never gate on live health.
		s.Live = &LiveFacts{TargetHash: target, Generation: 2, ObservedGeneration: 1, ReconcileError: true, HasAgent: true, DaemonSet: unhealthyDS()}
		return s, nil
	case stNotStarted:
		s.Live = &LiveFacts{TargetHash: "O-" + name, Generation: 1, ObservedGeneration: 1, HasAgent: true, DaemonSet: healthyDS()}
		return s, nil
	case stMissing:
		return s, nil
	case stWrittenUnacked:
		s.Live = acked
		s.Live.ObservedGeneration = 1
		return s, started
	case stReconcileError:
		s.Live = acked
		s.Live.ReconcileError = true
		return s, started
	case stRolling:
		s.Live = acked
		s.Live.DaemonSet.UpdatedNumberScheduled = 1
		return s, started
	case stBaking:
		s.Live = acked
		s.Config.StepSoak = 10 * time.Minute
		started.SoakStartedAt = ago(time.Minute)
		return s, started
	case stCompleted, stRegressed:
		s.Live = acked
		if st == stRegressed {
			s.Live.DaemonSet = unhealthyDS()
		}
		started.Phase, started.Reason, started.CompletedAt = v2alpha1.RolloutPhaseCompleted, ReasonComplete, ago(30*time.Second)
		return s, started
	case stTimedOutHalt, stTimedOutContinue:
		s.Live = acked
		s.Live.DaemonSet.UpdatedNumberScheduled = 1
		s.Config.StepTimeout = time.Hour
		s.Config.OnStepTimeout = v2alpha1.RolloutTimeoutActionContinue
		if st == stTimedOutHalt {
			s.Config.OnStepTimeout = v2alpha1.RolloutTimeoutActionHalt
		}
		started.StartedAt = ago(2 * time.Hour)
		return s, started
	}
	panic(st)
}

type bypassCase string

const (
	bypassNone   bypassCase = "none"
	bypassNew    bypassCase = "new"
	bypassActive bypassCase = "active"
	bypassStale  bypassCase = "stale"
)

type matrixCase struct {
	priorities [3]int32
	states     [3]stepState
	ddaHold    bool
	holdA      bool
	skipB      bool
	bypass     bypassCase
}

func (c matrixCase) String() string {
	return fmt.Sprintf("prio=%v states=%v ddaHold=%t holdA=%t skipB=%t bypass=%s", c.priorities, c.states, c.ddaHold, c.holdA, c.skipB, c.bypass)
}

func (c matrixCase) inputs() Inputs {
	var steps []StepFacts
	prev := &v2alpha1.RolloutStatus{}
	kinds := []v2alpha1.RolloutStepKind{v2alpha1.RolloutStepKindDefault, v2alpha1.RolloutStepKindProfile, v2alpha1.RolloutStepKindProfile}
	names := []string{"default", "a", "b"}
	for i := range 3 {
		s, p := buildStep(kinds[i], names[i], c.priorities[i], c.states[i])
		s.CreationTime = testNow.Add(time.Duration(i) * time.Second)
		steps = append(steps, s)
		if p != nil {
			prev.Steps = append(prev.Steps, *p)
		}
	}
	steps[1].Hold = c.holdA
	steps[2].Skip = c.skipB

	targets := map[string]string{}
	for _, s := range steps {
		targets[s.Key()] = s.TargetHash
	}
	in := Inputs{Steps: steps, DDAHold: c.ddaHold, Prev: prev, Generation: 3, Now: testNow}
	switch c.bypass {
	case bypassNew:
		in.Bypass, prev.LastBypass = "b2", "b1"
	case bypassActive:
		in.Bypass, prev.LastBypass, prev.BypassRolloutID = "b1", "b1", RolloutID(targets)
	case bypassStale:
		in.Bypass, prev.LastBypass, prev.BypassRolloutID = "b1", "b1", "sha256:old"
	}
	return in
}

func writesByKey(d Decisions) map[string]Write {
	out := map[string]Write{}
	for _, w := range d.Writes {
		out[w.Key] = w
	}
	return out
}

func stepStatus(d Decisions, key string) *v2alpha1.RolloutStepStatus {
	for i := range d.Steps {
		if d.Steps[i].Key == key {
			return &d.Steps[i].Status
		}
	}
	return nil
}

func prevStep(in Inputs, key string) *v2alpha1.RolloutStepStatus {
	if in.Prev == nil {
		return nil
	}
	for i := range in.Prev.Steps {
		p := &in.Prev.Steps[i]
		if StepKey(p.Kind, p.Namespace, p.Name) == key {
			return p
		}
	}
	return nil
}

// expectedWarnings recomputes the §6.2 warning conditions from the decided step statuses.
func expectedWarnings(in Inputs, d Decisions) []string {
	if d.Mode != ModeOrdered {
		return nil
	}
	cfg := map[string]Config{}
	kinds := map[string]v2alpha1.RolloutStepKind{}
	for _, s := range in.Steps {
		cfg[s.Key()], kinds[s.Key()] = s.Config, s.Kind
	}
	incompleteWaves := map[int32]bool{}
	active := int32(-1)
	defaultPriority := int32(-1)
	for _, sd := range d.Steps {
		c := cfg[sd.Key]
		if kinds[sd.Key] == v2alpha1.RolloutStepKindDefault {
			defaultPriority = c.Priority
		}
		if !orderingComplete(&sd.Status, c) {
			incompleteWaves[c.Priority] = true
			if active < 0 || c.Priority < active {
				active = c.Priority
			}
		}
	}
	var out []string
	if len(incompleteWaves) > 1 {
		for _, sd := range d.Steps {
			c := cfg[sd.Key]
			if c.Priority == active && !orderingComplete(&sd.Status, c) && c.StepTimeout == 0 {
				out = append(out, WarningNoStepTimeout)
				break
			}
		}
	}
	for _, sd := range d.Steps {
		c := cfg[sd.Key]
		if kinds[sd.Key] == v2alpha1.RolloutStepKindProfile && defaultPriority >= 0 && c.Priority < defaultPriority && !orderingComplete(&sd.Status, c) {
			out = append(out, WarningDefaultNotFirst)
			break
		}
	}
	return out
}

func assertInvariants(t *testing.T, name string, in Inputs, d Decisions) {
	t.Helper()
	writes := writesByKey(d)
	require.Len(t, writes, len(d.Writes), "%s: duplicate writes", name)

	// Creations come first.
	seenOther := false
	for _, w := range d.Writes {
		if w.Kind != WriteCreate {
			seenOther = true
		} else {
			require.False(t, seenOther, "%s: creation after another write", name)
		}
	}

	fast := d.Mode == ModeParallel && !d.Bypass
	if fast {
		require.True(t, d.Status == nil || d.Status.Phase == v2alpha1.RolloutPhaseIdle, "%s: fast path keeps no rollout status", name)
	} else {
		require.NotNil(t, d.Status, name)
		require.Len(t, d.Status.Steps, len(in.Steps), name)
	}

	for _, s := range in.Steps {
		key := s.Key()
		w, written := writes[key]
		isPending := s.Live != nil && s.Live.TargetHash != s.TargetHash
		specWrite := written && (w.Kind == WriteCreate || w.Kind == WriteUpdate)

		// I-3: every missing DDAI is created.
		if s.Live == nil {
			require.True(t, written && w.Kind == WriteCreate, "%s: %s missing but not created", name, key)
			continue
		}
		require.False(t, written && w.Kind == WriteCreate, "%s: %s exists but created", name, key)
		// Only pending steps get rendered spec writes outside the fast path; others get metadata patches only.
		if !fast && !isPending {
			require.False(t, written && w.Kind == WriteUpdate, "%s: %s at target got a spec write", name, key)
		}

		if fast {
			require.True(t, written && w.Kind == WriteUpdate, "%s: fast path must write %s", name, key)
			continue
		}

		st := stepStatus(d, key)
		require.NotNil(t, st, name)
		p := prevStep(in, key)
		if p != nil && p.TargetHash != s.TargetHash {
			p = nil
		}

		// I-4: bypass allows every pending step that is not skipped.
		if d.Bypass && isPending {
			require.Equal(t, !s.Skip, specWrite, "%s: bypass write for %s", name, key)
		}
		if d.Mode == ModeOrdered && specWrite {
			// I-1: no step is allowed above the active wave.
			require.NotNil(t, d.Status.CurrentPriority, name)
			require.Equal(t, *d.Status.CurrentPriority, s.Config.Priority, "%s: %s written outside the active wave", name, key)
			// I-2: a DDA hold allows no spec writes.
			require.False(t, in.DDAHold, "%s: %s spec written under DDA hold", name, key)
			require.False(t, s.Hold, "%s: held %s written", name, key)
			require.False(t, s.Skip, "%s: skipped %s written", name, key)
		}

		// I-5: latched completion stays complete.
		if p != nil && p.Phase == v2alpha1.RolloutPhaseCompleted && p.Reason == ReasonComplete && !isPending {
			require.Equal(t, v2alpha1.RolloutPhaseCompleted, st.Phase, "%s: latch lost for %s", name, key)
			require.True(t, orderingComplete(st, s.Config), name)
		}
		// I-6: unchanged and never started is ordering-complete regardless of live health.
		if p == nil && !isPending {
			require.Equal(t, v2alpha1.RolloutPhaseCompleted, st.Phase, "%s: %s", name, key)
			require.Equal(t, ReasonUnchanged, st.Reason, "%s: %s", name, key)
		}
		// I-7: startedAt is set only when entering InProgress, and is otherwise carried.
		if st.StartedAt != nil && (p == nil || p.StartedAt == nil) {
			require.True(t, st.StartedAt.Time.Equal(testNow), "%s: %s startedAt not now", name, key)
			require.Equal(t, v2alpha1.RolloutPhaseInProgress, st.Phase, "%s: %s", name, key)
			require.Equal(t, ReasonSpecApplying, st.Reason, "%s: %s", name, key)
			require.True(t, specWrite, "%s: %s started without a write", name, key)
		}
		if p != nil && p.StartedAt != nil && !isPending {
			require.Equal(t, p.StartedAt, st.StartedAt, "%s: %s startedAt changed", name, key)
		}
		// Held steps never get spec writes.
		if st.Phase == v2alpha1.RolloutPhaseHeld || st.Phase == v2alpha1.RolloutPhasePending || st.Phase == v2alpha1.RolloutPhaseSkipped {
			require.False(t, specWrite, "%s: %s in %s written", name, key, st.Phase)
		}
	}

	// I-9: warnings are present exactly when their conditions hold.
	if d.Status != nil && !fast {
		require.ElementsMatch(t, expectedWarnings(in, d), d.Status.Warnings, name)
	}
}

func TestDecideMatrix(t *testing.T) {
	priorities := [][3]int32{{0, 0, 0}, {0, 0, 10}, {10, 0, 10}, {0, 5, 10}}
	defaultStates := []stepState{stUnchanged, stNotStarted, stMissing, stRolling, stCompleted, stTimedOutHalt}
	count := 0
	for _, prio := range priorities {
		for _, sd := range defaultStates {
			for _, sa := range allStates {
				for _, sb := range allStates {
					for _, ddaHold := range []bool{false, true} {
						for _, holdA := range []bool{false, true} {
							for _, skipB := range []bool{false, true} {
								for _, bypass := range []bypassCase{bypassNone, bypassNew, bypassActive, bypassStale} {
									c := matrixCase{priorities: prio, states: [3]stepState{sd, sa, sb}, ddaHold: ddaHold, holdA: holdA, skipB: skipB, bypass: bypass}
									in := c.inputs()
									d := Decide(in)
									assertInvariants(t, c.String(), in, d)
									assertModeSelection(t, c, in, d)

									// Determinism, independent of input order.
									rev := in
									rev.Steps = slices.Clone(in.Steps)
									slices.Reverse(rev.Steps)
									require.Equal(t, d, Decide(rev), c.String())
									count++
								}
							}
						}
					}
				}
			}
		}
	}
	t.Logf("checked %d cases", count)
}

func assertModeSelection(t *testing.T, c matrixCase, in Inputs, d Decisions) {
	t.Helper()
	name := c.String()
	switch c.bypass {
	case bypassNew, bypassActive:
		require.Equal(t, ModeParallel, d.Mode, name)
		require.True(t, d.Bypass, name)
		require.Equal(t, c.bypass == bypassNew, d.BypassConsumed, name)
		require.Equal(t, in.Bypass, d.Status.LastBypass, name)
		return
	}
	require.False(t, d.Bypass, name)
	require.False(t, d.BypassConsumed, name)

	// I-13: holds and skips only count on pending steps.
	defaultConfig := true
	effectiveControl := false
	for _, s := range in.Steps {
		if s.Config.Priority != 0 || s.Config.StepTimeout != 0 || s.Config.StepSoak != 0 {
			defaultConfig = false
		}
		if s.Live != nil && s.Live.TargetHash != s.TargetHash && (in.DDAHold || s.Hold || s.Skip) {
			effectiveControl = true
		}
	}
	if defaultConfig && !effectiveControl {
		require.Equal(t, ModeParallel, d.Mode, name)
	} else {
		require.Equal(t, ModeOrdered, d.Mode, name)
	}
}

// Curated scenarios.

type scenarioStep struct {
	kind     v2alpha1.RolloutStepKind
	name     string
	priority int32
	state    stepState
}

func scenario(steps ...scenarioStep) Inputs {
	in := Inputs{Prev: &v2alpha1.RolloutStatus{}, Generation: 2, Now: testNow}
	for i, ss := range steps {
		s, p := buildStep(ss.kind, ss.name, ss.priority, ss.state)
		s.CreationTime = testNow.Add(time.Duration(i) * time.Second)
		in.Steps = append(in.Steps, s)
		if p != nil {
			in.Prev.Steps = append(in.Prev.Steps, *p)
		}
	}
	return in
}

func dflt(prio int32, st stepState) scenarioStep {
	return scenarioStep{v2alpha1.RolloutStepKindDefault, "default", prio, st}
}

func dap(name string, prio int32, st stepState) scenarioStep {
	return scenarioStep{v2alpha1.RolloutStepKindProfile, name, prio, st}
}

func specWrites(d Decisions) []string {
	var out []string
	for _, w := range d.Writes {
		if w.Kind != WritePatchMetadata {
			out = append(out, w.Key)
		}
	}
	slices.Sort(out)
	return out
}

func TestDecide_DefaultSettingsRollInParallel(t *testing.T) {
	in := scenario(dflt(0, stNotStarted), dap("a", 0, stNotStarted), dap("b", 0, stMissing))
	in.Prev = nil
	d := Decide(in)
	assert.Equal(t, ModeParallel, d.Mode)
	assert.Nil(t, d.Status)
	assert.Equal(t, []string{"default", "ns/a", "ns/b"}, specWrites(d))
	assert.Equal(t, WriteCreate, d.Writes[0].Kind)
}

func TestDecide_LowestWaveFirst(t *testing.T) {
	in := scenario(dflt(0, stNotStarted), dap("a", 0, stNotStarted), dap("b", 10, stNotStarted))
	d := Decide(in)
	assert.Equal(t, ModeOrdered, d.Mode)
	assert.Equal(t, []string{"default", "ns/a"}, specWrites(d))
	assert.Equal(t, v2alpha1.RolloutPhasePending, stepStatus(d, "ns/b").Phase)
	assert.Equal(t, ReasonPriorityWavePending, stepStatus(d, "ns/b").Reason)
	assert.Equal(t, v2alpha1.RolloutPhaseInProgress, d.Status.Phase)
	assert.Equal(t, int32(0), *d.Status.CurrentPriority)
	assert.True(t, d.Status.StartedAt.Time.Equal(testNow))
	assert.Equal(t, []string{WarningNoStepTimeout}, d.Status.Warnings)
	// Status order: default first, then DAP creation time.
	assert.Equal(t, "default", d.Steps[0].Key)
	assert.Equal(t, "ns/a", d.Steps[1].Key)
}

func TestDecide_NextWaveAfterCompletion(t *testing.T) {
	in := scenario(dflt(0, stRolling), dap("b", 10, stNotStarted))
	in.Steps[0].Live.DaemonSet = healthyDS()
	d := Decide(in)
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, stepStatus(d, "default").Phase)
	assert.NotNil(t, stepStatus(d, "default").CompletedAt)
	assert.Equal(t, []string{"ns/b"}, specWrites(d))
	assert.Equal(t, int32(10), *d.Status.CurrentPriority)
}

func TestDecide_TimeoutHaltAndContinue(t *testing.T) {
	d := Decide(scenario(dflt(0, stTimedOutHalt), dap("b", 10, stNotStarted)))
	assert.Empty(t, specWrites(d))
	assert.Equal(t, v2alpha1.RolloutPhaseTimedOut, d.Status.Phase)
	assert.Equal(t, v2alpha1.RolloutPhaseTimedOut, stepStatus(d, "default").Phase)
	assert.NotNil(t, stepStatus(d, "default").Deadline)

	d = Decide(scenario(dflt(0, stTimedOutContinue), dap("b", 10, stNotStarted)))
	assert.Equal(t, []string{"ns/b"}, specWrites(d))
	assert.Equal(t, v2alpha1.RolloutPhaseTimedOut, stepStatus(d, "default").Phase)
}

func TestDecide_DAPHoldBlocksLaterWavesNotPeers(t *testing.T) {
	in := scenario(dflt(0, stUnchanged), dap("a", 0, stNotStarted), dap("c", 0, stNotStarted), dap("b", 10, stNotStarted))
	in.Steps[1].Hold = true
	d := Decide(in)
	assert.Equal(t, []string{"ns/c"}, specWrites(d))
	assert.Equal(t, v2alpha1.RolloutPhaseHeld, stepStatus(d, "ns/a").Phase)
	assert.Equal(t, ReasonDAPHold, stepStatus(d, "ns/a").Reason)
	assert.Equal(t, v2alpha1.RolloutPhasePending, stepStatus(d, "ns/b").Phase)

	// A hold on an unchanged step is ignored.
	in = scenario(dflt(0, stUnchanged), dap("a", 0, stUnchanged), dap("b", 10, stNotStarted))
	in.Steps[1].Hold = true
	d = Decide(in)
	assert.Equal(t, []string{"ns/b"}, specWrites(d))
}

func TestDecide_DDAHold(t *testing.T) {
	in := scenario(dflt(0, stRolling), dap("a", 0, stNotStarted), dap("b", 5, stMissing))
	in.DDAHold = true
	d := Decide(in)
	assert.Equal(t, []string{"ns/b"}, specWrites(d), "creation is never gated")
	assert.Equal(t, WriteCreate, writesByKey(d)["ns/b"].Kind)
	assert.Equal(t, v2alpha1.RolloutPhaseInProgress, stepStatus(d, "default").Phase, "started step continues")
	assert.Equal(t, ReasonDDAHold, stepStatus(d, "ns/a").Reason)
}

func TestDecide_SkipLetsLaterWavesProceed(t *testing.T) {
	in := scenario(dflt(0, stUnchanged), dap("a", 0, stNotStarted), dap("b", 10, stNotStarted))
	in.Steps[1].Skip = true
	d := Decide(in)
	assert.Equal(t, []string{"ns/b"}, specWrites(d))
	assert.Equal(t, v2alpha1.RolloutPhaseSkipped, stepStatus(d, "ns/a").Phase)
}

func TestDecide_Bypass(t *testing.T) {
	in := scenario(dflt(0, stNotStarted), dap("a", 0, stNotStarted), dap("b", 10, stNotStarted), dap("c", 20, stNotStarted))
	in.Steps[1].Hold = true
	in.Steps[2].Skip = true
	in.DDAHold = true
	in.Bypass = "now"
	d := Decide(in)
	assert.True(t, d.Bypass)
	assert.True(t, d.BypassConsumed)
	assert.Equal(t, []string{"default", "ns/a", "ns/c"}, specWrites(d), "holds ignored, skip respected")
	assert.Equal(t, v2alpha1.RolloutPhaseBypassed, d.Status.Phase)
	assert.Equal(t, "now", d.Status.LastBypass)
	assert.Equal(t, d.Status.RolloutID, d.Status.BypassRolloutID)

	// Same value again with the same RolloutID continues the bypass (partial write recovery).
	next := in
	next.Prev = d.Status.DeepCopy()
	next.Prev.Steps[0].StartedAt = nil // default write failed
	next.Prev.Steps[0].Phase = v2alpha1.RolloutPhaseBlocked
	d2 := Decide(next)
	assert.True(t, d2.Bypass)
	assert.False(t, d2.BypassConsumed)
	assert.Contains(t, specWrites(d2), "default")

	// Same value with a new RolloutID does not bypass again.
	next.Steps = slices.Clone(next.Steps)
	next.Steps[3].TargetHash = "T-c2"
	d3 := Decide(next)
	assert.False(t, d3.Bypass)
	assert.Empty(t, d3.Status.BypassRolloutID)
	assert.Equal(t, "now", d3.Status.LastBypass)
}

func TestDecide_BypassEndsWhenComplete(t *testing.T) {
	in := scenario(dflt(0, stCompleted), dap("b", 10, stCompleted))
	in.Bypass = "x"
	d := Decide(in)
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, d.Status.Phase)
	assert.Empty(t, d.Status.BypassRolloutID)
	assert.Equal(t, "x", d.Status.LastBypass)
}

func TestDecide_Supersede(t *testing.T) {
	in := scenario(dflt(0, stUnchanged), dap("a", 0, stRolling), dap("b", 10, stNotStarted))
	// New target for a: the previous in-progress state no longer applies.
	in.Steps[1].TargetHash = "T-a2"
	d := Decide(in)
	st := stepStatus(d, "ns/a")
	assert.Equal(t, []string{"ns/a"}, specWrites(d))
	assert.True(t, st.StartedAt.Time.Equal(testNow), "startedAt restarts for the new target")
	assert.Equal(t, ReasonSpecApplying, st.Reason)
}

func TestDecide_ConfigEditKeepsStartedAt(t *testing.T) {
	in := scenario(dflt(0, stRolling), dap("b", 10, stNotStarted))
	in.Steps[0].Config.StepTimeout = 3 * time.Hour
	d := Decide(in)
	assert.Equal(t, in.Prev.Steps[0].StartedAt, stepStatus(d, "default").StartedAt)
}

func TestDecide_CreationNeverGated(t *testing.T) {
	d := Decide(scenario(dflt(0, stNotStarted), dap("new", 10, stMissing)))
	assert.Equal(t, []string{"default", "ns/new"}, specWrites(d))
	assert.Equal(t, WriteCreate, d.Writes[0].Kind)
	assert.Equal(t, ReasonSpecApplying, stepStatus(d, "ns/new").Reason)
}

func TestDecide_Warnings(t *testing.T) {
	// Single pending wave: no NoStepTimeout.
	d := Decide(scenario(dflt(0, stNotStarted), dap("b", 10, stUnchanged)))
	assert.Empty(t, d.Status.Warnings)

	// Two pending waves with a timeout on the blocking step: no NoStepTimeout.
	in := scenario(dflt(0, stNotStarted), dap("b", 10, stNotStarted))
	in.Steps[0].Config.StepTimeout = time.Hour
	assert.Empty(t, Decide(in).Status.Warnings)

	// Default after a pending profile.
	d = Decide(scenario(dflt(10, stNotStarted), dap("a", 0, stNotStarted)))
	assert.ElementsMatch(t, []string{WarningNoStepTimeout, WarningDefaultNotFirst}, d.Status.Warnings)
}

func TestDecide_ProgressPredicates(t *testing.T) {
	d := Decide(scenario(dflt(0, stWrittenUnacked), dap("b", 10, stNotStarted)))
	assert.Equal(t, ReasonSpecApplying, stepStatus(d, "default").Reason)
	assert.Empty(t, specWrites(d))

	d = Decide(scenario(dflt(0, stReconcileError), dap("b", 10, stNotStarted)))
	assert.Equal(t, ReasonSpecApplying, stepStatus(d, "default").Reason)

	d = Decide(scenario(dflt(0, stBaking), dap("b", 10, stNotStarted)))
	assert.Equal(t, v2alpha1.RolloutPhaseBaking, stepStatus(d, "default").Phase)
	assert.Equal(t, v2alpha1.RolloutPhaseBaking, d.Status.Phase)

	// Soak elapsed.
	in := scenario(dflt(0, stBaking), dap("b", 10, stNotStarted))
	in.Prev.Steps[0].SoakStartedAt = ago(time.Hour)
	d = Decide(in)
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, stepStatus(d, "default").Phase)
	assert.Equal(t, []string{"ns/b"}, specWrites(d))

	// Zero-node DaemonSet completes once observed.
	in = scenario(dflt(0, stRolling), dap("b", 10, stNotStarted))
	in.Steps[0].Live.DaemonSet = &DaemonSetFacts{Generation: 3, ObservedGeneration: 3}
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, stepStatus(Decide(in), "default").Phase)

	// No Agent DaemonSet: complete once applied.
	in = scenario(dflt(0, stRolling), dap("b", 10, stNotStarted))
	in.Steps[0].Live.HasAgent, in.Steps[0].Live.DaemonSet = false, nil
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, stepStatus(Decide(in), "default").Phase)

	// Agent DaemonSet expected but missing.
	in = scenario(dflt(0, stRolling), dap("b", 10, stNotStarted))
	in.Steps[0].Live.DaemonSet = nil
	assert.Equal(t, v2alpha1.RolloutPhaseInProgress, stepStatus(Decide(in), "default").Phase)
}

func TestDecide_RegressedLatchedStepDoesNotBlock(t *testing.T) {
	d := Decide(scenario(dflt(0, stRegressed), dap("b", 10, stNotStarted)))
	assert.Equal(t, v2alpha1.RolloutPhaseCompleted, stepStatus(d, "default").Phase)
	assert.Equal(t, int32(3), stepStatus(d, "default").Progress.NumberUnavailable)
	assert.Equal(t, []string{"ns/b"}, specWrites(d))
}

func TestDecide_AnnotationSync(t *testing.T) {
	in := scenario(dflt(0, stUnchanged), dap("a", 0, stNotStarted), dap("b", 10, stNotStarted))
	in.Steps[0].Live.AnnotationsDiffer = true
	in.Steps[2].Live.InertAnnotationsDiffer = true
	d := Decide(in)
	w := writesByKey(d)
	assert.Equal(t, Write{Key: "default", Kind: WritePatchMetadata, SyncHashes: true}, w["default"])
	assert.Equal(t, WriteUpdate, w["ns/a"].Kind)
	assert.Equal(t, Write{Key: "ns/b", Kind: WritePatchMetadata}, w["ns/b"], "gated step gets a metadata-only patch")
	assert.Equal(t, v2alpha1.RolloutPhasePending, stepStatus(d, "ns/b").Phase)
}

func TestDecide_TransitionTimesAreStable(t *testing.T) {
	in := scenario(dflt(0, stNotStarted), dap("b", 10, stNotStarted))
	d := Decide(in)
	next := in
	next.Prev = d.Status.DeepCopy()
	next.Now = testNow.Add(15 * time.Second)
	next.Steps = slices.Clone(in.Steps)
	live := *in.Steps[0].Live
	live.TargetHash, live.Generation, live.ObservedGeneration = in.Steps[0].TargetHash, 2, 1
	next.Steps[0].Live = &live
	d2 := Decide(next)
	assert.Equal(t, d.Status.LastTransitionTime, d2.Status.LastTransitionTime)
	assert.Equal(t, stepStatus(d, "ns/b").LastTransitionTime, stepStatus(d2, "ns/b").LastTransitionTime)
	assert.Equal(t, d.Status.StartedAt, d2.Status.StartedAt, "restart keeps startedAt")
	assert.Equal(t, stepStatus(d, "default").StartedAt, stepStatus(d2, "default").StartedAt)
}

func TestDecide_FastPathKeepsConsumedBypass(t *testing.T) {
	in := scenario(dflt(0, stUnchanged), dap("a", 0, stUnchanged))
	in.Bypass = "x"
	in.Prev.LastBypass = "x"
	d := Decide(in)
	assert.Equal(t, ModeParallel, d.Mode)
	require.NotNil(t, d.Status)
	assert.Equal(t, v2alpha1.RolloutPhaseIdle, d.Status.Phase)
	assert.Equal(t, "x", d.Status.LastBypass)
	assert.False(t, d.Active())
}
