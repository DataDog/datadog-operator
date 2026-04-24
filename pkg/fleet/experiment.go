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
	"sync"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	toolscache "k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

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
		maps.Copy(patch, specPatch)
	}

	return json.Marshal(patch)
}

// phaseWaitTimeout is the maximum time the daemon waits for the reconciler
// to process an annotation and transition the experiment phase. If the expected
// phase is not observed within this window, the daemon writes a self-abort
// (signal=rollback) and returns an error.
const phaseWaitTimeout = 5 * time.Minute

// phaseAcceptFunc determines whether an observed phase satisfies the wait condition.
// It returns (done bool, err error):
//   - (true, nil): expected phase observed — ack success.
//   - (true, error): unexpected terminal phase — ack error.
//   - (false, nil): keep waiting.
type phaseAcceptFunc func(phase v2alpha1.ExperimentPhase) (bool, error)

// acceptPhase returns a phaseAcceptFunc that accepts any of the given phases.
// If the observed phase is terminal but not in the accept set, it returns an error.
// If the observed phase is non-terminal and not in the accept set, it keeps waiting.
func acceptPhase(phases ...v2alpha1.ExperimentPhase) phaseAcceptFunc {
	accept := make(map[v2alpha1.ExperimentPhase]struct{}, len(phases))
	for _, p := range phases {
		accept[p] = struct{}{}
	}
	return func(phase v2alpha1.ExperimentPhase) (bool, error) {
		if _, ok := accept[phase]; ok {
			return true, nil
		}
		if isTerminalPhase(phase) {
			return true, fmt.Errorf("expected phase %v, got terminal phase %q", phases, phase)
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

// phaseResult is the outcome delivered through the phaseWatcher channel.
type phaseResult struct {
	phase v2alpha1.ExperimentPhase
	err   error
}

// phaseWaiter holds the registration for a single in-flight phase wait.
type phaseWaiter struct {
	nsn    types.NamespacedName
	accept phaseAcceptFunc
	ch     chan phaseResult
}

// phaseWatcher uses a DDA informer to observe experiment phase transitions.
// Only one waiter is active at a time (serial delivery guarantee from RC).
type phaseWatcher struct {
	k8sClient client.Client

	mu      sync.Mutex
	current *phaseWaiter
}

// newPhaseWatcher creates a phaseWatcher and registers an event handler on the
// provided DDA informer. The informer's UpdateFunc evaluates the active waiter's
// accept function whenever a DDA status changes.
func newPhaseWatcher(informer cache.Informer, k8sClient client.Client) *phaseWatcher {
	pw := &phaseWatcher{k8sClient: k8sClient}

	informer.AddEventHandler(toolscache.FilteringResourceEventHandler{
		FilterFunc: func(obj any) bool {
			dda, ok := obj.(*v2alpha1.DatadogAgent)
			return ok && dda.Status.Experiment != nil
		},
		Handler: toolscache.ResourceEventHandlerFuncs{
			AddFunc: func(obj any) {
				pw.evaluate(obj.(*v2alpha1.DatadogAgent))
			},
			UpdateFunc: func(_, newObj any) {
				pw.evaluate(newObj.(*v2alpha1.DatadogAgent))
			},
		},
	})

	return pw
}

// evaluate checks if the DDA matches the active waiter and signals it if done.
func (pw *phaseWatcher) evaluate(dda *v2alpha1.DatadogAgent) {
	pw.mu.Lock()
	w := pw.current
	pw.mu.Unlock()

	if w == nil {
		return
	}

	nsn := types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}
	if nsn != w.nsn {
		return
	}

	if dda.Status.Experiment == nil {
		return
	}

	done, err := w.accept(dda.Status.Experiment.Phase)
	if !done {
		return
	}

	// Non-blocking send — only one waiter reads, buffer size 1.
	select {
	case w.ch <- phaseResult{phase: dda.Status.Experiment.Phase, err: err}:
	default:
	}
}

// waitForPhase registers a waiter and blocks until the accept function is
// satisfied or the phaseWaitTimeout elapses. On timeout, it writes a self-abort
// (signal=rollback) and returns an error.
func (pw *phaseWatcher) waitForPhase(ctx context.Context, nsn types.NamespacedName, experimentID string, accept phaseAcceptFunc) error {
	logger := ctrl.LoggerFrom(ctx)

	w := &phaseWaiter{
		nsn:    nsn,
		accept: accept,
		ch:     make(chan phaseResult, 1),
	}

	pw.mu.Lock()
	pw.current = w
	pw.mu.Unlock()

	defer func() {
		pw.mu.Lock()
		if pw.current == w {
			pw.current = nil
		}
		pw.mu.Unlock()
	}()

	ctx, cancel := context.WithTimeout(ctx, phaseWaitTimeout)
	defer cancel()

	select {
	case result := <-w.ch:
		if result.err != nil {
			logger.Info("Unexpected phase observed", "phase", result.phase, "error", result.err)
			return result.err
		}
		logger.V(1).Info("Expected phase observed", "phase", result.phase)
		return nil

	case <-ctx.Done():
		// Timeout — self-abort by writing rollback annotation.
		logger.Info("Phase wait timed out, writing self-abort rollback signal")
		selfAbortCtx, selfAbortCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer selfAbortCancel()
		patch, patchErr := buildSignalPatch(v2alpha1.ExperimentSignalRollback, experimentID)
		if patchErr != nil {
			logger.Error(patchErr, "Failed to build self-abort rollback patch")
		} else {
			dda := &v2alpha1.DatadogAgent{}
			dda.Name = nsn.Name
			dda.Namespace = nsn.Namespace
			if patchErr = pw.k8sClient.Patch(selfAbortCtx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon")); patchErr != nil {
				logger.Error(patchErr, "Failed to write self-abort rollback signal")
			}
		}
		return fmt.Errorf("timed out waiting for expected phase transition after %s", phaseWaitTimeout)
	}
}
