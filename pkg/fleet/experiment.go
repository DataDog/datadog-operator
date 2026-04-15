// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"
	"math"
	"time"

	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

// validateOperation checks that a fleetManagementOperation has the fields
// required to locate and act on a DatadogAgent resource.
func validateOperation(op fleetManagementOperation) error {
	if op.NamespacedName.Name == "" {
		return fmt.Errorf("operation namespaced name must have a non-empty name")
	}
	if op.NamespacedName.Namespace == "" {
		return fmt.Errorf("operation namespaced name must have a non-empty namespace")
	}
	if op.GroupVersionKind.Kind != "DatadogAgent" {
		return fmt.Errorf("operation kind must be DatadogAgent, got %q", op.GroupVersionKind.Kind)
	}
	return nil
}

// canStart returns whether a start signal is allowed given the current experiment phase.
//
// Allowed from: nil/empty, rollback, timeout, promoted, aborted (start new experiment).
// Error from: running (experiment already running), stopped (rollback in progress).
func canStart(phase v2alpha1.ExperimentPhase) error {
	switch phase {
	case v2alpha1.ExperimentPhaseRunning:
		return fmt.Errorf("experiment already running (phase=%s); stop it before starting a new one", phase)
	case v2alpha1.ExperimentPhaseStopped:
		return fmt.Errorf("rollback in progress (phase=%s); wait for rollback to complete before starting a new experiment", phase)
	default:
		// nil/empty, rollback, timeout, promoted, aborted: start is allowed
		return nil
	}
}

// canStop returns whether a stop signal is allowed given the current experiment phase.
// Returns (true, nil) if the signal is a no-op and should be silently ignored, (false, err) if rejected, or (false, nil) to proceed.
//
// Allowed from: running.
// No-op from: stopped, rollback, timeout, promoted (already terminal or in-progress rollback).
// Error from: nil/empty (nothing to stop), aborted (terminal, user-driven state).
func canStop(ctx context.Context, phase v2alpha1.ExperimentPhase) (bool, error) {
	switch phase {
	case v2alpha1.ExperimentPhaseRunning:
		return false, nil
	case v2alpha1.ExperimentPhaseStopped,
		v2alpha1.ExperimentPhaseRollback,
		v2alpha1.ExperimentPhaseTimeout,
		v2alpha1.ExperimentPhasePromoted:
		ctrl.LoggerFrom(ctx).Info("Stop signal ignored: experiment is not in a stoppable state", "phase", phase)
		return true, nil
	case v2alpha1.ExperimentPhaseAborted:
		return false, fmt.Errorf("experiment was aborted due to a manual spec change (phase=%s)", phase)
	default:
		// nil/empty: no active experiment to stop
		return false, fmt.Errorf("no active experiment to stop (phase=%s)", phase)
	}
}

// canPromote returns whether a promote signal is allowed given the current experiment phase.
// Returns (true, nil) if the signal is a no-op and should be silently ignored, (false, err) if rejected, or (false, nil) to proceed.
//
// Allowed from: running.
// No-op from: stopped, rollback, timeout, promoted (already terminal or rolling back).
// Error from: nil/empty (nothing to promote), aborted (terminal, user-driven state).
func canPromote(ctx context.Context, phase v2alpha1.ExperimentPhase) (bool, error) {
	switch phase {
	case v2alpha1.ExperimentPhaseRunning:
		return false, nil
	case v2alpha1.ExperimentPhaseStopped,
		v2alpha1.ExperimentPhaseRollback,
		v2alpha1.ExperimentPhaseTimeout,
		v2alpha1.ExperimentPhasePromoted:
		ctrl.LoggerFrom(ctx).Info("Promote signal ignored: experiment is not in a promotable state", "phase", phase)
		return true, nil
	case v2alpha1.ExperimentPhaseAborted:
		return false, fmt.Errorf("experiment was aborted due to a manual spec change (phase=%s)", phase)
	default:
		// nil/empty: no active experiment to promote
		return false, fmt.Errorf("no active experiment to promote (phase=%s)", phase)
	}
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

// retryWithBackoff retries fn on any error with exponential backoff.
// The total retry window is bounded by a 3-minute context timeout.
func retryWithBackoff(ctx context.Context, fn func() error) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	return retry.OnError(experimentBackoff, func(err error) bool {
		return ctx.Err() == nil
	}, fn)
}
