// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orderedrollout

import (
	"cmp"
	"fmt"
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// Mode is the gate policy selected for a reconcile.
type Mode string

const (
	// ModeParallel allows every changed step: default-compatible settings or a manual bypass.
	ModeParallel Mode = "Parallel"
	// ModeOrdered allows only the active priority wave.
	ModeOrdered Mode = "Ordered"
)

// Warning reasons.
const (
	WarningNoStepTimeout   = "NoStepTimeout"
	WarningDefaultNotFirst = "DefaultNotFirst"
)

// WriteKind is the kind of DatadogAgentInternal write requested by Decide.
type WriteKind string

const (
	// WriteCreate creates a missing DatadogAgentInternal.
	WriteCreate WriteKind = "Create"
	// WriteUpdate writes the rendered DatadogAgentInternal.
	WriteUpdate WriteKind = "Update"
	// WritePatchMetadata patches only inert annotations on the live DatadogAgentInternal.
	WritePatchMetadata WriteKind = "PatchMetadata"
)

// Write is a DatadogAgentInternal write for one step.
type Write struct {
	Key  string
	Kind WriteKind
	// SyncHashes also syncs the hash annotations in a metadata patch. Only set
	// for steps whose live object is at the target.
	SyncHashes bool
}

// LiveFacts describe the live DatadogAgentInternal of a step and its Agent DaemonSet.
type LiveFacts struct {
	// TargetHash is the rollout target hash of the live object.
	TargetHash         string
	Generation         int64
	ObservedGeneration int64
	ReconcileError     bool
	// HasAgent reports whether the DatadogAgentInternal status references an Agent DaemonSet.
	HasAgent  bool
	DaemonSet *DaemonSetFacts
	// AnnotationsDiffer reports whether any annotation differs from the rendered object.
	AnnotationsDiffer bool
	// InertAnnotationsDiffer reports whether InertAnnotationPatch is not empty.
	InertAnnotationsDiffer bool
}

// StepFacts describe one rollout step.
type StepFacts struct {
	Kind         v2alpha1.RolloutStepKind
	Name         string
	Namespace    string
	CreationTime time.Time
	Source       string
	TargetHash   string
	Config       Config
	Hold         bool
	Skip         bool
	// Live is nil when the DatadogAgentInternal does not exist.
	Live *LiveFacts
}

// Key returns the step key.
func (s StepFacts) Key() string {
	return StepKey(s.Kind, s.Namespace, s.Name)
}

// StepKey returns the key identifying a step.
func StepKey(kind v2alpha1.RolloutStepKind, namespace, name string) string {
	if kind == v2alpha1.RolloutStepKindDefault {
		return "default"
	}
	return namespace + "/" + name
}

// Inputs is the snapshot evaluated by Decide.
type Inputs struct {
	Steps      []StepFacts
	DDAHold    bool
	Bypass     string
	Prev       *v2alpha1.RolloutStatus
	Generation int64
	Now        time.Time
}

// StepDecision is the decision for one step.
type StepDecision struct {
	Key     string
	Allowed bool
	Status  v2alpha1.RolloutStepStatus
}

// Decisions is the result of Decide.
type Decisions struct {
	Mode Mode
	// Bypass reports whether a manual bypass selected Parallel mode.
	Bypass bool
	// BypassConsumed reports whether a new bypass annotation value was consumed.
	BypassConsumed bool
	// Steps are in rollout order.
	Steps []StepDecision
	// Writes are ordered: creations, then updates, then metadata patches.
	Writes []Write
	// Status is nil when no rollout status is relevant.
	Status *v2alpha1.RolloutStatus
}

// Active reports whether an ordered rollout still needs reconciles to progress.
func (d Decisions) Active() bool {
	return d.Status != nil && d.Status.Phase != v2alpha1.RolloutPhaseCompleted && d.Status.Phase != v2alpha1.RolloutPhaseIdle
}

// SortSteps sorts steps in rollout order: priority, default first, DAP
// creation time, then namespace/name.
func SortSteps(steps []StepFacts) {
	slices.SortStableFunc(steps, func(a, b StepFacts) int {
		if c := cmp.Compare(a.Config.Priority, b.Config.Priority); c != 0 {
			return c
		}
		aDefault, bDefault := a.Kind == v2alpha1.RolloutStepKindDefault, b.Kind == v2alpha1.RolloutStepKindDefault
		if aDefault != bDefault {
			if aDefault {
				return -1
			}
			return 1
		}
		if c := a.CreationTime.Compare(b.CreationTime); c != 0 {
			return c
		}
		return cmp.Compare(a.Key(), b.Key())
	})
}

func pending(s StepFacts) bool {
	return s.Live != nil && s.Live.TargetHash != s.TargetHash
}

func orderingComplete(st *v2alpha1.RolloutStepStatus, cfg Config) bool {
	switch st.Phase {
	case v2alpha1.RolloutPhaseCompleted, v2alpha1.RolloutPhaseSkipped:
		return true
	case v2alpha1.RolloutPhaseTimedOut:
		return cfg.OnStepTimeout != v2alpha1.RolloutTimeoutActionHalt
	}
	return false
}

func isFastPath(in Inputs, steps []StepFacts) bool {
	for _, s := range steps {
		if s.Config.Priority != 0 || s.Config.StepTimeout != 0 || s.Config.StepSoak != 0 {
			return false
		}
		if pending(s) && (in.DDAHold || s.Hold || s.Skip) {
			return false
		}
	}
	return true
}

// Decide computes the gate decisions from a snapshot. It is pure.
func Decide(in Inputs) Decisions {
	steps := slices.Clone(in.Steps)
	SortSteps(steps)
	now := metav1.NewTime(in.Now)

	targets := make(map[string]string, len(steps))
	for _, s := range steps {
		targets[s.Key()] = s.TargetHash
	}
	rolloutID := RolloutID(targets)

	var lastBypass, bypassID string
	prevSteps := map[string]*v2alpha1.RolloutStepStatus{}
	if in.Prev != nil {
		lastBypass = in.Prev.LastBypass
		if in.Prev.BypassRolloutID == rolloutID {
			bypassID = rolloutID
		}
		for i := range in.Prev.Steps {
			p := &in.Prev.Steps[i]
			prevSteps[StepKey(p.Kind, p.Namespace, p.Name)] = p
		}
	}
	d := Decisions{Mode: ModeOrdered}
	if in.Bypass != "" && in.Bypass != lastBypass {
		lastBypass = in.Bypass
		bypassID = rolloutID
		d.BypassConsumed = true
	}
	d.Bypass = bypassID != ""

	if !d.Bypass && isFastPath(in, steps) {
		return fastPath(in, steps, d)
	}
	if d.Bypass {
		d.Mode = ModeParallel
	}

	// Evaluate every step's lifecycle; gated marks pending steps not yet started.
	statuses := make([]v2alpha1.RolloutStepStatus, len(steps))
	writes := make([]WriteKind, len(steps))
	gated := make([]bool, len(steps))
	for i, s := range steps {
		var p *v2alpha1.RolloutStepStatus
		if prev := prevSteps[s.Key()]; prev != nil && prev.TargetHash == s.TargetHash {
			p = prev
		}
		statuses[i], writes[i], gated[i] = evaluateStep(s, p, now)
		// A DDA hold takes precedence over a skip for steps not started yet.
		skippable := gated[i] && (d.Bypass || !in.DDAHold) ||
			!gated[i] && writes[i] == "" && statuses[i].Phase != v2alpha1.RolloutPhaseCompleted
		if s.Skip && skippable {
			setPhase(&statuses[i], v2alpha1.RolloutPhaseSkipped, ReasonDAPSkip, "Skipped by the rollout-skip annotation; the live spec is kept")
			gated[i] = false
		}
	}

	activeWave, hasActive := lowestIncompleteWave(steps, statuses, gated)
	for i, s := range steps {
		if !gated[i] {
			continue
		}
		st := &statuses[i]
		switch {
		case d.Bypass || (s.Config.Priority == activeWave && !in.DDAHold && !s.Hold):
			writes[i] = WriteUpdate
			st.StartedAt = &now
			setPhase(st, v2alpha1.RolloutPhaseInProgress, ReasonSpecApplying, "Spec written, waiting for the DatadogAgentInternal controller")
		case in.DDAHold:
			setPhase(st, v2alpha1.RolloutPhaseHeld, ReasonDDAHold, "Held by the rollout-hold annotation on the DatadogAgent")
		case s.Hold:
			setPhase(st, v2alpha1.RolloutPhaseHeld, ReasonDAPHold, "Held by the rollout-hold annotation on the DatadogAgentProfile")
		default:
			setPhase(st, v2alpha1.RolloutPhasePending, ReasonPriorityWavePending, fmt.Sprintf("Waiting for priority %d to complete", activeWave))
		}
	}

	// Annotation sync for steps without a spec write never touches the spec.
	syncHashes := make([]bool, len(steps))
	for i, s := range steps {
		if writes[i] != "" || s.Live == nil {
			continue
		}
		switch {
		case !pending(s) && s.Live.AnnotationsDiffer:
			writes[i], syncHashes[i] = WritePatchMetadata, true
		case pending(s) && s.Live.InertAnnotationsDiffer:
			writes[i] = WritePatchMetadata
		}
	}

	for i, s := range steps {
		if p := prevSteps[s.Key()]; p != nil && p.Phase == statuses[i].Phase && p.Reason == statuses[i].Reason && p.LastTransitionTime != nil {
			statuses[i].LastTransitionTime = p.LastTransitionTime
		} else {
			statuses[i].LastTransitionTime = &now
		}
		d.Steps = append(d.Steps, StepDecision{
			Key:     s.Key(),
			Allowed: writes[i] == WriteCreate || (writes[i] == WriteUpdate && pending(s)),
			Status:  statuses[i],
		})
	}
	d.Writes = orderWrites(steps, writes, syncHashes)

	status := &v2alpha1.RolloutStatus{
		Generation: in.Generation,
		RolloutID:  rolloutID,
		LastBypass: lastBypass,
		Steps:      statuses,
	}
	if hasActive {
		status.CurrentPriority = &activeWave
	}
	if d.Mode == ModeOrdered {
		status.Warnings = warnings(steps, statuses)
	}
	aggregate(status, in, d.Bypass, steps)
	if d.Bypass && status.Phase != v2alpha1.RolloutPhaseCompleted {
		status.BypassRolloutID = bypassID
	}
	status.StartedAt = rolloutStartedAt(in.Prev, rolloutID, statuses, now)
	if in.Prev != nil && in.Prev.Phase == status.Phase && in.Prev.Reason == status.Reason && in.Prev.LastTransitionTime != nil {
		status.LastTransitionTime = in.Prev.LastTransitionTime
	} else {
		status.LastTransitionTime = &now
	}
	d.Status = status
	return d
}

func fastPath(in Inputs, steps []StepFacts, d Decisions) Decisions {
	d.Mode = ModeParallel
	for _, s := range steps {
		kind := WriteUpdate
		if s.Live == nil {
			kind = WriteCreate
		}
		d.Writes = append(d.Writes, Write{Key: s.Key(), Kind: kind})
		d.Steps = append(d.Steps, StepDecision{Key: s.Key(), Allowed: true})
	}
	slices.SortStableFunc(d.Writes, func(a, b Write) int { return cmp.Compare(writeRank(a.Kind), writeRank(b.Kind)) })
	// Keep the consumed bypass value so a stale annotation does not bypass a later ordered rollout.
	if in.Bypass != "" {
		d.Status = &v2alpha1.RolloutStatus{Phase: v2alpha1.RolloutPhaseIdle, Generation: in.Generation, LastBypass: in.Bypass}
	}
	return d
}

// evaluateStep computes a step's lifecycle status. gated is true for pending
// steps that were not started and await the gate decision.
func evaluateStep(s StepFacts, p *v2alpha1.RolloutStepStatus, now metav1.Time) (st v2alpha1.RolloutStepStatus, write WriteKind, gated bool) {
	st = v2alpha1.RolloutStepStatus{
		Name:       s.Name,
		Namespace:  s.Namespace,
		Kind:       s.Kind,
		Priority:   s.Config.Priority,
		TargetHash: s.TargetHash,
		Source:     s.Source,
	}
	if s.Live != nil {
		st.CurrentHash = s.Live.TargetHash
	}
	if p != nil {
		st.StartedAt = p.StartedAt
		st.CompletedAt = p.CompletedAt
		st.SoakStartedAt = p.SoakStartedAt
	}

	switch {
	case s.Live == nil:
		// Creation is never gated.
		if st.StartedAt == nil {
			st.StartedAt = &now
		}
		st.CompletedAt, st.SoakStartedAt = nil, nil
		setPhase(&st, v2alpha1.RolloutPhaseInProgress, ReasonSpecApplying, "Creating the DatadogAgentInternal")
		return st, WriteCreate, false
	case pending(s):
		// Not started for this target, or the written spec is no longer live.
		st.StartedAt, st.CompletedAt, st.SoakStartedAt = nil, nil, nil
		return st, "", true
	case p != nil && p.Phase == v2alpha1.RolloutPhaseCompleted && p.Reason == ReasonComplete:
		// Completion is latched for this target; live health is reported only.
		setPhase(&st, v2alpha1.RolloutPhaseCompleted, ReasonComplete, "Rollout complete")
		if s.Live.DaemonSet != nil {
			st.Progress = EvaluateDaemonSet(s.Live.DaemonSet, s.Config, nil, now.Time).Progress
		}
		return st, "", false
	case st.StartedAt == nil:
		setPhase(&st, v2alpha1.RolloutPhaseCompleted, ReasonUnchanged, "Already at the target")
		st.CompletedAt, st.SoakStartedAt = nil, nil
		return st, "", false
	}

	evaluateProgress(&st, s, now)
	return st, "", false
}

// evaluateProgress evaluates a started step whose live object is at the target.
func evaluateProgress(st *v2alpha1.RolloutStepStatus, s StepFacts, now metav1.Time) {
	live := s.Live
	complete := false
	switch {
	case live.ReconcileError:
		setPhase(st, v2alpha1.RolloutPhaseInProgress, ReasonSpecApplying, "The DatadogAgentInternal reports a reconcile error")
	case live.ObservedGeneration < live.Generation:
		setPhase(st, v2alpha1.RolloutPhaseInProgress, ReasonSpecApplying, "Waiting for the DatadogAgentInternal controller to apply the spec")
	case !live.HasAgent && live.DaemonSet == nil:
		complete = true
		setPhase(st, v2alpha1.RolloutPhaseCompleted, ReasonComplete, "Applied; no Agent DaemonSet to roll")
	default:
		c := EvaluateDaemonSet(live.DaemonSet, s.Config, st.SoakStartedAt, now.Time)
		st.Progress = c.Progress
		st.SoakStartedAt = c.SoakStartedAt
		switch {
		case c.Complete:
			complete = true
			setPhase(st, v2alpha1.RolloutPhaseCompleted, ReasonComplete, c.Message)
		case c.Phase == v2alpha1.RolloutPhaseBaking:
			setPhase(st, v2alpha1.RolloutPhaseBaking, ReasonSoaking, c.Message)
		default:
			setPhase(st, v2alpha1.RolloutPhaseInProgress, ReasonRolling, fmt.Sprintf("%s: %s", c.Reason, c.Message))
		}
	}

	if complete {
		if st.CompletedAt == nil {
			st.CompletedAt = &now
		}
		return
	}
	st.CompletedAt = nil
	if s.Config.StepTimeout > 0 {
		deadline := metav1.NewTime(st.StartedAt.Add(s.Config.StepTimeout))
		st.Deadline = &deadline
		if now.After(deadline.Time) {
			setPhase(st, v2alpha1.RolloutPhaseTimedOut, ReasonStepTimedOut, fmt.Sprintf("Not complete after %s (onStepTimeout=%s)", s.Config.StepTimeout, s.Config.OnStepTimeout))
		}
	}
}

func setPhase(st *v2alpha1.RolloutStepStatus, phase v2alpha1.RolloutPhase, reason, message string) {
	st.Phase, st.Reason, st.Message = phase, reason, message
}

func lowestIncompleteWave(steps []StepFacts, statuses []v2alpha1.RolloutStepStatus, gated []bool) (int32, bool) {
	var wave int32
	found := false
	for i, s := range steps {
		if gated[i] || !orderingComplete(&statuses[i], s.Config) {
			if !found || s.Config.Priority < wave {
				wave, found = s.Config.Priority, true
			}
		}
	}
	return wave, found
}

func warnings(steps []StepFacts, statuses []v2alpha1.RolloutStepStatus) []string {
	waves := map[int32]bool{}
	defaultPriority, hasDefault := int32(0), false
	for i, s := range steps {
		if s.Kind == v2alpha1.RolloutStepKindDefault {
			defaultPriority, hasDefault = s.Config.Priority, true
		}
		if !orderingComplete(&statuses[i], s.Config) {
			waves[s.Config.Priority] = true
		}
	}
	var out []string
	activeWave, ok := lowestIncompleteWave(steps, statuses, make([]bool, len(steps)))
	if ok && len(waves) > 1 {
		for i, s := range steps {
			if s.Config.Priority == activeWave && !orderingComplete(&statuses[i], s.Config) && s.Config.StepTimeout == 0 {
				out = append(out, WarningNoStepTimeout)
				break
			}
		}
	}
	if hasDefault {
		for i, s := range steps {
			if s.Kind == v2alpha1.RolloutStepKindProfile && s.Config.Priority < defaultPriority && !orderingComplete(&statuses[i], s.Config) {
				out = append(out, WarningDefaultNotFirst)
				break
			}
		}
	}
	return out
}

func aggregate(status *v2alpha1.RolloutStatus, in Inputs, bypass bool, steps []StepFacts) {
	allComplete := true
	var timedOut, inProgress, baking, held string
	for i, s := range steps {
		st := &status.Steps[i]
		if !orderingComplete(st, s.Config) {
			allComplete = false
		}
		switch st.Phase {
		case v2alpha1.RolloutPhaseTimedOut:
			if s.Config.OnStepTimeout == v2alpha1.RolloutTimeoutActionHalt && timedOut == "" {
				timedOut = st.Name
			}
		case v2alpha1.RolloutPhaseInProgress:
			inProgress = st.Name
		case v2alpha1.RolloutPhaseBaking:
			baking = st.Name
		case v2alpha1.RolloutPhaseHeld:
			held = st.Name
		}
	}
	set := func(phase v2alpha1.RolloutPhase, reason, message string) {
		status.Phase, status.Reason, status.Message = phase, reason, message
	}
	switch {
	case allComplete:
		set(v2alpha1.RolloutPhaseCompleted, ReasonComplete, "All rollout steps are complete")
	case bypass:
		set(v2alpha1.RolloutPhaseBypassed, ReasonManualBypass, fmt.Sprintf("Manual bypass %q: rolling all changed steps in parallel", status.LastBypass))
	case timedOut != "":
		set(v2alpha1.RolloutPhaseTimedOut, ReasonStepTimedOut, fmt.Sprintf("Step %s timed out; later priorities are halted", timedOut))
	case inProgress != "":
		set(v2alpha1.RolloutPhaseInProgress, ReasonRolling, fmt.Sprintf("Rolling priority %d", *status.CurrentPriority))
	case baking != "":
		set(v2alpha1.RolloutPhaseBaking, ReasonSoaking, fmt.Sprintf("Soaking priority %d", *status.CurrentPriority))
	case held != "" && in.DDAHold:
		set(v2alpha1.RolloutPhaseHeld, ReasonDDAHold, "Held by the rollout-hold annotation on the DatadogAgent")
	case held != "":
		set(v2alpha1.RolloutPhaseHeld, ReasonDAPHold, fmt.Sprintf("Step %s is held by the rollout-hold annotation", held))
	default:
		set(v2alpha1.RolloutPhasePending, ReasonPriorityWavePending, fmt.Sprintf("Waiting to start priority %d", *status.CurrentPriority))
	}
}

func rolloutStartedAt(prev *v2alpha1.RolloutStatus, rolloutID string, statuses []v2alpha1.RolloutStepStatus, now metav1.Time) *metav1.Time {
	if prev != nil && prev.RolloutID == rolloutID && prev.StartedAt != nil {
		return prev.StartedAt
	}
	for _, st := range statuses {
		if st.StartedAt != nil && st.StartedAt.Equal(&now) {
			return &now
		}
	}
	return nil
}

func writeRank(k WriteKind) int {
	switch k {
	case WriteCreate:
		return 0
	case WriteUpdate:
		return 1
	}
	return 2
}

func orderWrites(steps []StepFacts, kinds []WriteKind, syncHashes []bool) []Write {
	var out []Write
	for i, s := range steps {
		if kinds[i] != "" {
			out = append(out, Write{Key: s.Key(), Kind: kinds[i], SyncHashes: syncHashes[i]})
		}
	}
	slices.SortStableFunc(out, func(a, b Write) int { return cmp.Compare(writeRank(a.Kind), writeRank(b.Kind)) })
	return out
}
