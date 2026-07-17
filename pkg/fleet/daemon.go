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
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
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
// The daemon processes local managed Agent installation intents and translates
// RC tasks into experiment annotation writes. It reports both outcomes through RC
// and never writes status.experiment.phase, which belongs to the reconciler.
type Daemon struct {
	rcClient                                     remoteconfig.RCClient
	client                                       client.Client
	apiReader                                    client.Reader // bypasses the informer cache; used at startup before the cache is populated
	cache                                        ctrlcache.Cache
	recorder                                     record.EventRecorder // Kubernetes-event recorder for fleet-daemon-source events (gated by env var)
	revisionsEnabled                             bool
	managedAgentInstallationIdentity             ManagedAgentInstallationIdentity
	managedAgentInstallationTaskRunner           func(func())
	mu                                           sync.RWMutex
	configs                                      map[string]installerConfig // keyed by config ID; replaced on each RC update
	taskMu                                       sync.Mutex                 // serializes package task-state updates
	installerStateMu                             sync.Mutex                 // serializes RC installer-state read/modify/write operations
	transitionMu                                 sync.Mutex                 // serializes DDA managed Agent installation and experiment state transitions
	managedAgentInstallationActive               bool                       // reserves installer state while a managed Agent installation mutation is in progress
	managedAgentInstallationOperationID          string
	managedAgentInstallationCancel               context.CancelFunc
	managedAgentInstallationDone                 chan struct{}
	managedAgentInstallationTaskReserved         bool
	managedAgentInstallationCredentialRetryIndex int
	// statusUpdates carries DDA informer events to the worker. The worker reads
	// status.experiment and pending annotations to update RC task state.
	statusUpdates                   chan ddaStatusSnapshot
	managedAgentInstallationUpdates chan struct{}
}

// DaemonOption configures optional Fleet daemon transports.
type DaemonOption func(*Daemon)

// WithManagedAgentInstallation enables managed Agent installation intents for identity.
func WithManagedAgentInstallation(identity ManagedAgentInstallationIdentity) DaemonOption {
	return func(daemon *Daemon) {
		daemon.managedAgentInstallationIdentity = identity
		daemon.managedAgentInstallationTaskReserved = true
	}
}

