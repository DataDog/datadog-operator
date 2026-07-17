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

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

type managedAgentInstallationAction string

const (
	managedAgentInstallationActionInstall           managedAgentInstallationAction = "install"
	managedAgentInstallationActionUninstall         managedAgentInstallationAction = "uninstall"
	managedAgentInstallationActionVerifyUninstalled managedAgentInstallationAction = "verify-uninstalled"
	managedAgentInstallationActionClearFence        managedAgentInstallationAction = "clear-uninstall-fence"
)

type managedAgentInstallationMetadata struct {
	Digest                  string
	Provider                remoteconfig.ManagedAgentInstallationProvider
	TargetID                string
	Bootstrap               managedAgentInstallationBootstrap
	AcknowledgedOperationID string
}

type managedAgentInstallationCommand struct {
	TaskID         string
	OperationID    string
	InstallationID string
	ConfigID       string
	Action         managedAgentInstallationAction
	Config         json.RawMessage
	Metadata       *managedAgentInstallationMetadata
}

func (c managedAgentInstallationCommand) instrumenterManaged() bool {
	return c.Metadata != nil
}

func (c managedAgentInstallationCommand) operation() (Operation, error) {
	return c.Action.operation()
}

func (a managedAgentInstallationAction) operation() (Operation, error) {
	switch a {
	case managedAgentInstallationActionInstall:
		return OperationCreate, nil
	case managedAgentInstallationActionUninstall, managedAgentInstallationActionVerifyUninstalled, managedAgentInstallationActionClearFence:
		return OperationDelete, nil
	default:
		return "", fmt.Errorf("unknown managed Agent installation action %q", a)
	}
}

func (c managedAgentInstallationCommand) desiredState() (managedAgentInstallationDesiredState, error) {
	switch c.Action {
	case managedAgentInstallationActionInstall:
		return managedAgentInstallationDesiredStateInstalled, nil
	case managedAgentInstallationActionUninstall:
		return managedAgentInstallationDesiredStateAbsent, nil
	default:
		return "", fmt.Errorf("managed Agent installation action %q has no desired state", c.Action)
	}
}

func newManagedAgentInstallationCommand(intent managedAgentInstallationIntent, config json.RawMessage, digest string) managedAgentInstallationCommand {
	action := managedAgentInstallationActionInstall
	if intent.DesiredState == managedAgentInstallationDesiredStateAbsent {
		action = managedAgentInstallationActionUninstall
	}
	return managedAgentInstallationCommand{
		TaskID:         intent.OperationID,
		OperationID:    intent.OperationID,
		InstallationID: intent.InstallationID,
		ConfigID:       intent.OperationID,
		Action:         action,
		Config:         config,
		Metadata: &managedAgentInstallationMetadata{
			Digest:                  digest,
			Provider:                intent.Provider,
			TargetID:                intent.TargetID,
			Bootstrap:               intent.Bootstrap,
			AcknowledgedOperationID: intent.AcknowledgedOperationID,
		},
	}
}

func (d *Daemon) managedAgentInstallationCommandFromRemoteRequest(req remoteAPIRequest) (managedAgentInstallationCommand, error) {
	if d.managedAgentInstallationEnabled {
		return managedAgentInstallationCommand{}, &stateDoesntMatchError{msg: "managed Agent installation tasks must originate from the local managed Agent installation intent adapter"}
	}
	if err := d.validateRemoteManagedAgentInstallationRequest(req); err != nil {
		return managedAgentInstallationCommand{}, err
	}
	action, err := managedAgentInstallationActionFromMethod(req.Method)
	if err != nil {
		return managedAgentInstallationCommand{}, err
	}
	expectedOperation, err := action.operation()
	if err != nil {
		return managedAgentInstallationCommand{}, err
	}
	resolved, err := d.resolveRemoteManagedAgentInstallationOperation(req, expectedOperation)
	if err != nil {
		return managedAgentInstallationCommand{}, err
	}
	return managedAgentInstallationCommand{
		TaskID:         req.ID,
		OperationID:    req.Params.OperationID,
		InstallationID: req.Params.InstallationID,
		ConfigID:       req.Params.Version,
		Action:         action,
		Config:         resolved.Config,
	}, nil
}

func (d *Daemon) validateRemoteManagedAgentInstallationRequest(req remoteAPIRequest) error {
	if req.ID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation task ID is required"}
	}
	if req.Params.OperationID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation operation ID is required"}
	}
	if !d.managedAgentInstallationIdentity.Configured() {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation identity is not configured"}
	}
	if err := d.managedAgentInstallationIdentity.Validate(); err != nil {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation identity is invalid"}
	}
	if d.rcClient == nil || d.rcClient.GetClientID() == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation local RC client ID is unavailable"}
	}
	if req.ExpectedState.ClientID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation expected RC client ID is required"}
	}
	if req.ExpectedState.ClientID != d.rcClient.GetClientID() {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation expected RC client ID does not match the local client"}
	}
	if req.Params.InstallationID != d.managedAgentInstallationIdentity.InstallationID() {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation ID does not match the local managed installation configuration"}
	}
	if err := d.verifyExpectedState(req); err != nil {
		return err
	}
	if req.ExpectedState.ExperimentConfig != "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation transitions require no active experiment config"}
	}
	return nil
}

