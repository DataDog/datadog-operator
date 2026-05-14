// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"errors"
	"fmt"
	"sync"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"google.golang.org/protobuf/proto"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

// stateDoesntMatchError is returned by verifyExpectedState when the task's expected
// state doesn't match the operator's current installer state.
type stateDoesntMatchError struct {
	msg string
}

func (e *stateDoesntMatchError) Error() string { return e.msg }

const (
	methodStartDatadogAgentExperiment   = "operator/start_datadogagent_experiment"
	methodStopDatadogAgentExperiment    = "operator/stop_datadogagent_experiment"
	methodPromoteDatadogAgentExperiment = "operator/promote_datadogagent_experiment"
)

var _ manager.Runnable = &Daemon{}
var _ manager.LeaderElectionRunnable = &Daemon{}

// Daemon subscribes to fleet-specific RC products (installer configs and tasks)
// and runs after leader election as a controller-runtime Runnable.
//
// The daemon is a pure RC adapter: it translates RC tasks into DDA annotation
// writes and observes phase transitions to report outcomes back to RC. It never
// reads or writes status.experiment.phase — that is the reconciler's exclusive
// responsibility.
type Daemon struct {
	rcClient         remoteconfig.RCClient
	client           client.Client
	cache            ctrlcache.Cache
	revisionsEnabled bool
	mu               sync.RWMutex
	configs          map[string]installerConfig // keyed by config ID; replaced on each RC update
	taskMu           sync.Mutex                 // serializes task dispatch and package task-state updates
	// statusUpdates carries DDA informer events to the worker. The worker reads
	// status.experiment and pending annotations to update RC task state.
	statusUpdates chan ddaStatusSnapshot
}

// NewDaemon creates a new Fleet Daemon. When revisionsEnabled is false, experiment
// signals are rejected because the reconciler cannot process them without the
// ControllerRevision machinery.
func NewDaemon(rcClient remoteconfig.RCClient, mgr manager.Manager, revisionsEnabled bool) *Daemon {
	return &Daemon{
		rcClient:         rcClient,
		client:           mgr.GetClient(),
		cache:            mgr.GetCache(), // Informer cache
		revisionsEnabled: revisionsEnabled,
		configs:          make(map[string]installerConfig),
		statusUpdates:    make(chan ddaStatusSnapshot, 128),
	}
}

