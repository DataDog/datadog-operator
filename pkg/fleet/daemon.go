// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"fmt"
	"sync"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

const (
	methodStartDatadogAgentExperiment   = "operator/start_datadogagent_experiment"
	methodStopDatadogAgentExperiment    = "operator/stop_datadogagent_experiment"
	methodPromoteDatadogAgentExperiment = "operator/promote_datadogagent_experiment"
)

var _ manager.Runnable = &Daemon{}
var _ manager.LeaderElectionRunnable = &Daemon{}

// Daemon subscribes to fleet-specific RC products (installer configs and tasks)
// and runs after leader election as a controller-runtime Runnable.
type Daemon struct {
	rcClient         remoteconfig.RCClient
	client           client.Client
	revisionsEnabled bool
	mu               sync.RWMutex
	configs          map[string]installerConfig // keyed by config ID; replaced on each RC update
}

// NewDaemon creates a new Fleet Daemon. When revisionsEnabled is false, experiment
// signals are rejected because the reconciler cannot process them without the
// ControllerRevision machinery.
func NewDaemon(rcClient remoteconfig.RCClient, k8sClient client.Client, revisionsEnabled bool) *Daemon {
	return &Daemon{
		rcClient:         rcClient,
		client:           k8sClient,
		revisionsEnabled: revisionsEnabled,
		configs:          make(map[string]installerConfig),
	}
}

// Start implements manager.Runnable. It subscribes to fleet RC products and
// blocks until ctx is cancelled.
func (d *Daemon) Start(ctx context.Context) error {
	logger := ctrl.LoggerFrom(ctx).WithName("fleet-daemon").WithValues("kind", "DatadogAgent")
	ctx = ctrl.LoggerInto(ctx, logger)
	logger.Info("Starting Fleet daemon")

	d.rcClient.Subscribe(state.ProductUpdaterAgent, handleInstallerConfigUpdate(func(configs map[string]installerConfig) error {
		return d.handleConfigs(ctx, configs)
	}))
	d.rcClient.Subscribe(state.ProductUpdaterTask, handleUpdaterTaskUpdate(func(req remoteAPIRequest) error {
		return d.handleRemoteAPIRequest(ctx, req)
	}))

	<-ctx.Done()
	logger.Info("Stopping Fleet daemon")
	return nil
}

// NeedLeaderElection implements manager.LeaderElectionRunnable.
// The daemon only runs on the elected leader.
func (d *Daemon) NeedLeaderElection() bool {
	return true
}

// handleConfigs replaces the stored installer configs with the latest RC update.
// Configs are indexed by their ID so they can be retrieved by task handlers.
func (d *Daemon) handleConfigs(ctx context.Context, configs map[string]installerConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	logger := ctrl.LoggerFrom(ctx)
	newConfigs := make(map[string]installerConfig, len(configs))
	for _, cfg := range configs {
		logger.Info("Received installer config", "id", cfg.ID, "operations", len(cfg.Operations))
		newConfigs[cfg.ID] = cfg
	}
	d.configs = newConfigs
	return nil
}

// getConfig returns the installer config with the given ID.
func (d *Daemon) getConfig(id string) (installerConfig, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	cfg, ok := d.configs[id]
	if !ok {
		return installerConfig{}, fmt.Errorf("config %s not found", id)
	}
	return cfg, nil
}

// handleRemoteAPIRequest dispatches the incoming task to the appropriate handler.
func (d *Daemon) handleRemoteAPIRequest(ctx context.Context, req remoteAPIRequest) error {
	ctrl.LoggerFrom(ctx).Info("Received remote API request", "id", req.ID, "package", req.Package, "method", req.Method)

	if !d.revisionsEnabled {
		return fmt.Errorf("experiment signals require the CreateControllerRevisions and DatadogAgentInternal feature gates")
	}

	switch req.Method {
	case methodStartDatadogAgentExperiment:
		return d.startDatadogAgentExperiment(ctx, req)
	case methodStopDatadogAgentExperiment:
		return d.stopDatadogAgentExperiment(ctx, req)
	case methodPromoteDatadogAgentExperiment:
		return d.promoteDatadogAgentExperiment(ctx, req)
	default:
		return fmt.Errorf("unknown method: %s", req.Method)
	}
}

// resolveOperation looks up the installer config for the request, validates its single
// DatadogAgent operation, and fills in the canonical GVK. Returns the operation ready for use.
func (d *Daemon) resolveOperation(req remoteAPIRequest, signal string) (fleetManagementOperation, error) {
	cfg, err := d.getConfig(req.ExpectedState.ExperimentConfig)
	if err != nil {
		return fleetManagementOperation{}, fmt.Errorf("%s: %w", signal, err)
	}

	if len(cfg.Operations) != 1 {
		return fleetManagementOperation{}, fmt.Errorf("%s: config %s must have exactly 1 operation, got %d", signal, cfg.ID, len(cfg.Operations))
	}
	op := cfg.Operations[0]

	if err := validateOperation(op); err != nil {
		return fleetManagementOperation{}, fmt.Errorf("%s: invalid operation: %w", signal, err)
	}

	op.GroupVersionKind = v2alpha1.GroupVersion.WithKind("DatadogAgent")

	return op, nil
}

