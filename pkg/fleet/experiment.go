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

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

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
	return buildSignalPatchWithAnnotations(signal, id, nil, config...)
}

// buildSignalPatchWithAnnotations behaves like buildSignalPatch and additionally
// merges caller-supplied annotations into the patch. Callers use this for
// signal-adjacent metadata that must land atomically with the spec write —
// e.g. the daemon writes the rollback-target-revision annotation here so the
// reconciler observes the baseline checkpoint in the same MergePatch that
// carried the start signal.
func buildSignalPatchWithAnnotations(signal, id string, extraAnnotations map[string]string, config ...json.RawMessage) ([]byte, error) {
	annotations := map[string]string{
		v2alpha1.AnnotationExperimentSignal: signal,
		v2alpha1.AnnotationExperimentID:     id,
	}
	maps.Copy(annotations, extraAnnotations)
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

// Baseline-freshness sentinel errors. These distinguish the ways a Fleet
// start can fail before the daemon writes the spec, so the RC ERROR carries
// an actionable reason.
var (
	errBaselineUninitialized = errors.New("baseline-uninitialized")
	errBaselineNotReady      = errors.New("baseline-not-ready")
	errBaselineConflict      = errors.New("baseline-conflict")
)

// checkBaselineFreshness returns nil when the reconciler-published current-
// revision pointer is fresh for the DDA's current generation. Callers use the
// returned sentinel to distinguish the failure mode when reporting ERROR to
// Remote Config.
func checkBaselineFreshness(dda *v2alpha1.DatadogAgent) error {
	if dda.Status.CurrentRevision == "" {
		return errBaselineUninitialized
	}
	if dda.Status.CurrentRevisionObservedGeneration != dda.Generation {
		return errBaselineNotReady
	}
	return nil
}

// waitForBaselineFreshness re-reads the DDA and evaluates freshness with a
// bounded retry budget (~10s across 4 attempts, exponential 500ms → 4s).
// Callers hold no daemon locks while this runs, so other operations on the
// same DDA are not stalled waiting for the reconciler to catch up. Returns
// the fresh DDA on success, or the last freshness sentinel on failure.
func (d *Daemon) waitForBaselineFreshness(ctx context.Context, nsn types.NamespacedName) (*v2alpha1.DatadogAgent, error) {
	backoff := 500 * time.Millisecond
	const maxAttempts = 4
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}
		dda := &v2alpha1.DatadogAgent{}
		if err := d.client.Get(ctx, nsn, dda); err != nil {
			lastErr = err
			continue
		}
		if err := checkBaselineFreshness(dda); err != nil {
			lastErr = err
			continue
		}
		return dda, nil
	}
	return nil, lastErr
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