func (d *Daemon) validateManagedAgentInstallationCommand(command managedAgentInstallationCommand) error {
	if command.TaskID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation task ID is required"}
	}
	if command.OperationID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation operation ID is required"}
	}
	if command.ConfigID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation config ID is required"}
	}
	if !d.managedAgentInstallationIdentity.Configured() || d.managedAgentInstallationIdentity.Validate() != nil {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation identity is invalid"}
	}
	if command.InstallationID != d.managedAgentInstallationIdentity.InstallationID() {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation ID does not match the local managed installation configuration"}
	}
	if _, err := command.operation(); err != nil {
		return err
	}
	if command.instrumenterManaged() {
		if command.Action != managedAgentInstallationActionInstall && command.Action != managedAgentInstallationActionUninstall {
			return &stateDoesntMatchError{msg: "local managed Agent installation intent requested an unsupported action"}
		}
		if command.Metadata.Provider != d.managedAgentInstallationIdentity.Provider() ||
			command.Metadata.TargetID != d.managedAgentInstallationIdentity.TargetID() {
			return &stateDoesntMatchError{msg: "managed Agent installation command belongs to a different provider target"}
		}
	}
	_, experimentConfig := d.getPackageConfigVersions(packageDatadogOperator)
	if experimentConfig != "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation transitions require no active experiment config"}
	}
	return nil
}

func (d *Daemon) handleRemoteManagedAgentInstallationTask(ctx context.Context, req remoteAPIRequest) error {
	command, err := d.managedAgentInstallationCommandFromRemoteRequest(req)
	if err != nil {
		d.taskMu.Lock()
		d.setTaskState(req.Package, req.ID, pbgo.TaskState_INVALID_STATE, err)
		d.taskMu.Unlock()
		d.emitTaskRejectedEvent(ctx, req.Params.NamespacedName, req, err.Error())
		return err
	}
	return d.handleManagedAgentInstallationCommand(ctx, command)
}

func (d *Daemon) handleManagedAgentInstallationCommand(ctx context.Context, command managedAgentInstallationCommand) error {
	d.transitionMu.Lock()
	d.taskMu.Lock()
	if d.managedAgentInstallationActive {
		err := &stateDoesntMatchError{msg: "a DatadogAgent managed Agent installation transition is already in progress"}
		d.taskMu.Unlock()
		d.transitionMu.Unlock()
		d.emitManagedAgentInstallationTaskRejectedEvent(ctx, command, err.Error())
		return err
	}
	if err := d.validateManagedAgentInstallationCommand(command); err != nil {
		d.taskMu.Unlock()
		d.transitionMu.Unlock()
		if persistErr := d.recordManagedAgentInstallationResult(ctx, command, pbgo.TaskState_INVALID_STATE, err); persistErr != nil {
			d.requestManagedAgentInstallationRetry()
			return errors.Join(err, persistErr)
		}
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, command.TaskID, pbgo.TaskState_INVALID_STATE, err)
		d.taskMu.Unlock()
		d.emitManagedAgentInstallationTaskRejectedEvent(ctx, command, err.Error())
		return err
	}
	taskCtx, cancel := context.WithCancel(ctx)
	d.managedAgentInstallationActive = true
	d.managedAgentInstallationOperationID = command.OperationID
	d.managedAgentInstallationCancel = cancel
	d.managedAgentInstallationDone = make(chan struct{})
	d.setTaskState(packageDatadogOperator, command.TaskID, pbgo.TaskState_RUNNING, nil)
	d.taskMu.Unlock()
	d.transitionMu.Unlock()

	if command.instrumenterManaged() {
		if err := d.writeManagedAgentInstallationState(taskCtx, managedAgentInstallationStateFromCommand(command, pbgo.TaskState_RUNNING, nil)); err != nil {
			d.finishManagedAgentInstallationTask(command.OperationID)
			return fmt.Errorf("persist accepted managed Agent installation intent: %w", err)
		}
		d.taskMu.Lock()
		d.managedAgentInstallationTaskReserved = true
		d.taskMu.Unlock()
	}
	if d.managedAgentInstallationTaskRunner == nil {
		return d.executeManagedAgentInstallationCommand(taskCtx, command)
	}
	d.managedAgentInstallationTaskRunner(func() {
		_ = d.executeManagedAgentInstallationCommand(taskCtx, command)
	})
	return nil
}

