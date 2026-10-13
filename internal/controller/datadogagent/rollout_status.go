// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/orderedrollout"
)

// setRolloutConditions maintains the rollout conditions while rollout status
// is present. RolloutProgressing and RolloutComplete are always written;
// RolloutHeld and RolloutTimedOut are written True or False.
func setRolloutConditions(status *v2alpha1.DatadogAgentStatus, now metav1.Time) {
	ro := status.Rollout
	if ro == nil || ro.Phase == v2alpha1.RolloutPhaseIdle {
		return
	}
	set := func(t string, ok bool, reason, message string) {
		s := metav1.ConditionFalse
		if ok {
			s = metav1.ConditionTrue
		}
		condition.UpdateDatadogAgentStatusConditions(status, now, t, s, reason, message, true)
	}

	switch ro.Phase {
	case v2alpha1.RolloutPhasePending, v2alpha1.RolloutPhaseInProgress, v2alpha1.RolloutPhaseBaking, v2alpha1.RolloutPhaseBypassed:
		set(common.RolloutProgressingConditionType, true, ro.Reason, ro.Message)
	default:
		set(common.RolloutProgressingConditionType, false, ro.Reason, ro.Message)
	}
	set(common.RolloutCompleteConditionType, ro.Phase == v2alpha1.RolloutPhaseCompleted, ro.Reason, ro.Message)

	held, timedOut := stepsInPhase(ro, v2alpha1.RolloutPhaseHeld), stepsInPhase(ro, v2alpha1.RolloutPhaseTimedOut)
	set(common.RolloutHeldConditionType, len(held) > 0, conditionReason(held, "StepsHeld", "NotHeld"), fmt.Sprintf("Held steps: %v", held))
	set(common.RolloutTimedOutConditionType, len(timedOut) > 0, conditionReason(timedOut, "StepsTimedOut", "NotTimedOut"), fmt.Sprintf("Timed out steps: %v", timedOut))
}

func stepsInPhase(ro *v2alpha1.RolloutStatus, phase v2alpha1.RolloutPhase) []string {
	var names []string
	for _, st := range ro.Steps {
		if st.Phase == phase {
			names = append(names, st.Name)
		}
	}
	return names
}

func conditionReason(steps []string, active, inactive string) string {
	if len(steps) > 0 {
		return active
	}
	return inactive
}

// syncProfileRolloutStatus copies each profile step status to its
// DatadogAgentProfile, clearing it when no rollout status is relevant.
func (r *Reconciler) syncProfileRolloutStatus(ctx context.Context, plan *rolloutPlan, status *v2alpha1.RolloutStatus) {
	steps := map[string]*v2alpha1.RolloutStepStatus{}
	if status != nil {
		for i := range status.Steps {
			st := &status.Steps[i]
			steps[orderedrollout.StepKey(st.Kind, st.Namespace, st.Name)] = st
		}
	}
	for key, profile := range plan.profiles {
		current := &v1alpha1.DatadogAgentProfile{}
		if err := r.client.Get(ctx, types.NamespacedName{Namespace: profile.Namespace, Name: profile.Name}, current); err != nil {
			r.log.Error(err, "unable to get profile for rollout status", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			continue
		}
		want := steps[key].DeepCopy()
		if apiequality.Semantic.DeepEqual(current.Status.Rollout, want) {
			continue
		}
		current.Status.Rollout = want
		if err := r.client.Status().Update(ctx, current); err != nil {
			r.log.Error(err, "unable to update profile rollout status", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
		}
	}
}

// emitRolloutEvents records events for rollout transitions between the
// persisted and the new status: aggregate events on the DatadogAgent and step
// events on the DatadogAgentProfile (or the DatadogAgent for the default step).
func (r *Reconciler) emitRolloutEvents(dda *v2alpha1.DatadogAgent, plan *rolloutPlan, decisions orderedrollout.Decisions, prev, next *v2alpha1.RolloutStatus) {
	if decisions.BypassConsumed && next != nil {
		r.recorder.Eventf(dda, corev1.EventTypeNormal, "RolloutBypassed", "Manual bypass %q consumed: rolling all changed steps in parallel", next.LastBypass)
	}
	if next == nil || next.Phase == v2alpha1.RolloutPhaseIdle {
		return
	}

	var prevPhase v2alpha1.RolloutPhase
	var prevPriority *int32
	prevSteps := map[string]v2alpha1.RolloutStepStatus{}
	if prev != nil {
		prevPhase, prevPriority = prev.Phase, prev.CurrentPriority
		for _, st := range prev.Steps {
			prevSteps[orderedrollout.StepKey(st.Kind, st.Namespace, st.Name)] = st
		}
	}

	if next.CurrentPriority != nil && (prevPriority == nil || *prevPriority != *next.CurrentPriority) {
		r.recorder.Eventf(dda, corev1.EventTypeNormal, "RolloutWaveStarted", "Rollout priority %d is active", *next.CurrentPriority)
	}
	if next.Phase != prevPhase {
		switch next.Phase {
		case v2alpha1.RolloutPhaseCompleted:
			if prev != nil {
				r.recorder.Event(dda, corev1.EventTypeNormal, "RolloutCompleted", next.Message)
			}
		case v2alpha1.RolloutPhaseTimedOut:
			r.recorder.Event(dda, corev1.EventTypeWarning, "RolloutTimedOut", next.Message)
		case v2alpha1.RolloutPhaseHeld:
			r.recorder.Event(dda, corev1.EventTypeNormal, "RolloutHeld", next.Message)
		}
	}

	for _, st := range next.Steps {
		key := orderedrollout.StepKey(st.Kind, st.Namespace, st.Name)
		p, ok := prevSteps[key]
		if ok && p.Phase == st.Phase && p.Reason == st.Reason && p.TargetHash == st.TargetHash {
			continue
		}
		reason, eventType := stepEventReason(st)
		if reason == "" {
			continue
		}
		var obj runtime.Object = dda
		if profile := plan.profiles[key]; profile != nil {
			obj = profile
		}
		r.recorder.Eventf(obj, eventType, reason, "Rollout step %s (priority %d): %s", st.Name, st.Priority, st.Message)
	}
}

func stepEventReason(st v2alpha1.RolloutStepStatus) (string, string) {
	switch {
	case st.Phase == v2alpha1.RolloutPhaseInProgress && st.Reason == orderedrollout.ReasonSpecApplying:
		return "RolloutStepStarted", corev1.EventTypeNormal
	case st.Phase == v2alpha1.RolloutPhaseCompleted && st.Reason == orderedrollout.ReasonComplete:
		return "RolloutStepCompleted", corev1.EventTypeNormal
	case st.Phase == v2alpha1.RolloutPhaseHeld:
		return "RolloutStepHeld", corev1.EventTypeNormal
	case st.Phase == v2alpha1.RolloutPhaseSkipped:
		return "RolloutStepSkipped", corev1.EventTypeNormal
	case st.Phase == v2alpha1.RolloutPhaseTimedOut:
		return "RolloutStepTimedOut", corev1.EventTypeWarning
	case st.Phase == v2alpha1.RolloutPhaseBlocked:
		return "RolloutStepBlocked", corev1.EventTypeWarning
	}
	return "", ""
}
