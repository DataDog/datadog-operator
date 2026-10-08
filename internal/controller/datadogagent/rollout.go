// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/orderedrollout"
)

// rolloutWriteFailedReason marks a step whose DatadogAgentInternal write failed.
const rolloutWriteFailedReason = "WriteFailed"

// rolloutPlan is the snapshot evaluated by orderedrollout.Decide, with the
// objects needed to act on the decisions.
type rolloutPlan struct {
	inputs   orderedrollout.Inputs
	rendered map[string]*v1alpha1.DatadogAgentInternal
	live     map[string]*v1alpha1.DatadogAgentInternal
	profiles map[string]*v1alpha1.DatadogAgentProfile
}

// buildRolloutPlan computes rollout target hashes, sets the target hash
// annotation on the rendered DatadogAgentInternals and reads the live objects.
func (r *Reconciler) buildRolloutPlan(ctx context.Context, instance *v2alpha1.DatadogAgent, appliedProfiles []*v1alpha1.DatadogAgentProfile, ddais []*v1alpha1.DatadogAgentInternal, now time.Time) (*rolloutPlan, error) {
	plan := &rolloutPlan{
		inputs: orderedrollout.Inputs{
			DDAHold:    orderedrollout.IsAnnotationTrue(instance.Annotations, orderedrollout.HoldAnnotation),
			Bypass:     instance.Annotations[orderedrollout.BypassAnnotation],
			Prev:       instance.Status.Rollout,
			Generation: instance.Generation,
			Now:        now,
		},
		rendered: map[string]*v1alpha1.DatadogAgentInternal{},
		live:     map[string]*v1alpha1.DatadogAgentInternal{},
		profiles: map[string]*v1alpha1.DatadogAgentProfile{},
	}

	profilesByDDAIName := map[string]*v1alpha1.DatadogAgentProfile{}
	for _, p := range appliedProfiles {
		profilesByDDAIName[getProfileDDAIName(instance.Name, p.Name, p.Namespace)] = p
	}
	ddaConfig := orderedrollout.ResolveDDAConfig(instance.Spec.RolloutStrategy)

	for _, ddai := range ddais {
		step := orderedrollout.StepFacts{
			Kind:   v2alpha1.RolloutStepKindDefault,
			Name:   "default",
			Source: "DatadogAgent/" + instance.Name,
			Config: ddaConfig,
		}
		if ddai.Labels[constants.ProfileLabelKey] != "" {
			profile := profilesByDDAIName[ddai.Name]
			if profile == nil {
				return nil, fmt.Errorf("no applied DatadogAgentProfile for DatadogAgentInternal %s", ddai.Name)
			}
			step.Kind = v2alpha1.RolloutStepKindProfile
			step.Name, step.Namespace = profile.Name, profile.Namespace
			step.CreationTime = profile.CreationTimestamp.Time
			step.Source = fmt.Sprintf("DatadogAgentProfile/%s/%s", profile.Namespace, profile.Name)
			step.Config = orderedrollout.ResolveDAPConfig(ddaConfig, profile.Spec.Rollout)
			step.Hold = orderedrollout.IsAnnotationTrue(profile.Annotations, orderedrollout.HoldAnnotation)
			step.Skip = orderedrollout.IsAnnotationTrue(profile.Annotations, orderedrollout.SkipAnnotation)
			plan.profiles[step.Key()] = profile
		}

		target, err := targetHashForDDAI(ddai)
		if err != nil {
			return nil, err
		}
		if ddai.Annotations == nil {
			ddai.Annotations = map[string]string{}
		}
		ddai.Annotations[orderedrollout.TargetHashAnnotation] = target
		step.TargetHash = target

		live, err := r.getRolloutLiveFacts(ctx, ddai)
		if err != nil {
			return nil, err
		}
		step.Live = live.facts
		if live.ddai != nil {
			plan.live[step.Key()] = live.ddai
			if v, ok := live.ddai.Annotations[orderedrollout.StartedAtAnnotation]; ok && live.facts.TargetHash == target {
				ddai.Annotations[orderedrollout.StartedAtAnnotation] = v
			}
		}
		plan.rendered[step.Key()] = ddai
		plan.inputs.Steps = append(plan.inputs.Steps, step)
	}
	return plan, nil
}

// targetHashForDDAI returns the rollout target hash of a DatadogAgentInternal.
// The spec hash is read from the legacy spec hash annotation when present, so
// a live object written by an older operator hashes like its rendered form.
func targetHashForDDAI(ddai *v1alpha1.DatadogAgentInternal) (string, error) {
	specHash := ddai.Annotations[constants.MD5DDAIDeploymentAnnotationKey]
	if specHash == "" {
		h, err := comparison.GenerateMD5ForSpec(ddai.Spec)
		if err != nil {
			return "", err
		}
		specHash = h
	}
	return orderedrollout.TargetHash(specHash, ddai.Annotations), nil
}

type rolloutLive struct {
	ddai  *v1alpha1.DatadogAgentInternal
	facts *orderedrollout.LiveFacts
}

