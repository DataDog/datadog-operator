// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"
	"sync"
	"time"

	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const phaseWaitTimeout = 5 * time.Minute

type phaseResult struct {
	phase v2alpha1.ExperimentPhase
	err   error
}

type phaseWaiter struct {
	nsn    types.NamespacedName
	accept phaseAcceptFunc
	ch     chan phaseResult
}

type phaseWatcher struct {
	k8sClient client.Client

	mu      sync.Mutex
	nextID  uint64
	waiters map[uint64]*phaseWaiter
}

func newPhaseWatcher(informer cache.Informer, k8sClient client.Client) *phaseWatcher {
	pw := &phaseWatcher{
		k8sClient: k8sClient,
		waiters:   make(map[uint64]*phaseWaiter),
	}

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

func (pw *phaseWatcher) evaluate(dda *v2alpha1.DatadogAgent) {
	pw.mu.Lock()
	waiters := make([]*phaseWaiter, 0, len(pw.waiters))
	for _, w := range pw.waiters {
		waiters = append(waiters, w)
	}
	pw.mu.Unlock()

	if len(waiters) == 0 {
		return
	}

	for _, w := range waiters {
		nsn := types.NamespacedName{Namespace: dda.Namespace, Name: dda.Name}
		if nsn != w.nsn {
			continue
		}

		done, err := w.accept(dda.Status.Experiment)
		if !done {
			continue
		}

		select {
		case w.ch <- phaseResult{phase: dda.Status.Experiment.Phase, err: err}:
		default:
		}
	}
}

func (pw *phaseWatcher) waitForPhase(ctx context.Context, nsn types.NamespacedName, experimentID string, accept phaseAcceptFunc) error {
	logger := ctrl.LoggerFrom(ctx)

	w := &phaseWaiter{
		nsn:    nsn,
		accept: accept,
		ch:     make(chan phaseResult, 1),
	}

	pw.mu.Lock()
	waiterID := pw.nextID
	pw.nextID++
	pw.waiters[waiterID] = w
	pw.mu.Unlock()

	defer func() {
		pw.mu.Lock()
		delete(pw.waiters, waiterID)
		pw.mu.Unlock()
	}()

	current := &v2alpha1.DatadogAgent{}
	if err := pw.k8sClient.Get(ctx, nsn, current); err == nil {
		done, acceptErr := accept(current.Status.Experiment)
		if done {
			if acceptErr != nil {
				logger.Info("Unexpected phase already present", "phase", current.Status.Experiment.Phase, "error", acceptErr)
				return acceptErr
			}
			logger.V(1).Info("Expected phase already present", "phase", current.Status.Experiment.Phase)
			return nil
		}
	}

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