// NewDaemon creates a new Fleet Daemon. When revisionsEnabled is false, experiment
// signals are rejected because the reconciler cannot process them without the
// ControllerRevision machinery.
func NewDaemon(rcClient remoteconfig.RCClient, mgr manager.Manager, revisionsEnabled bool, options ...DaemonOption) *Daemon {
	daemon := &Daemon{
		rcClient:         rcClient,
		client:           mgr.GetClient(),
		apiReader:        mgr.GetAPIReader(),
		cache:            mgr.GetCache(), // Informer cache
		recorder:         mgr.GetEventRecorderFor("fleet-daemon"),
		revisionsEnabled: revisionsEnabled,
		managedAgentInstallationTaskRunner: func(task func()) {
			go task()
		},
		configs:                         make(map[string]installerConfig),
		statusUpdates:                   make(chan ddaStatusSnapshot, 128),
		managedAgentInstallationUpdates: make(chan struct{}, 1),
	}
	for _, option := range options {
		option(daemon)
	}
	return daemon
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
	logger.Info("DDA status forwarder initialized")
	if err := d.rehydrateInstallerState(ctx); err != nil {
		logger.Error(err, "Failed to rehydrate installer state from existing DatadogAgents")
	}
	if d.managedAgentInstallationIdentity.Configured() {
		if err := d.rehydrateManagedAgentInstallationState(ctx); err != nil {
			logger.Error(err, "Failed to rehydrate managed Agent installation state")
		}
	}
	go newOperationTracker(d).run(ctx)
	logger.Info("DDA status worker initialized")
	if d.managedAgentInstallationIdentity.Configured() {
		go d.runManagedAgentInstallationIntentWorker(ctx)
		if err := d.installManagedAgentInstallationIntentForwarder(ctx); err != nil {
			return err
		}
		if err := d.installManagedAgentInstallationCredentialForwarder(ctx); err != nil {
			return err
		}
		d.requestManagedAgentInstallationRetry()
		logger.Info("Managed Agent installation intent worker initialized", "provider", d.managedAgentInstallationIdentity.Provider())
	}

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
	// Incoming-edge event: emitted before processing so the timeline shows
	// every task FA sent, including those that will be rejected below.
	d.emitTaskReceivedEvent(ctx, req)
	d.transitionMu.Lock()
	defer d.transitionMu.Unlock()
	d.taskMu.Lock()
	if d.managedAgentInstallationActive || d.managedAgentInstallationTaskReserved {
		err := &stateDoesntMatchError{msg: "a DatadogAgent managed Agent installation transition is already in progress"}
		d.taskMu.Unlock()
		d.emitTaskRejectedEvent(ctx, req.Params.NamespacedName, req, err.Error())
		return err
	}
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
		d.emitTaskRejectedEvent(ctx, req.Params.NamespacedName, req, err.Error())
		return err
	}
	// The request is not relevant (stop a terminated experiment) or the desired
	// state is already true.
	if pending == nil {
		// Nothing is left for the worker to wait on.
		d.setTaskState(req.Package, req.ID, pbgo.TaskState_DONE, nil)
		d.taskMu.Unlock()
		// Synthesize a pendingOperation for the event message so the
		// timeline shows both ends of this idempotent task (received +
		// completed). There is no in-flight op because the worker is
		// never engaged on this path.
		d.emitTaskCompletedEvent(ctx, pendingOperation{
			taskID: req.ID,
			intent: pendingIntent(methodLabel(req.Method)),
			nsn:    req.Params.NamespacedName,
		})
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
	logger.V(2).Info("Updated installer configs", "count", len(d.configs))
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

	if err := d.verifyExpectedState(req); err != nil {
		logger.Error(err, "Expected state mismatch")
		return nil, err
	}
	return d.dispatchRemoteAPIRequest(ctx, req)
}

func (d *Daemon) dispatchRemoteAPIRequest(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	switch req.Method {
	case methodStartDatadogAgentExperiment:
		if !d.revisionsEnabled {
			return nil, fmt.Errorf("experiment signals require the CreateControllerRevisions and DatadogAgentInternal feature gates")
		}
		return d.startDatadogAgentExperiment(ctx, req)
	case methodStopDatadogAgentExperiment:
		if !d.revisionsEnabled {
			return nil, fmt.Errorf("experiment signals require the CreateControllerRevisions and DatadogAgentInternal feature gates")
		}
		return d.stopDatadogAgentExperiment(ctx, req)
	case methodPromoteDatadogAgentExperiment:
		if !d.revisionsEnabled {
			return nil, fmt.Errorf("experiment signals require the CreateControllerRevisions and DatadogAgentInternal feature gates")
		}
		return d.promoteDatadogAgentExperiment(ctx, req)
	default:
		return nil, fmt.Errorf("unknown method: %s", req.Method)
	}
}