// Start implements manager.Runnable. It subscribes to fleet RC products and
// blocks until ctx is cancelled.
func (d *Daemon) Start(ctx context.Context) error {
	logger := ctrl.LoggerFrom(ctx).WithName("fleet-daemon").WithValues("kind", "DatadogAgent")
	ctx = ctrl.LoggerInto(ctx, logger)
	logger.Info("Starting Fleet daemon")

	if d.cache == nil {
		return fmt.Errorf("fleet daemon requires a controller cache")
	}
	if err := d.installDDAStatusForwarder(ctx); err != nil {
		return err
	}
	logger.Info("DDA status worker initialized")

	d.rcClient.Subscribe(state.ProductInstallerConfig, handleInstallerConfigUpdate(ctx, func(configs map[string]installerConfig) error {
		return d.handleConfigs(ctx, configs)
	}))
	d.rcClient.Subscribe(state.ProductUpdaterTask, handleUpdaterTaskUpdate(ctx, func(req remoteAPIRequest) error {
		return d.handleTask(ctx, req)
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

// handleTask serializes task dispatch bookkeeping and package task-state updates.
func (d *Daemon) handleTask(ctx context.Context, req remoteAPIRequest) error {
	d.taskMu.Lock()
	pending, err := d.handleRemoteAPIRequest(ctx, req)
	if err != nil {
		// Expected and current stable/experiment configs don't match.
		var stateErr *stateDoesntMatchError
		if errors.As(err, &stateErr) {
			d.setTaskState(req.Package, req.ID, pbgo.TaskState_INVALID_STATE, err)
		} else {
			d.setTaskState(req.Package, req.ID, pbgo.TaskState_ERROR, err)
		}
		d.taskMu.Unlock()
		return err
	}
	// The request is not relevant (stop a terminated experiment) or the desired
	// state is already true.
	if pending == nil {
		// Nothing is left for the worker to wait on.
		d.setTaskState(req.Package, req.ID, pbgo.TaskState_DONE, nil)
		d.taskMu.Unlock()
		return nil
	}
	// The DDA annotations are already written. From the task handler's point
	// of view dispatch is done; the worker watches DDA status and updates
	// Task.State.
	d.taskMu.Unlock()
	return nil
}

// handleConfigs replaces the stored installer configs with the latest RC update.
// Configs are indexed by their ID so they can be retrieved by task handlers.
func (d *Daemon) handleConfigs(ctx context.Context, configs map[string]installerConfig) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	logger := ctrl.LoggerFrom(ctx)
	newConfigs := make(map[string]installerConfig, len(configs))
	for _, cfg := range configs {
		logger.V(2).Info("Received installer config", "id", cfg.ID, "operations", len(cfg.Operations))
		newConfigs[cfg.ID] = cfg
	}
	d.configs = newConfigs
	logger.V(2).Info("Updated installer configs", "configs", d.configs)
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

// verifyExpectedState checks that the config versions in the task's expected_state
// match the operator's current installer state for the given package.
// Returns *stateDoesntMatchError when they don't match.
func (d *Daemon) verifyExpectedState(req remoteAPIRequest) error {
	stable, experiment := d.getPackageConfigVersions(req.Package)
	if req.ExpectedState.StableConfig != stable || req.ExpectedState.ExperimentConfig != experiment {
		return &stateDoesntMatchError{
			msg: fmt.Sprintf(
				"state mismatch for package %s: expected stable_config=%q experiment_config=%q, got stable_config=%q experiment_config=%q",
				req.Package,
				req.ExpectedState.StableConfig, req.ExpectedState.ExperimentConfig,
				stable, experiment,
			),
		}
	}
	return nil
}

// handleRemoteAPIRequest dispatches the incoming task to the appropriate handler.
func (d *Daemon) handleRemoteAPIRequest(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	logger := ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "package", req.Package, "method", req.Method)
	logger.Info("Received remote API request")

	if !d.revisionsEnabled {
		return nil, fmt.Errorf("experiment signals require the CreateControllerRevisions and DatadogAgentInternal feature gates")
	}

	if err := d.verifyExpectedState(req); err != nil {
		logger.Error(err, "Expected state mismatch")
		return nil, err
	}

	switch req.Method {
	case methodStartDatadogAgentExperiment:
		return d.startDatadogAgentExperiment(ctx, req)
	case methodStopDatadogAgentExperiment:
		return d.stopDatadogAgentExperiment(ctx, req)
	case methodPromoteDatadogAgentExperiment:
		return d.promoteDatadogAgentExperiment(ctx, req)
	default:
		return nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}

// setTaskState updates the Task field of the package entry in the RC installer state.
// If the package is not yet present in the state, a new entry is added.
// This is a no-op when rcClient is nil (e.g. in unit tests that construct Daemon directly).
func (d *Daemon) setTaskState(pkgName, taskID string, taskState pbgo.TaskState, taskErr error) {
	if d.rcClient == nil {
		return
	}
	task := &pbgo.PackageStateTask{
		Id:    taskID,
		State: taskState,
	}
	if taskErr != nil {
		task.Error = &pbgo.TaskError{Message: taskErr.Error()}
	}

	current := d.rcClient.GetInstallerState()
	updated := make([]*pbgo.PackageState, 0, len(current)+1)
	found := false
	for _, pkg := range current {
		if pkg.GetPackage() == pkgName {
			updated = append(updated, &pbgo.PackageState{
				Package:                 pkg.GetPackage(),
				StableVersion:           pkg.GetStableVersion(),
				ExperimentVersion:       pkg.GetExperimentVersion(),
				Task:                    task,
				StableConfigVersion:     pkg.GetStableConfigVersion(),
				ExperimentConfigVersion: pkg.GetExperimentConfigVersion(),
			})
			found = true
		} else {
			updated = append(updated, pkg)
		}
	}
	if !found {
		updated = append(updated, &pbgo.PackageState{
			Package: pkgName,
			Task:    task,
		})
	}
	d.rcClient.SetInstallerState(updated)
	d.logInstallerState("setTaskState")
}

// getPackageConfigVersions returns the current stable and experiment config versions
// for the given package from the RC installer state.
func (d *Daemon) getPackageConfigVersions(pkgName string) (stable, experiment string) {
	if d.rcClient == nil {
		return "", ""
	}
	for _, pkg := range d.rcClient.GetInstallerState() {
		if pkg.GetPackage() == pkgName {
			return pkg.GetStableConfigVersion(), pkg.GetExperimentConfigVersion()
		}
	}
	return "", ""
}

// setPackageConfigVersions updates only the config versions for one package.
func (d *Daemon) setPackageConfigVersions(pkgName, stable, experiment string) {
	if d.rcClient == nil {
		return
	}
	current := d.rcClient.GetInstallerState()
	updated := make([]*pbgo.PackageState, 0, len(current)+1)
	found := false
	for _, pkg := range current {
		if pkg.GetPackage() == pkgName {
			next := proto.Clone(pkg).(*pbgo.PackageState)
			next.StableConfigVersion = stable
			next.ExperimentConfigVersion = experiment
			updated = append(updated, next)
			found = true
			continue
		}
		updated = append(updated, pkg)
	}
	if !found {
		updated = append(updated, &pbgo.PackageState{
			Package:                 pkgName,
			StableConfigVersion:     stable,
			ExperimentConfigVersion: experiment,
		})
	}
	d.rcClient.SetInstallerState(updated)
	d.logInstallerState("setPackageConfigVersions")
}

// logInstallerState logs the full installer state for debugging.
func (d *Daemon) logInstallerState(caller string) {
	if d.rcClient == nil {
		return
	}
	logger := ctrl.Log.WithName("fleet-daemon")
	for _, pkg := range d.rcClient.GetInstallerState() {
		var taskID string
		var taskState pbgo.TaskState
		if pkg.GetTask() != nil {
			taskID = pkg.GetTask().GetId()
			taskState = pkg.GetTask().GetState()
		}
		logger.V(1).Info("Installer state",
			"caller", caller,
			"package", pkg.GetPackage(),
			"stableVersion", pkg.GetStableVersion(),
			"experimentVersion", pkg.GetExperimentVersion(),
			"stableConfigVersion", pkg.GetStableConfigVersion(),
			"experimentConfigVersion", pkg.GetExperimentConfigVersion(),
			"taskID", taskID,
			"taskState", taskState,
		)
	}
}
