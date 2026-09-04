// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"time"

	jsonpatch "github.com/evanphx/json-patch/v5"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// errBaselineUninitialized means the reconciler has not yet published the
// current-revision barrier (status.currentRevision is empty).
var errBaselineUninitialized = errors.New("baseline revision not yet initialized")

// errBaselineNotReady means the reconciler has published a current-revision
// barrier, but it does not yet reflect the DDA's latest generation and
// annotations. planStart must not start an experiment against a stale baseline.
var errBaselineNotReady = errors.New("baseline revision not yet up to date")

// errBaselineConflict means applyOperation replanned once after an optimistic-lock
// conflict and hit a second conflict; the caller should surface the error rather
// than retry indefinitely.
var errBaselineConflict = errors.New("baseline changed concurrently, refusing to retry indefinitely")

// baselineFreshnessPollInterval is the delay between baseline-freshness re-checks.
// Var (not const) so tests can shrink it instead of waiting on real time.
var baselineFreshnessPollInterval = 200 * time.Millisecond

// baselineFreshnessBudget bounds how long waitForBaselineFreshness waits for the
// reconciler to publish a fresh current-revision barrier before giving up.
// Var (not const) so tests can shrink it instead of waiting on real time.
var baselineFreshnessBudget = 5 * time.Second

// validateParams checks that experimentParams has the fields required to locate
// and act on a DatadogAgent resource.
func validateParams(p experimentParams) error {
	if p.NamespacedName.Name == "" {
		return fmt.Errorf("params namespaced_name must have a non-empty name")
	}
	if p.NamespacedName.Namespace == "" {
		return fmt.Errorf("params namespaced_name must have a non-empty namespace")
	}
	if p.GroupVersionKind.Kind != "DatadogAgent" {
		return fmt.Errorf("params kind must be DatadogAgent, got %q", p.GroupVersionKind.Kind)
	}
	return nil
}

// experimentSignal is the signal value passed to resolveOperation, used as an error prefix.
type experimentSignal string

const (
	signalStartDatadogAgentExperiment   experimentSignal = "start DatadogAgent experiment"
	signalStopDatadogAgentExperiment    experimentSignal = "stop DatadogAgent experiment"
	signalPromoteDatadogAgentExperiment experimentSignal = "promote DatadogAgent experiment"
)

// resolvedOperation holds the resolved data needed to execute an experiment operation.
type resolvedOperation struct {
	NamespacedName types.NamespacedName
	Config         json.RawMessage
}

// experimentBackoff is the retry backoff for k8s operations during experiment signals.
// Retries start at 1s, doubling each attempt up to 10s, for up to 3 minutes total.
var experimentBackoff = wait.Backoff{
	Duration: 1 * time.Second,
	Factor:   2.0,
	Jitter:   0.1,
	Cap:      10 * time.Second,
	Steps:    math.MaxInt32,
}

// isRetryable returns true for errors that are worth retrying (i.e. not permanent).
func isRetryable(err error) bool {
	return !apierrors.IsNotFound(err) &&
		!apierrors.IsForbidden(err) &&
		!apierrors.IsInvalid(err) &&
		!apierrors.IsMethodNotSupported(err)
}

// retryWithBackoff retries fn on transient errors with exponential backoff.
// The total retry window is bounded by a 3-minute context timeout.
// Permanent errors (not-found, forbidden, invalid, method-not-supported) are not retried.
func retryWithBackoff(ctx context.Context, fn func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return retry.OnError(experimentBackoff, func(err error) bool {
		return ctx.Err() == nil && isRetryable(err)
	}, fn)
}

// buildSignalPatch creates a JSON merge patch that sets the experiment signal
// and ID annotations. If config is non-nil, spec fields from the config are
// merged into the patch so that the spec and annotations are written atomically.
func buildSignalPatch(signal, id string, config ...json.RawMessage) ([]byte, error) {
	return BuildSignalPatchWithAnnotations(signal, id, nil, config...)
}

// BuildSignalPatchWithAnnotations behaves like buildSignalPatch but also merges
// extra annotations (e.g. the rollback-target checkpoint) into the same patch.
// Exported so reconciler tests drive stop and promote signals through the real
// Fleet patch construction instead of an approximation that can drift.
func BuildSignalPatchWithAnnotations(signal, id string, extra map[string]string, config ...json.RawMessage) ([]byte, error) {
	annotations := map[string]string{
		v2alpha1.AnnotationExperimentSignal: signal,
		v2alpha1.AnnotationExperimentID:     id,
	}
	maps.Copy(annotations, extra)
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": annotations,
		},
	}

	if len(config) > 0 && config[0] != nil {
		var specPatch map[string]any
		if err := json.Unmarshal(config[0], &specPatch); err != nil {
			return nil, fmt.Errorf("failed to unmarshal config: %w", err)
		}
		// Top-level maps.Copy is safe because the config currently only contains
		// "spec" keys and never "metadata". If the config ever includes metadata,
		// this will need a deep merge to avoid overwriting the signal annotations.
		maps.Copy(patch, specPatch)
	}

	return json.Marshal(patch)
}

// BuildStartPatch builds the whole start patch: the signal, ID, rollback-target
// and expected-spec-hash annotations plus the experiment spec, all in one
// atomic JSON MergePatch.
//
// config is Fleet's whole-resource MergePatch (RC delivers `{"spec": {...}}`),
// so the expected-spec-hash has to be computed over the *post-merge* spec: the
// pin must equal what the reconciler will later compute from the live object
// once the patch has landed. Hashing the raw config fragment or the un-merged
// dda.Spec would produce a different shape and silently defeat the pin, so the
// merge is reproduced here in memory rather than approximated.
func BuildStartPatch(dda *v2alpha1.DatadogAgent, experimentID string, config json.RawMessage, rollbackTarget string) ([]byte, error) {
	expectedHash, err := expectedSpecHashAfterMerge(dda, config)
	if err != nil {
		return nil, err
	}
	return BuildSignalPatchWithAnnotations(v2alpha1.ExperimentSignalStart, experimentID, map[string]string{
		v2alpha1.AnnotationExperimentRollbackTargetRevision: rollbackTarget,
		v2alpha1.AnnotationExperimentExpectedSpecHash:       expectedHash,
	}, config)
}

