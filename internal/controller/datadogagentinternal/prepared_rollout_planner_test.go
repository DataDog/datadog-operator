// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"reflect"
	"testing"
)

func TestPreparedRolloutPlannerUsesUnavailableBudget(t *testing.T) {
	states := []preparedNodePlanState{
		{name: "node-c", phase: preparedNodeSource, sourceServing: true},
		{name: "node-a", phase: preparedNodeTarget, targetReady: false},
		{name: "node-b", phase: preparedNodeSource, sourceServing: true},
	}

	plan := planPreparedRollout(states, 2, preparedRolloutMutationLimit)
	want := []preparedNodeAction{{node: "node-b", kind: preparedActionStartOverlap}}
	if !reflect.DeepEqual(plan.actions, want) {
		t.Fatalf("unexpected actions: got %#v, want %#v", plan.actions, want)
	}
	if plan.unavailable != 1 {
		t.Fatalf("unexpected unavailable count: got %d, want 1", plan.unavailable)
	}
}

func TestPreparedRolloutPlannerStopsBehindInFlightNode(t *testing.T) {
	states := []preparedNodePlanState{
		{name: "node-a", phase: preparedNodeTransition, sourceServing: true},
		{name: "node-b", phase: preparedNodeSource, sourceServing: true},
	}

	plan := planPreparedRollout(states, 1, preparedRolloutMutationLimit)
	if len(plan.actions) != 0 {
		t.Fatalf("unexpected actions while a prior batch is in flight: %#v", plan.actions)
	}
	if plan.inFlight != 1 {
		t.Fatalf("unexpected in-flight count: got %d, want 1", plan.inFlight)
	}
}

func TestPreparedRolloutPlannerReobservesCandidateBeforeHandoff(t *testing.T) {
	state := preparedNodePlanState{
		name:           "node-a",
		phase:          preparedNodeTransition,
		sourceServing:  true,
		targetPrepared: true,
		targetUID:      "target-uid",
		targetMatches:  true,
	}

	plan := planPreparedRollout([]preparedNodePlanState{state}, 1, preparedRolloutMutationLimit)
	want := []preparedNodeAction{{node: "node-a", kind: preparedActionRecordCandidate}}
	if !reflect.DeepEqual(plan.actions, want) {
		t.Fatalf("unexpected first observation: got %#v, want %#v", plan.actions, want)
	}

	state.recordedTargetUID = state.targetUID
	plan = planPreparedRollout([]preparedNodePlanState{state}, 1, preparedRolloutMutationLimit)
	want = []preparedNodeAction{{node: "node-a", kind: preparedActionSelectTarget}}
	if !reflect.DeepEqual(plan.actions, want) {
		t.Fatalf("unexpected second observation: got %#v, want %#v", plan.actions, want)
	}
}

func TestPreparedRolloutPlannerLimitsAPIMutations(t *testing.T) {
	states := []preparedNodePlanState{
		{name: "node-e", phase: preparedNodeSource, sourceServing: true},
		{name: "node-d", phase: preparedNodeSource, sourceServing: true},
		{name: "node-c", phase: preparedNodeSource, sourceServing: true},
		{name: "node-b", phase: preparedNodeSource, sourceServing: true},
		{name: "node-a", phase: preparedNodeSource, sourceServing: true},
	}

	plan := planPreparedRollout(states, len(states), 2)
	want := []preparedNodeAction{
		{node: "node-a", kind: preparedActionStartOverlap},
		{node: "node-b", kind: preparedActionStartOverlap},
	}
	if !reflect.DeepEqual(plan.actions, want) {
		t.Fatalf("unexpected bounded actions: got %#v, want %#v", plan.actions, want)
	}
}

func TestPreparedRolloutPlannerRepairsUnavailableSourceWithoutSpendingBudget(t *testing.T) {
	states := []preparedNodePlanState{
		{name: "node-a", phase: preparedNodeSource, sourceServing: false},
		{name: "node-b", phase: preparedNodeSource, sourceServing: true},
	}

	plan := planPreparedRollout(states, 1, preparedRolloutMutationLimit)
	want := []preparedNodeAction{{node: "node-a", kind: preparedActionStartOverlap}}
	if !reflect.DeepEqual(plan.actions, want) {
		t.Fatalf("unexpected recovery actions: got %#v, want %#v", plan.actions, want)
	}
	if plan.unavailable != 1 {
		t.Fatalf("unexpected unavailable count: got %d, want 1", plan.unavailable)
	}
}