// startDatadogAgentExperiment starts a DatadogAgent experiment.
// The first step updates the DDA spec with the experiment configuration.
// The second step updates the DDA status to running, recording the experiment ID and generation.
func (d *Daemon) startDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) error {
	op, err := d.resolveOperation(req, "start DatadogAgent experiment")
	if err != nil {
		return err
	}

	logger := ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name)
	ctx = ctrl.LoggerInto(ctx, logger)

	// Fetch current DDA to check signal preconditions.
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return fmt.Errorf("start DatadogAgent experiment: failed to get DatadogAgent %s: %w", op.NamespacedName, err)
	}

	if err := canStart(getExperimentPhase(dda)); err != nil {
		return fmt.Errorf("start DatadogAgent experiment: %w", err)
	}

	// Apply the spec patch.
	if err := retryWithBackoff(ctx, func() error {
		return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, op.Config))
	}); err != nil {
		return fmt.Errorf("start DatadogAgent experiment: failed to patch spec: %w", err)
	}

	// Re-fetch to get the generation that was bumped by the spec patch.
	if err := retryWithBackoff(ctx, func() error {
		return d.client.Get(ctx, op.NamespacedName, dda)
	}); err != nil {
		return fmt.Errorf("start DatadogAgent experiment: failed to re-fetch DatadogAgent: %w", err)
	}
	newGeneration := dda.Generation

	// Update status: phase=running, record experiment ID and new generation.
	// Re-fetch inside the retry to get the latest ResourceVersion on conflict.
	if err := retryWithBackoff(ctx, func() error {
		if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
			return err
		}
		dda.Status.Experiment = &v2alpha1.ExperimentStatus{
			Phase:      v2alpha1.ExperimentPhaseRunning,
			ID:         req.ID,
			Generation: newGeneration,
		}
		return d.client.Status().Update(ctx, dda)
	}); err != nil {
		return fmt.Errorf("start DatadogAgent experiment: failed to update status: %w", err)
	}

	logger.Info("Started DatadogAgent experiment", "generation", newGeneration)
	return nil
}

func (d *Daemon) stopDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) error {
	op, err := d.resolveOperation(req, "stop DatadogAgent experiment")
	if err != nil {
		return err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)

	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return fmt.Errorf("stop DatadogAgent experiment: failed to get DatadogAgent %s: %w", op.NamespacedName, err)
	}

	if isNoOp, err := canStop(ctx, getExperimentPhase(dda)); err != nil {
		return fmt.Errorf("stop DatadogAgent experiment: %w", err)
	} else if isNoOp {
		return nil
	}

	// Update status: set phase=stopped; leave ID and generation for the reconciler.
	if err := retryWithBackoff(ctx, func() error {
		if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
			return err
		}
		dda.Status.Experiment.Phase = v2alpha1.ExperimentPhaseStopped
		return d.client.Status().Update(ctx, dda)
	}); err != nil {
		return fmt.Errorf("stop DatadogAgent experiment: failed to update status: %w", err)
	}

	logger.Info("Stopped DatadogAgent experiment")
	return nil
}

func (d *Daemon) promoteDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) error {
	op, err := d.resolveOperation(req, "promote DatadogAgent experiment")
	if err != nil {
		return err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)

	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
		return fmt.Errorf("promote DatadogAgent experiment: failed to get DatadogAgent %s: %w", op.NamespacedName, err)
	}

	if isNoOp, err := canPromote(ctx, getExperimentPhase(dda)); err != nil {
		return fmt.Errorf("promote DatadogAgent experiment: %w", err)
	} else if isNoOp {
		return nil
	}

	// Update status: set phase=promoted; leave ID and generation intact.
	if err := retryWithBackoff(ctx, func() error {
		if err := d.client.Get(ctx, op.NamespacedName, dda); err != nil {
			return err
		}
		dda.Status.Experiment.Phase = v2alpha1.ExperimentPhasePromoted
		return d.client.Status().Update(ctx, dda)
	}); err != nil {
		return fmt.Errorf("promote DatadogAgent experiment: failed to update status: %w", err)
	}

	logger.Info("Promoted DatadogAgent experiment")
	return nil
}

// getExperimentPhase returns the current experiment phase from a DDA's status,
// or empty string if no experiment is active.
func getExperimentPhase(dda *v2alpha1.DatadogAgent) v2alpha1.ExperimentPhase {
	if dda.Status.Experiment == nil {
		return ""
	}
	return dda.Status.Experiment.Phase
}
