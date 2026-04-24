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
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	ctrlcache "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
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
	watcher          *phaseWatcher
}

// NewDaemon creates a new Fleet Daemon. When revisionsEnabled is false, experiment
// signals are rejected because the reconciler cannot process them without the
// ControllerRevision machinery.
func NewDaemon(rcClient remoteconfig.RCClient, mgr manager.Manager, revisionsEnabled bool) *Daemon {
	return &Daemon{
		rcClient:         rcClient,
		client:           mgr.GetClient(),
		cache:            mgr.GetCache(),
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

	// Set up the DDA informer and phase watcher for experiment acks.
	// When cache is nil (unit tests), watcher stays nil and phase waits are skipped.
	if d.cache != nil {
		ddaInformer, err := d.cache.GetInformer(ctx, &v2alpha1.DatadogAgent{})
		if err != nil {
			return fmt.Errorf("failed to get DatadogAgent informer: %w", err)
		}
		d.watcher = newPhaseWatcher(ddaInformer, d.client)
		logger.Info("Phase watcher initialized with DDA informer")
	}

	d.rcClient.Subscribe(state.ProductInstallerConfig, handleInstallerConfigUpdate(ctx, func(configs map[string]installerConfig) error {
		return d.handleConfigs(ctx, configs)
	}))
	d.rcClient.Subscribe(state.ProductUpdaterTask, handleUpdaterTaskUpdate(ctx, func(req remoteAPIRequest) error {
		d.setTaskState(req.Package, req.ID, pbgo.TaskState_RUNNING, nil)
		err := d.handleRemoteAPIRequest(ctx, req)
		if err != nil {
			var stateErr *stateDoesntMatchError
			if errors.As(err, &stateErr) {
				d.setTaskState(req.Package, req.ID, pbgo.TaskState_INVALID_STATE, err)
			} else {
				d.setTaskState(req.Package, req.ID, pbgo.TaskState_ERROR, err)
			}
		} else {
			d.setTaskState(req.Package, req.ID, pbgo.TaskState_DONE, nil)
		}
		return err
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
func (d *Daemon) handleRemoteAPIRequest(ctx context.Context, req remoteAPIRequest) error {
	logger := ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "package", req.Package, "method", req.Method)
	logger.Info("Received remote API request")

	if !d.revisionsEnabled {
		return fmt.Errorf("experiment signals require the CreateControllerRevisions and DatadogAgentInternal feature gates")
	}

	if err := d.verifyExpectedState(req); err != nil {
		logger.Error(err, "Expected state mismatch")
		return err
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
//
// The target DDA's namespace/name is resolved from the task params if present,
// falling back to the installer config operation's NamespacedName.
func (d *Daemon) resolveOperation(req remoteAPIRequest, signal string) (fleetManagementOperation, error) {
	// get params version from req
	id := req.Params.Version
	if id == "" {
		return fleetManagementOperation{}, fmt.Errorf("%s: version is required", signal)
	}
	// match to d.configs[params.version] to get config
	cfg, err := d.getConfig(id)
	if err != nil {
		return fleetManagementOperation{}, fmt.Errorf("%s: %w", signal, err)
	}

	if len(cfg.Operations) != 1 {
		return fleetManagementOperation{}, fmt.Errorf("%s: config %s must have exactly 1 operation, got %d", signal, cfg.ID, len(cfg.Operations))
	}
	op := cfg.Operations[0]

	// Prefer namespace/name from task params over the installer config.
	if req.Params.Namespace != "" && req.Params.Name != "" {
		op.NamespacedName = types.NamespacedName{
			Namespace: req.Params.Namespace,
			Name:      req.Params.Name,
		}
	}

	if err := validateOperation(op); err != nil {
		return fleetManagementOperation{}, fmt.Errorf("%s: invalid operation: %w", signal, err)
	}

	op.GroupVersionKind = v2alpha1.GroupVersion.WithKind("DatadogAgent")

	return op, nil
}

// startDatadogAgentExperiment starts a DatadogAgent experiment by atomically
// patching both the DDA spec (experiment config) and experiment signal annotations.
// If the annotation ID already matches and the reconciler has already set
// phase=running, the patch is skipped. After writing, the daemon waits for the
// reconciler to set phase=running before acking the task to RC.
func (d *Daemon) startDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) error {
	logger := ctrl.LoggerFrom(ctx).WithValues("id", req.ID)
	logger.V(1).Info("Starting DatadogAgent experiment", "config", req.Params.Version)
	op, err := d.resolveOperation(req, "start DatadogAgent experiment")
	if err != nil {
		logger.Error(err, "Failed to resolve operation")
		return err
	}

	logger = logger.WithValues("namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name)
	ctx = ctrl.LoggerInto(ctx, logger)

	if op.Operation != OperationUpdate {
		return fmt.Errorf("start DatadogAgent experiment: invalid operation: %s", op.Operation)
	}

	// Check if this experiment is already running — skip the patch if so.
	skipPatch := false
	dda := &v2alpha1.DatadogAgent{}
	if err := d.client.Get(ctx, op.NamespacedName, dda); err == nil {
		if dda.Annotations[v2alpha1.AnnotationExperimentID] == req.ID {
			// Same ID already annotated. If the reconciler already set phase=running,
			// there's nothing to do. If it hasn't yet, the watcher will pick it up.
			if dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhaseRunning && dda.Status.Experiment.ID == req.ID {
				logger.V(1).Info("Experiment already running, skipping patch")
				return nil
			}
			logger.V(1).Info("Annotation already set, skipping patch")
			skipPatch = true
		}
		// Different experiment is already running — error.
		if !skipPatch && dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhaseRunning && dda.Status.Experiment.ID != req.ID {
			return fmt.Errorf("start DatadogAgent experiment: experiment %q already running", dda.Status.Experiment.ID)
		}
	}

	if !skipPatch {
		// Build atomic patch: spec + annotations in a single MergePatch.
		patch, err := buildSignalPatch(v2alpha1.ExperimentSignalStart, req.ID, op.Config)
		if err != nil {
			return fmt.Errorf("start DatadogAgent experiment: %w", err)
		}

		dda = &v2alpha1.DatadogAgent{}
		dda.Name = op.NamespacedName.Name
		dda.Namespace = op.NamespacedName.Namespace
		if err := retryWithBackoff(ctx, func() error {
			return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
		}); err != nil {
			return fmt.Errorf("start DatadogAgent experiment: failed to patch: %w", err)
		}

		// Report the experiment config version to the backend.
		stable, _ := d.getPackageConfigVersions(req.Package)
		d.setPackageConfigVersions(req.Package, stable, req.Params.Version)

		logger.Info("Wrote start signal")
	}

	// Wait for the reconciler to process the annotation and set phase=running.
	if d.watcher != nil {
		logger.V(1).Info("Waiting for phase=running")
		if err := d.watcher.waitForPhase(ctx, op.NamespacedName, req.ID, acceptPhase(v2alpha1.ExperimentPhaseRunning)); err != nil {
			return fmt.Errorf("start DatadogAgent experiment: %w", err)
		}
	}

	logger.Info("Started DatadogAgent experiment")
	return nil
}

// stopDatadogAgentExperiment writes a rollback signal annotation on the DDA.
// If the phase is already terminal, the patch is skipped. After writing, the
// daemon waits for any terminal phase before acking the task to RC.
func (d *Daemon) stopDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) error {
	op, err := d.resolveOperation(req, "stop DatadogAgent experiment")
	if err != nil {
		return err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Stopping DatadogAgent experiment")

	// Check if already in a terminal phase — skip the patch if so.
	dda := &v2alpha1.DatadogAgent{}
	if getErr := d.client.Get(ctx, op.NamespacedName, dda); getErr == nil {
		if dda.Status.Experiment != nil && isTerminalPhase(dda.Status.Experiment.Phase) {
			logger.V(1).Info("Already in terminal phase, skipping patch", "phase", dda.Status.Experiment.Phase)
			return nil
		}
	}

	// Write rollback signal annotation.
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalRollback, req.ID)
	if err != nil {
		return fmt.Errorf("stop DatadogAgent experiment: %w", err)
	}
	dda = &v2alpha1.DatadogAgent{}
	dda.Name = op.NamespacedName.Name
	dda.Namespace = op.NamespacedName.Namespace
	if err := retryWithBackoff(ctx, func() error {
		return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
	}); err != nil {
		return fmt.Errorf("stop DatadogAgent experiment: failed to patch annotation: %w", err)
	}

	// Clear the experiment config version.
	stable, _ := d.getPackageConfigVersions(req.Package)
	d.setPackageConfigVersions(req.Package, stable, "")

	logger.Info("Wrote rollback signal")

	// Wait for the reconciler to process the annotation and reach a terminal phase.
	// Stop acks on any terminal phase (terminated, promoted, aborted) — "already done."
	if d.watcher != nil {
		logger.V(1).Info("Waiting for terminal phase")
		if err := d.watcher.waitForPhase(ctx, op.NamespacedName, req.ID, acceptPhase(v2alpha1.ExperimentPhaseTerminated, v2alpha1.ExperimentPhasePromoted, v2alpha1.ExperimentPhaseAborted)); err != nil {
			return fmt.Errorf("stop DatadogAgent experiment: %w", err)
		}
	}

	logger.Info("Stopped DatadogAgent experiment")
	return nil
}

// promoteDatadogAgentExperiment writes a promote signal annotation on the DDA.
// If the phase is already promoted, the patch is skipped. After writing, the
// daemon waits for phase=promoted before acking the task to RC.
func (d *Daemon) promoteDatadogAgentExperiment(ctx context.Context, req remoteAPIRequest) error {
	op, err := d.resolveOperation(req, "promote DatadogAgent experiment")
	if err != nil {
		return err
	}

	ctx = ctrl.LoggerInto(ctx, ctrl.LoggerFrom(ctx).WithValues("id", req.ID, "namespace", op.NamespacedName.Namespace, "name", op.NamespacedName.Name))
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Promoting DatadogAgent experiment")

	// Verify there is an active experiment to promote.
	_, experiment := d.getPackageConfigVersions(req.Package)
	if experiment == "" {
		return fmt.Errorf("promote DatadogAgent experiment: no experiment config version set")
	}

	// Check if already promoted — skip the patch if so.
	dda := &v2alpha1.DatadogAgent{}
	if getErr := d.client.Get(ctx, op.NamespacedName, dda); getErr == nil {
		if dda.Status.Experiment != nil && dda.Status.Experiment.Phase == v2alpha1.ExperimentPhasePromoted {
			logger.V(1).Info("Already promoted, skipping patch")
			return nil
		}
	}

	// Write promote signal annotation.
	patch, err := buildSignalPatch(v2alpha1.ExperimentSignalPromote, req.ID)
	if err != nil {
		return fmt.Errorf("promote DatadogAgent experiment: %w", err)
	}
	dda = &v2alpha1.DatadogAgent{}
	dda.Name = op.NamespacedName.Name
	dda.Namespace = op.NamespacedName.Namespace
	if err := retryWithBackoff(ctx, func() error {
		return d.client.Patch(ctx, dda, client.RawPatch(types.MergePatchType, patch), client.FieldOwner("fleet-daemon"))
	}); err != nil {
		return fmt.Errorf("promote DatadogAgent experiment: failed to patch annotation: %w", err)
	}

	// Promote: stable = old experiment, experiment = ""
	d.setPackageConfigVersions(req.Package, experiment, "")

	logger.Info("Wrote promote signal")

	// Wait for the reconciler to process the annotation and set phase=promoted.
	if d.watcher != nil {
		logger.V(1).Info("Waiting for phase=promoted")
		if err := d.watcher.waitForPhase(ctx, op.NamespacedName, req.ID, acceptPhase(v2alpha1.ExperimentPhasePromoted)); err != nil {
			return fmt.Errorf("promote DatadogAgent experiment: %w", err)
		}
	}

	logger.Info("Promoted DatadogAgent experiment")
	return nil
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

// setPackageConfigVersions updates the config version fields of the
// package entry in the RC installer state. Only the config variants
// (StableConfigVersion/ExperimentConfigVersion) are set; the package variants
// (StableVersion/ExperimentVersion) are preserved since this is a config
// experiment, not a package upgrade.
func (d *Daemon) setPackageConfigVersions(pkgName, stable, experiment string) {
	if d.rcClient == nil {
		return
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
				Task:                    pkg.GetTask(),
				StableConfigVersion:     stable,
				ExperimentConfigVersion: experiment,
			})
			found = true
		} else {
			updated = append(updated, pkg)
		}
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