func (d *Daemon) executeManagedAgentInstallationCommand(ctx context.Context, command managedAgentInstallationCommand) error {
	d.transitionMu.Lock()
	defer d.transitionMu.Unlock()
	defer d.finishManagedAgentInstallationTask(command.OperationID)

	pending, err := d.dispatchManagedAgentInstallationCommand(ctx, command)
	if err != nil {
		var stateErr *stateDoesntMatchError
		resultState := pbgo.TaskState_ERROR
		if errors.As(err, &stateErr) {
			resultState = pbgo.TaskState_INVALID_STATE
		}
		if persistErr := d.recordManagedAgentInstallationResult(ctx, command, resultState, err); persistErr != nil {
			d.requestManagedAgentInstallationRetry()
			return errors.Join(err, persistErr)
		}
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, command.TaskID, resultState, err)
		d.taskMu.Unlock()
		d.emitManagedAgentInstallationTaskRejectedEvent(ctx, command, err.Error())
		return err
	}
	if pending != nil {
		err = fmt.Errorf("managed Agent installation action %s returned an asynchronous operation", command.Action)
		if persistErr := d.recordManagedAgentInstallationResult(ctx, command, pbgo.TaskState_ERROR, err); persistErr != nil {
			d.requestManagedAgentInstallationRetry()
			return errors.Join(err, persistErr)
		}
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, command.TaskID, pbgo.TaskState_ERROR, err)
		d.taskMu.Unlock()
		d.emitManagedAgentInstallationTaskRejectedEvent(ctx, command, err.Error())
		return err
	}

	if err := d.recordManagedAgentInstallationResult(ctx, command, pbgo.TaskState_DONE, nil); err != nil {
		d.requestManagedAgentInstallationRetry()
		return err
	}
	d.taskMu.Lock()
	switch command.Action {
	case managedAgentInstallationActionInstall:
		d.setPackageConfigVersions(packageDatadogOperator, command.ConfigID, "")
	case managedAgentInstallationActionUninstall:
		d.setPackageConfigVersions(packageDatadogOperator, "", "")
	}
	d.setTaskState(packageDatadogOperator, command.TaskID, pbgo.TaskState_DONE, nil)
	d.taskMu.Unlock()
	d.emitTaskCompletedEvent(ctx, pendingOperation{
		taskID: command.TaskID,
		intent: pendingIntent(command.Action),
		nsn:    managedAgentInstallationTarget,
	})
	return nil
}

func (d *Daemon) finishManagedAgentInstallationTask(operationID string) {
	d.taskMu.Lock()
	defer d.taskMu.Unlock()
	if d.managedAgentInstallationOperationID != operationID {
		return
	}
	d.managedAgentInstallationActive = false
	d.managedAgentInstallationOperationID = ""
	if d.managedAgentInstallationCancel != nil {
		d.managedAgentInstallationCancel()
		d.managedAgentInstallationCancel = nil
	}
	if d.managedAgentInstallationDone != nil {
		close(d.managedAgentInstallationDone)
		d.managedAgentInstallationDone = nil
	}
}

func (d *Daemon) dispatchManagedAgentInstallationCommand(ctx context.Context, command managedAgentInstallationCommand) (*pendingOperation, error) {
	switch command.Action {
	case managedAgentInstallationActionInstall:
		if err := d.ensureUninstallFenceInactive(ctx); err != nil {
			return nil, err
		}
		return d.installDatadogAgent(ctx, command)
	case managedAgentInstallationActionUninstall:
		return d.uninstallDatadogAgent(ctx, command)
	case managedAgentInstallationActionVerifyUninstalled:
		return d.verifyDatadogAgentUninstalled(ctx, command)
	case managedAgentInstallationActionClearFence:
		return d.clearDatadogAgentUninstallFence(ctx, command)
	default:
		return nil, fmt.Errorf("unknown managed Agent installation action %q", command.Action)
	}
}

func managedAgentInstallationActionFromMethod(method string) (managedAgentInstallationAction, error) {
	switch method {
	case methodInstallDatadogAgent:
		return managedAgentInstallationActionInstall, nil
	case methodUninstallDatadogAgent:
		return managedAgentInstallationActionUninstall, nil
	case methodVerifyDatadogAgentUninstalled:
		return managedAgentInstallationActionVerifyUninstalled, nil
	case methodClearDatadogAgentUninstallFence:
		return managedAgentInstallationActionClearFence, nil
	default:
		return "", fmt.Errorf("method %q is not a DatadogAgent managed Agent installation mutation", method)
	}
}

func isManagedAgentInstallationMethod(method string) bool {
	_, err := managedAgentInstallationActionFromMethod(method)
	return err == nil
}

var managedAgentInstallationTarget = client.ObjectKey{
	Namespace: fleetDatadogAgentNamespace,
	Name:      fleetDatadogAgentName,
}
