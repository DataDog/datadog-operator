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
)

type managedAgentInstallationCommand struct {
	Intent managedAgentInstallationIntent
	Config json.RawMessage
	Digest string
}

func newManagedAgentInstallationCommand(intent managedAgentInstallationIntent, config json.RawMessage, digest string) managedAgentInstallationCommand {
	return managedAgentInstallationCommand{
		Intent: intent,
		Config: config,
		Digest: digest,
	}
}

func (d *Daemon) validateManagedAgentInstallationCommand(command managedAgentInstallationCommand) error {
	if command.Intent.OperationID == "" {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation operation ID is required"}
	}
	if !d.managedAgentInstallationIdentity.Configured() || d.managedAgentInstallationIdentity.Validate() != nil {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation identity is invalid"}
	}
	if command.Intent.InstallationID != d.managedAgentInstallationIdentity.InstallationID() {
		return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation ID does not match the local managed installation configuration"}
	}
	if command.Intent.Provider != d.managedAgentInstallationIdentity.Provider() ||
		command.Intent.TargetID != d.managedAgentInstallationIdentity.TargetID() {
		return &stateDoesntMatchError{msg: "managed Agent installation command belongs to a different provider target"}
	}
	if command.Intent.DesiredState != managedAgentInstallationDesiredStateInstalled &&
		command.Intent.DesiredState != managedAgentInstallationDesiredStateAbsent {
		return &stateDoesntMatchError{msg: "local managed Agent installation intent requested an unsupported desired state"}
	}
	if command.Intent.DesiredState == managedAgentInstallationDesiredStateInstalled {
		_, experimentConfig := d.getPackageConfigVersions(packageDatadogOperator)
		if experimentConfig != "" {
			return &stateDoesntMatchError{msg: "DatadogAgent managed Agent installation requires no active experiment config"}
		}
	}
	return nil
}

func (d *Daemon) handleManagedAgentInstallationCommand(ctx context.Context, command managedAgentInstallationCommand) error {
	d.transitionMu.Lock()
	d.taskMu.Lock()
	if d.managedAgentInstallationActive {
		err := &stateDoesntMatchError{msg: "a DatadogAgent managed Agent installation transition is already in progress"}
		d.taskMu.Unlock()
		d.transitionMu.Unlock()
		d.emitManagedAgentInstallationRejectedEvent(ctx, command, err.Error())
		return err
	}
	if err := d.validateManagedAgentInstallationCommand(command); err != nil {
		d.taskMu.Unlock()
		d.transitionMu.Unlock()
		if persistErr := d.recordManagedAgentInstallationResult(ctx, command, pbgo.TaskState_INVALID_STATE, err); persistErr != nil {
			d.requestManagedAgentInstallationRetryAfter()
			return errors.Join(err, persistErr)
		}
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, command.Intent.OperationID, pbgo.TaskState_INVALID_STATE, err)
		d.taskMu.Unlock()
		d.emitManagedAgentInstallationRejectedEvent(ctx, command, err.Error())
		return err
	}
	taskCtx, cancel := context.WithCancel(ctx)
	d.managedAgentInstallationActive = true
	d.managedAgentInstallationTaskReserved = true
	d.managedAgentInstallationOperationID = command.Intent.OperationID
	d.managedAgentInstallationCancel = cancel
	d.managedAgentInstallationDone = make(chan struct{})
	d.setTaskState(packageDatadogOperator, command.Intent.OperationID, pbgo.TaskState_RUNNING, nil)
	d.taskMu.Unlock()
	d.transitionMu.Unlock()

	if err := d.writeManagedAgentInstallationState(taskCtx, managedAgentInstallationStateFromCommand(command, pbgo.TaskState_RUNNING, nil)); err != nil {
		d.finishManagedAgentInstallationTask(command.Intent.OperationID)
		return fmt.Errorf("persist accepted managed Agent installation intent: %w", err)
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
	defer d.finishManagedAgentInstallationTask(command.Intent.OperationID)

	pending, err := d.dispatchManagedAgentInstallationCommand(ctx, command)
	if err != nil {
		var stateErr *stateDoesntMatchError
		if !errors.As(err, &stateErr) && ctx.Err() == nil && isRetryable(err) {
			if persistErr := d.recordManagedAgentInstallationResult(ctx, command, pbgo.TaskState_RUNNING, err); persistErr != nil {
				d.requestManagedAgentInstallationRetryAfter()
				return errors.Join(err, persistErr)
			}
			d.requestManagedAgentInstallationRetryAfter()
			return err
		}
		resultState := pbgo.TaskState_ERROR
		if stateErr != nil {
			resultState = pbgo.TaskState_INVALID_STATE
		}
		if persistErr := d.recordManagedAgentInstallationResult(ctx, command, resultState, err); persistErr != nil {
			d.requestManagedAgentInstallationRetryAfter()
			return errors.Join(err, persistErr)
		}
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, command.Intent.OperationID, resultState, err)
		d.taskMu.Unlock()
		d.emitManagedAgentInstallationRejectedEvent(ctx, command, err.Error())
		return err
	}
	if pending != nil {
		err = fmt.Errorf("managed Agent installation desired state %s returned an asynchronous operation", command.Intent.DesiredState)
		if persistErr := d.recordManagedAgentInstallationResult(ctx, command, pbgo.TaskState_ERROR, err); persistErr != nil {
			d.requestManagedAgentInstallationRetryAfter()
			return errors.Join(err, persistErr)
		}
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, command.Intent.OperationID, pbgo.TaskState_ERROR, err)
		d.taskMu.Unlock()
		d.emitManagedAgentInstallationRejectedEvent(ctx, command, err.Error())
		return err
	}

	if err := d.recordManagedAgentInstallationResult(ctx, command, pbgo.TaskState_DONE, nil); err != nil {
		d.requestManagedAgentInstallationRetryAfter()
		return err
	}
	d.taskMu.Lock()
	switch command.Intent.DesiredState {
	case managedAgentInstallationDesiredStateInstalled:
		d.setPackageConfigVersions(packageDatadogOperator, command.Intent.OperationID, "")
	case managedAgentInstallationDesiredStateAbsent:
		d.setPackageConfigVersions(packageDatadogOperator, "", "")
	}
	d.setTaskState(packageDatadogOperator, command.Intent.OperationID, pbgo.TaskState_DONE, nil)
	d.taskMu.Unlock()
	d.emitManagedAgentInstallationCompletedEvent(ctx, command)
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
	switch command.Intent.DesiredState {
	case managedAgentInstallationDesiredStateInstalled:
		return d.installDatadogAgent(ctx, command)
	case managedAgentInstallationDesiredStateAbsent:
		return d.uninstallDatadogAgent(ctx)
	default:
		return nil, fmt.Errorf("unknown managed Agent installation desired state %q", command.Intent.DesiredState)
	}
}

var managedAgentInstallationTarget = client.ObjectKey{
	Namespace: fleetDatadogAgentNamespace,
	Name:      fleetDatadogAgentName,
}