func (r *Reconciler) getRolloutLiveFacts(ctx context.Context, rendered *v1alpha1.DatadogAgentInternal) (rolloutLive, error) {
	live := &v1alpha1.DatadogAgentInternal{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: rendered.Namespace, Name: rendered.Name}, live); err != nil {
		if apierrors.IsNotFound(err) {
			return rolloutLive{}, nil
		}
		return rolloutLive{}, err
	}

	liveHash := live.Annotations[orderedrollout.TargetHashAnnotation]
	if liveHash == "" {
		h, err := targetHashForDDAI(live)
		if err != nil {
			return rolloutLive{}, err
		}
		liveHash = h
	}
	facts := &orderedrollout.LiveFacts{
		TargetHash:             liveHash,
		Generation:             live.Generation,
		ObservedGeneration:     live.Status.ObservedGeneration,
		AnnotationsDiffer:      !maps.Equal(live.Annotations, rendered.Annotations),
		InertAnnotationsDiffer: orderedrollout.InertAnnotationPatch(rendered.Annotations, live.Annotations) != nil,
	}
	if v := live.Annotations[orderedrollout.StartedAtAnnotation]; v != "" && liveHash == live.Annotations[orderedrollout.TargetHashAnnotation] {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			facts.StartedAt = &metav1.Time{Time: t}
		}
	}
	if c := condition.GetDDAICondition(&live.Status, common.DatadogAgentReconcileErrorConditionType); c != nil && c.Status == metav1.ConditionTrue {
		facts.ReconcileError = true
	}

	dsName := component.GetDaemonSetNameFromDatadogAgent(live, &live.Spec)
	if live.Status.Agent != nil {
		facts.HasAgent = true
		if live.Status.Agent.DaemonsetName != "" {
			dsName = live.Status.Agent.DaemonsetName
		}
	}
	ds := &appsv1.DaemonSet{}
	if err := r.client.Get(ctx, types.NamespacedName{Namespace: live.Namespace, Name: dsName}, ds); err != nil {
		if !apierrors.IsNotFound(err) {
			return rolloutLive{}, err
		}
	} else {
		facts.HasAgent = true
		facts.DaemonSet = &orderedrollout.DaemonSetFacts{
			Generation:             ds.Generation,
			ObservedGeneration:     ds.Status.ObservedGeneration,
			DesiredNumberScheduled: ds.Status.DesiredNumberScheduled,
			UpdatedNumberScheduled: ds.Status.UpdatedNumberScheduled,
			NumberUnavailable:      ds.Status.NumberUnavailable,
		}
	}
	return rolloutLive{ddai: live, facts: facts}, nil
}

// actRollout executes the writes decided by the gate, creations first. A
// failed spec write leaves its step not started in the returned status; the
// next reconcile recomputes.
func (r *Reconciler) actRollout(ctx context.Context, plan *rolloutPlan, decisions orderedrollout.Decisions) (*v2alpha1.RolloutStatus, error) {
	status := decisions.Status.DeepCopy()
	startedAt := map[string]*metav1.Time{}
	for _, s := range decisions.Steps {
		startedAt[s.Key] = s.Status.StartedAt
	}
	var errs []error
	for _, w := range decisions.Writes {
		rendered := plan.rendered[w.Key]
		var err error
		switch w.Kind {
		case orderedrollout.WriteCreate, orderedrollout.WriteUpdate:
			if t := startedAt[w.Key]; t != nil {
				rendered.Annotations[orderedrollout.StartedAtAnnotation] = t.UTC().Format(time.RFC3339)
			}
			err = r.createOrUpdateDDAI(rendered)
		case orderedrollout.WritePatchMetadata:
			err = r.patchDDAIAnnotations(ctx, plan.live[w.Key], rendered, w.SyncHashes)
		}
		if err == nil {
			continue
		}
		errs = append(errs, fmt.Errorf("rollout step %s: %w", w.Key, err))
		if status != nil && w.Kind != orderedrollout.WritePatchMetadata {
			markRolloutWriteFailed(status, w.Key, plan.inputs.Now, err)
		}
	}
	return status, errors.Join(errs...)
}

func markRolloutWriteFailed(status *v2alpha1.RolloutStatus, key string, now time.Time, err error) {
	for i := range status.Steps {
		st := &status.Steps[i]
		if orderedrollout.StepKey(st.Kind, st.Namespace, st.Name) != key {
			continue
		}
		if st.StartedAt != nil && st.StartedAt.Time.Equal(now) {
			st.StartedAt = nil
		}
		st.Phase, st.Reason, st.Message = v2alpha1.RolloutPhaseBlocked, rolloutWriteFailedReason, err.Error()
		return
	}
}

// patchDDAIAnnotations patches only annotations on the live object, so a gated
// step never receives its rendered spec.
func (r *Reconciler) patchDDAIAnnotations(ctx context.Context, live, rendered *v1alpha1.DatadogAgentInternal, syncHashes bool) error {
	if live == nil {
		return nil
	}
	patched := live.DeepCopy()
	if patched.Annotations == nil {
		patched.Annotations = map[string]string{}
	}
	for k, v := range orderedrollout.InertAnnotationPatch(rendered.Annotations, live.Annotations) {
		if v == nil {
			delete(patched.Annotations, k)
		} else {
			patched.Annotations[k] = *v
		}
	}
	if syncHashes {
		for _, k := range []string{orderedrollout.TargetHashAnnotation, constants.MD5DDAIDeploymentAnnotationKey} {
			if v, ok := rendered.Annotations[k]; ok {
				patched.Annotations[k] = v
			}
		}
	}
	if maps.Equal(patched.Annotations, live.Annotations) {
		return nil
	}
	r.log.Info("patching DatadogAgentInternal annotations", "ns", live.Namespace, "name", live.Name)
	return r.client.Patch(ctx, patched, client.MergeFrom(live))
}