// expectedSpecHashAfterMerge applies config onto a copy of dda in memory and
// hashes the result through the same helper the reconciler validates with.
func expectedSpecHashAfterMerge(dda *v2alpha1.DatadogAgent, config json.RawMessage) (string, error) {
	merged := dda.DeepCopy()
	if len(config) > 0 {
		base, err := json.Marshal(merged)
		if err != nil {
			return "", fmt.Errorf("failed to marshal DatadogAgent for merge: %w", err)
		}
		mergedBytes, err := jsonpatch.MergePatch(base, config)
		if err != nil {
			return "", fmt.Errorf("failed to merge config onto DatadogAgent: %w", err)
		}
		merged = &v2alpha1.DatadogAgent{}
		if err := json.Unmarshal(mergedBytes, merged); err != nil {
			return "", fmt.Errorf("failed to unmarshal merged DatadogAgent: %w", err)
		}
	}
	return v2alpha1.ComputeSpecHash(merged.Spec, merged.GetAnnotations())
}

// rollbackTargetIsOwned reports whether name resolves to a ControllerRevision
// owned by dda: same namespace (implied by the keyed Get), matching agent-name
// label, and a controller owner reference pointing at dda's UID. The
// rollback-target annotation is user-writable, so a same-ID start signal can
// only be trusted as a resend when the revision it names is genuinely this
// DDA's baseline.
func (d *Daemon) rollbackTargetIsOwned(ctx context.Context, dda *v2alpha1.DatadogAgent, name string) bool {
	rev := &appsv1.ControllerRevision{}
	nn := types.NamespacedName{Namespace: dda.GetNamespace(), Name: name}
	if err := d.client.Get(ctx, nn, rev); err != nil {
		return false
	}
	if rev.Labels[apicommon.DatadogAgentNameLabelKey] != dda.GetName() {
		return false
	}
	for _, ref := range rev.OwnerReferences {
		if ref.Controller != nil && *ref.Controller && ref.UID == dda.GetUID() {
			return true
		}
	}
	return false
}

// checkBaselineFreshness returns nil when dda's current-revision barrier
// (status.currentRevision + CurrentRevisionObservedGeneration +
// CurrentRevisionObservedAnnotationsHash) reflects dda's latest generation and
// annotations. planStart uses this to refuse starting an experiment against a
// baseline the reconciler has not finished checkpointing yet.
func checkBaselineFreshness(dda *v2alpha1.DatadogAgent) error {
	if dda.Status.CurrentRevision == "" {
		return errBaselineUninitialized
	}
	if dda.Status.CurrentRevisionObservedGeneration != dda.Generation {
		return errBaselineNotReady
	}
	annotationsHash, err := v2alpha1.DatadogAnnotationsHash(dda.Annotations)
	if err != nil {
		return fmt.Errorf("failed to hash annotations: %w", err)
	}
	if dda.Status.CurrentRevisionObservedAnnotationsHash != annotationsHash {
		return errBaselineNotReady
	}
	return nil
}

// isBaselineNotFreshErr reports whether err indicates the baseline barrier is
// missing or stale, as opposed to some other (non-retryable) failure.
func isBaselineNotFreshErr(err error) bool {
	return errors.Is(err, errBaselineUninitialized) || errors.Is(err, errBaselineNotReady)
}

// pollBaselineFreshness calls check repeatedly, spaced by interval, until it
// succeeds, returns a non-freshness error, the context is canceled, or budget
// elapses. It never holds any lock; callers run it before acquiring one.
func pollBaselineFreshness(ctx context.Context, interval, budget time.Duration, check func() error) error {
	deadline := time.Now().Add(budget)
	for {
		err := check()
		if err == nil || !isBaselineNotFreshErr(err) {
			return err
		}
		if !time.Now().Add(interval).Before(deadline) {
			return err
		}
		timer := time.NewTimer(interval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

// waitForBaselineFreshness blocks (without holding any Daemon lock) until the
// DDA identified by nsn has a fresh current-revision barrier, or the bounded
// freshness budget is exhausted.
func (d *Daemon) waitForBaselineFreshness(ctx context.Context, nsn types.NamespacedName) error {
	return pollBaselineFreshness(ctx, baselineFreshnessPollInterval, baselineFreshnessBudget, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.client.Get(ctx, nsn, dda); err != nil {
			return err
		}
		return checkBaselineFreshness(dda)
	})
}

// retryWithBackoffPreconditioned behaves like retryWithBackoff, but treats
// optimistic-lock conflicts (HTTP 409) as non-retryable so the caller can
// replan against the latest object instead of retrying a stale precondition.
func retryWithBackoffPreconditioned(ctx context.Context, fn func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return retry.OnError(experimentBackoff, func(err error) bool {
		return ctx.Err() == nil && isRetryable(err) && !apierrors.IsConflict(err)
	}, fn)
}

// isTerminalPhase returns true for terminal experiment phases.
func isTerminalPhase(phase v2alpha1.ExperimentPhase) bool {
	switch phase {
	case v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentPhasePromoted, v2alpha1.ExperimentPhaseAborted:
		return true
	default:
		return false
	}
}
