// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
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
	patch := map[string]any{
		"metadata": map[string]any{
			"annotations": map[string]string{
				v2alpha1.AnnotationExperimentSignal: signal,
				v2alpha1.AnnotationExperimentID:     id,
			},
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

// phaseAcceptFunc determines whether an observed experiment status satisfies the
// wait condition. It receives the full ExperimentStatus (which may be nil) so
// that accept functions can key on the experiment ID, not just the phase.
// It returns (done bool, err error):
//   - (true, nil): expected phase observed — ack success.
//   - (true, error): unexpected terminal phase — ack error.
//   - (false, nil): keep waiting.
type phaseAcceptFunc func(status *v2alpha1.ExperimentStatus) (bool, error)

// acceptPhase returns a phaseAcceptFunc that accepts any of the given phases,
// but only when the status belongs to the specified experiment ID.
// If the status is nil or the ID doesn't match, it keeps waiting (the expected
// experiment hasn't started yet). If the ID matches and the phase is terminal
// but not in the accept set, it returns an error.
func acceptPhase(experimentID string, phases ...v2alpha1.ExperimentPhase) phaseAcceptFunc {
	accept := make(map[v2alpha1.ExperimentPhase]struct{}, len(phases))
	for _, p := range phases {
		accept[p] = struct{}{}
	}
	return func(status *v2alpha1.ExperimentStatus) (bool, error) {
		if status == nil || status.ID != experimentID {
			// Wrong experiment or no experiment — keep waiting.
			return false, nil
		}
		if _, ok := accept[status.Phase]; ok {
			return true, nil
		}
		if isTerminalPhase(status.Phase) {
			return true, fmt.Errorf("expected phase %v, got terminal phase %q", phases, status.Phase)
		}
		return false, nil
	}
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
