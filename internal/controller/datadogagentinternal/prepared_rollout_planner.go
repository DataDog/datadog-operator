// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import "sort"

// Kubernetes limits the number of DaemonSet mutations in one synchronization
// independently from maxUnavailable. Keep the same limit here so a percentage
// budget cannot produce an unbounded API write burst on a large cluster.
const preparedRolloutMutationLimit = 250

type preparedNodePhase uint8

const (
	preparedNodeSource preparedNodePhase = iota
	preparedNodeTransition
	preparedNodeTarget
)

type preparedNodePlanState struct {
	name string

	phase             preparedNodePhase
	sourceServing     bool
	sourcePrepared    bool
	targetServing     bool
	targetReady       bool
	targetPrepared    bool
	targetRestarted   bool
	targetMatches     bool
	targetCapacity    bool
	targetExists      bool
	targetUID         string
	recordedTargetUID string
}

type preparedNodeActionKind uint8

const (
	preparedActionRecordCandidate preparedNodeActionKind = iota
	preparedActionSelectTarget
	preparedActionRetryTarget
	preparedActionStartOverlap
)

type preparedNodeAction struct {
	node string
	kind preparedNodeActionKind
}

type preparedRolloutPlan struct {
	actions     []preparedNodeAction
	unavailable int
	inFlight    int
}

// planPreparedRollout applies the same maxUnavailable model as the Kubernetes
// DaemonSet controller. It counts unavailable nodes before it spends budget,
// permits recovery that adds no unavailability, and then admits a bounded set
// of healthy nodes. Datadog-specific Prepared and two-slot states are inputs to
// this planner; Kubernetes API reads and writes remain outside it.
func planPreparedRollout(states []preparedNodePlanState, maxUnavailable, mutationLimit int) preparedRolloutPlan {
	ordered := append([]preparedNodePlanState(nil), states...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].name < ordered[j].name })

	plan := preparedRolloutPlan{}
	for i := range ordered {
		state := &ordered[i]
		switch state.phase {
		case preparedNodeTransition:
			plan.inFlight++
			if !state.sourceServing {
				plan.unavailable++
			}
		case preparedNodeSource:
			if !state.sourceServing {
				plan.unavailable++
			}
		case preparedNodeTarget:
			if !state.targetReady {
				plan.unavailable++
			}
		}
	}

	remaining := max(0, maxUnavailable-plan.unavailable)
	appendAction := func(state *preparedNodePlanState, kind preparedNodeActionKind) bool {
		if mutationLimit <= 0 || len(plan.actions) >= mutationLimit {
			return false
		}
		plan.actions = append(plan.actions, preparedNodeAction{node: state.name, kind: kind})
		return true
	}

	// Finish or repair nodes that already entered the overlap state before a
	// new overlap batch starts.
	for i := range ordered {
		state := &ordered[i]
		if state.phase != preparedNodeTransition {
			continue
		}

		targetRecovered := !state.sourceServing && state.targetReady
		switch {
		case state.targetPrepared || targetRecovered:
			if state.targetUID == "" {
				continue
			}
			if state.recordedTargetUID != state.targetUID {
				appendAction(state, preparedActionRecordCandidate)
				continue
			}
			handoffCost := 0
			if state.sourceServing {
				handoffCost = 1
			}
			if handoffCost <= remaining && appendAction(state, preparedActionSelectTarget) {
				remaining -= handoffCost
			}
		case state.sourceServing && state.targetMatches && state.targetRestarted:
			appendAction(state, preparedActionRetryTarget)
		case state.targetMatches && state.targetCapacity:
			handoffCost := 0
			if state.sourceServing {
				handoffCost = 1
			}
			if handoffCost <= remaining && appendAction(state, preparedActionSelectTarget) {
				remaining -= handoffCost
			}
		case state.targetExists && !state.targetMatches &&
			(state.sourcePrepared || (!state.sourceServing && !state.targetServing)):
			appendAction(state, preparedActionRetryTarget)
		}
	}

	// An unavailable source has already consumed budget. Starting its target
	// cannot make availability worse, so it is a recovery action.
	for i := range ordered {
		state := &ordered[i]
		if state.phase == preparedNodeSource && !state.sourceServing {
			appendAction(state, preparedActionStartOverlap)
		}
	}

	if len(plan.actions) > 0 || plan.inFlight > 0 {
		return plan
	}

	// No prior batch remains. Admit healthy source nodes up to both the
	// availability budget and the independent API mutation limit.
	for i := range ordered {
		if remaining == 0 {
			break
		}
		state := &ordered[i]
		if state.phase != preparedNodeSource || !state.sourceServing {
			continue
		}
		if !appendAction(state, preparedActionStartOverlap) {
			break
		}
		remaining--
	}
	return plan
}