// setTaskState updates the Task field of the package entry in the RC installer state.
// If the package is not yet present in the state, a new entry is added.
// This is a no-op when rcClient is nil (e.g. in unit tests that construct Daemon directly).
func (d *Daemon) setTaskState(pkgName, taskID string, taskState pbgo.TaskState, taskErr error) {
	d.installerStateMu.Lock()
	defer d.installerStateMu.Unlock()

	if d.rcClient == nil {
		return
	}
	task := &pbgo.PackageStateTask{
		Id:    taskID,
		State: taskState,
	}
	if taskErr != nil {
		task.Error = &pbgo.TaskError{Message: boundedTaskErrorMessage(taskErr)}
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

// rehydrateInstallerState seeds the RC installer state from the
// current DatadogAgent objects on disk. The rcClient's installer state
// is in-memory only — after a daemon restart it would otherwise report
// no in-progress experiment even when status.experiment.Phase is still
// Running. That mismatch causes Fleet Automation to re-send the start
// task and also makes reconcileTimedOutExperiment a no-op (its guard
// `pkg.experimentConfigVersion == experiment.ID` never matches).
//
// Reads go through the API reader (not the cache) because the informer
// cache may not be populated yet at the moment Start runs.
func (d *Daemon) rehydrateInstallerState(ctx context.Context) error {
	if d.rcClient == nil {
		return nil
	}
	if !d.managedAgentInstallationIdentity.Configured() {
		return d.rehydrateLegacyInstallerState(ctx)
	}
	if d.apiReader == nil {
		return fmt.Errorf("API reader is required to rehydrate installer state")
	}
	logger := ctrl.LoggerFrom(ctx)
	ddas := &v2alpha1.DatadogAgentList{}
	if err := d.apiReader.List(ctx, ddas); err != nil {
		return fmt.Errorf("list DatadogAgents: %w", err)
	}
	target := managedAgentInstallationTarget
	for i := range ddas.Items {
		if client.ObjectKeyFromObject(&ddas.Items[i]) == target {
			ddas.Items = []v2alpha1.DatadogAgent{ddas.Items[i]}
			break
		}
	}
	if len(ddas.Items) != 1 || client.ObjectKeyFromObject(&ddas.Items[0]) != target {
		ddas.Items = nil
	}
	var fleetOwned *types.NamespacedName
	var unmanaged *types.NamespacedName
	rehydrated := false
	for i := range ddas.Items {
		dda := &ddas.Items[i]
		owned, err := classifyFleetDatadogAgentOwnershipForRehydration(dda)
		if err != nil {
			return err
		}
		if !owned {
			if unmanaged == nil {
				nsn := client.ObjectKeyFromObject(dda)
				unmanaged = &nsn
			}
			continue
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		nsn := client.ObjectKeyFromObject(dda)
		if fleetOwned != nil && *fleetOwned != nsn {
			return fmt.Errorf(
				"multiple Fleet-managed DatadogAgents found: %s/%s and %s/%s",
				fleetOwned.Namespace,
				fleetOwned.Name,
				nsn.Namespace,
				nsn.Name,
			)
		}
		fleetOwned = &nsn
	}
	if fleetOwned != nil && unmanaged != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf(
			"Fleet-managed DatadogAgent %s/%s coexists with unmanaged DatadogAgent %s/%s",
			fleetOwned.Namespace,
			fleetOwned.Name,
			unmanaged.Namespace,
			unmanaged.Name,
		)}
	}
	if fleetOwned == nil && unmanaged != nil {
		d.setPackageConfigVersions(packageDatadogOperator, fleetUnmanagedConfigVersion, "")
		logger.Info("Rehydrated installer state with unmanaged DatadogAgent",
			"namespace", unmanaged.Namespace,
			"name", unmanaged.Name,
		)
		return nil
	}
	if fleetOwned == nil {
		var activeExperiment *types.NamespacedName
		for i := range ddas.Items {
			dda := &ddas.Items[i]
			exp := dda.Status.Experiment
			if exp == nil || exp.ID == "" || isTerminalPhase(exp.Phase) {
				continue
			}
			nsn := client.ObjectKeyFromObject(dda)
			if activeExperiment != nil && *activeExperiment != nsn {
				return fmt.Errorf(
					"multiple DatadogAgents with active Fleet experiments found: %s/%s and %s/%s",
					activeExperiment.Namespace,
					activeExperiment.Name,
					nsn.Namespace,
					nsn.Name,
				)
			}
			activeExperiment = &nsn
		}
	}
	for i := range ddas.Items {
		dda := &ddas.Items[i]
		if fleetOwned != nil && client.ObjectKeyFromObject(dda) != *fleetOwned {
			continue
		}
		managedAgentInstallationConfigID := ""
		if dda.Labels[fleetManagedByLabel] == fleetManagedByValue && dda.Labels[fleetConfigIDLabel] != "" {
			managedAgentInstallationConfigID = dda.Labels[fleetConfigIDLabel]
		}
		stable := managedAgentInstallationConfigID
		experiment := ""
		experimentPhase := v2alpha1.ExperimentPhase("")
		exp := dda.Status.Experiment
		if managedAgentInstallationConfigID != "" {
			if dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady || dda.Annotations[fleetConfigHashAnnotation] == "" {
				stable = fleetPartialConfigVersionPrefix + managedAgentInstallationConfigID
			}
		}
		if exp != nil && exp.ID != "" && !isTerminalPhase(exp.Phase) {
			experiment = exp.ID
			experimentPhase = exp.Phase
		} else if exp != nil && exp.ID != "" && exp.Phase == v2alpha1.ExperimentPhasePromoted && managedAgentInstallationConfigID != "" && managedAgentInstallationConfigID != exp.ID {
			experiment = exp.ID
			experimentPhase = exp.Phase
		}
		if stable == "" && experiment == "" {
			continue
		}
		if stable == "" {
			stable, _ = d.getPackageConfigVersions(packageDatadogOperator)
			if stable == "empty" || stable == remoteconfig.InstallerStateUnknownConfigVersion {
				stable = ""
			}
		}
		d.setPackageConfigVersions(packageDatadogOperator, stable, experiment)
		rehydrated = true
		logger.Info("Rehydrated installer state from DatadogAgent",
			"namespace", dda.Namespace,
			"name", dda.Name,
			"stableConfigVersion", stable,
			"experimentConfigVersion", experiment,
		)
		if experiment == "" {
			continue
		}
		// Pass dda directly — apiReader returned a fully-populated object,
		// and the informer cache may not be synced yet at this point so
		// a cache-backed lookup would silently drop this event.
		d.emitInstallerStateRehydratedEvent(ctx, dda, experiment, experimentPhase)
	}
	if !rehydrated {
		d.setPackageConfigVersions(packageDatadogOperator, "", "")
		logger.Info("Rehydrated installer state with no Fleet-managed DatadogAgent")
	}
	return nil
}

func (d *Daemon) rehydrateLegacyInstallerState(ctx context.Context) error {
	if d.apiReader == nil {
		return nil
	}
	logger := ctrl.LoggerFrom(ctx)
	ddas := &v2alpha1.DatadogAgentList{}
	if err := d.apiReader.List(ctx, ddas); err != nil {
		return fmt.Errorf("list DatadogAgents: %w", err)
	}
	for i := range ddas.Items {
		dda := &ddas.Items[i]
		experiment := dda.Status.Experiment
		if experiment == nil || experiment.ID == "" || isTerminalPhase(experiment.Phase) {
			continue
		}
		stable, _ := d.getPackageConfigVersions(packageDatadogOperator)
		d.setPackageConfigVersions(packageDatadogOperator, stable, experiment.ID)
		logger.Info("Rehydrated installer state from DatadogAgent",
			"namespace", dda.Namespace,
			"name", dda.Name,
			"experimentID", experiment.ID,
			"phase", experiment.Phase,
		)
		d.emitInstallerStateRehydratedEvent(ctx, dda, experiment.ID, experiment.Phase)
	}
	return nil
}

// packageDatadogOperator is the RC package name the daemon reports for itself.
const packageDatadogOperator = "datadog-operator"

const fleetUnmanagedConfigVersion = "unmanaged"

// getPackageConfigVersions returns the current stable and experiment config versions
// for the given package from the RC installer state.
func (d *Daemon) getPackageConfigVersions(pkgName string) (stable, experiment string) {
	d.installerStateMu.Lock()
	defer d.installerStateMu.Unlock()

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
	d.installerStateMu.Lock()
	defer d.installerStateMu.Unlock()

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
