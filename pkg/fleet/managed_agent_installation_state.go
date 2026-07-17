// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

const (
	managedAgentInstallationStateConfigMapName = "datadog-agent-managed-installation-state"

	managedAgentInstallationStateProviderKey       = "provider"
	managedAgentInstallationStateInstallationIDKey = "installation_id"
	managedAgentInstallationStateTargetIDKey       = "target_id"
	managedAgentInstallationStateOperationIDKey    = "operation_id"
	managedAgentInstallationStateDigestKey         = "intent_digest"
	managedAgentInstallationStateDesiredStateKey   = "desired_state"
	managedAgentInstallationStateBootstrapKey      = "bootstrap"
	managedAgentInstallationStateAcknowledgedKey   = "acknowledged_operation_id"
	managedAgentInstallationStateConfigIDKey       = "config_id"
	managedAgentInstallationStateTaskStateKey      = "task_state"
	managedAgentInstallationStateErrorKey          = "error"
)

var (
	managedAgentInstallationStateKey = types.NamespacedName{
		Namespace: fleetDatadogAgentNamespace,
		Name:      managedAgentInstallationStateConfigMapName,
	}
	managedAgentInstallationFenceMonitorInterval = 10 * time.Second
)

type managedAgentInstallationPersistedState struct {
	Provider                remoteconfig.ManagedAgentInstallationProvider
	InstallationID          string
	TargetID                string
	OperationID             string
	Digest                  string
	DesiredState            managedAgentInstallationDesiredState
	Bootstrap               managedAgentInstallationBootstrap
	AcknowledgedOperationID string
	ConfigID                string
	TaskState               pbgo.TaskState
	Error                   string
}

func ManagedAgentInstallationReadinessTags(ctx context.Context, reader client.Reader, identity remoteconfig.ManagedAgentInstallationIdentity) ([]string, error) {
	if reader == nil || !identity.Configured() {
		return nil, nil
	}
	intentConfigMap := &corev1.ConfigMap{}
	if err := reader.Get(ctx, managedAgentInstallationIntentKey, intentConfigMap); err != nil {
		return nil, fmt.Errorf("read managed Agent installation intent for Remote Configuration updater tags: %w", err)
	}
	intent, _, _, err := decodeManagedAgentInstallationIntent([]byte(intentConfigMap.Data[managedAgentInstallationIntentDataKey]), identity)
	if err != nil {
		return nil, err
	}
	if intent.DesiredState != managedAgentInstallationDesiredStateInstalled || intent.AcknowledgedOperationID == "" || intent.OperationID != intent.AcknowledgedOperationID {
		return nil, nil
	}

	daemon := &Daemon{
		apiReader:                        reader,
		managedAgentInstallationIdentity: identity,
	}
	state, err := daemon.readManagedAgentInstallationState(ctx)
	if err != nil {
		return nil, fmt.Errorf("read managed Agent installation acknowledgement state for Remote Configuration updater tags: %w", err)
	}
	if state == nil ||
		state.Provider != intent.Provider ||
		state.InstallationID != intent.InstallationID ||
		state.TargetID != intent.TargetID ||
		state.OperationID != intent.AcknowledgedOperationID ||
		state.AcknowledgedOperationID != intent.AcknowledgedOperationID ||
		state.DesiredState != managedAgentInstallationDesiredStateInstalled ||
		state.TaskState != pbgo.TaskState_DONE {
		return nil, fmt.Errorf("managed Agent installation acknowledgement state is not yet consistent with the acknowledged install intent")
	}
	return []string{
		"managed_agent_installation_ack:" + intent.AcknowledgedOperationID,
		"operator_config_updates:ready",
	}, nil
}

func managedAgentInstallationStateFromCommand(command managedAgentInstallationCommand, taskState pbgo.TaskState, taskErr error) managedAgentInstallationPersistedState {
	desiredState, _ := command.desiredState()
	state := managedAgentInstallationPersistedState{
		Provider:                command.Metadata.Provider,
		InstallationID:          command.InstallationID,
		TargetID:                command.Metadata.TargetID,
		OperationID:             command.OperationID,
		Digest:                  command.Metadata.Digest,
		DesiredState:            desiredState,
		Bootstrap:               command.Metadata.Bootstrap,
		AcknowledgedOperationID: command.Metadata.AcknowledgedOperationID,
		ConfigID:                command.ConfigID,
		TaskState:               taskState,
	}
	if taskErr != nil {
		state.Error = boundedTaskErrorMessage(taskErr)
	}
	return state
}

func (d *Daemon) recordManagedAgentInstallationResult(ctx context.Context, command managedAgentInstallationCommand, taskState pbgo.TaskState, taskErr error) error {
	if !command.instrumenterManaged() {
		return nil
	}
	if err := d.writeManagedAgentInstallationResult(ctx, managedAgentInstallationStateFromCommand(command, taskState, taskErr)); err != nil {
		return fmt.Errorf("persist managed Agent installation result for operation %s: %w", command.OperationID, err)
	}
	return nil
}

func (d *Daemon) writeManagedAgentInstallationState(ctx context.Context, state managedAgentInstallationPersistedState) error {
	return d.writeManagedAgentInstallationStateForOperation(ctx, state, "")
}

func (d *Daemon) writeManagedAgentInstallationResult(ctx context.Context, state managedAgentInstallationPersistedState) error {
	return d.writeManagedAgentInstallationStateForOperation(ctx, state, state.OperationID)
}

func (d *Daemon) writeManagedAgentInstallationStateForOperation(ctx context.Context, state managedAgentInstallationPersistedState, expectedOperationID string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		current := &corev1.ConfigMap{}
		getErr := d.client.Get(ctx, managedAgentInstallationStateKey, current)
		if apierrors.IsNotFound(getErr) {
			if expectedOperationID != "" {
				return fmt.Errorf("managed Agent installation state is missing for operation %s", expectedOperationID)
			}
			intent := &corev1.ConfigMap{}
			if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationIntentKey, intent); err != nil {
				return fmt.Errorf("read managed Agent installation intent owner: %w", err)
			}
			return d.client.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: managedAgentInstallationStateKey.Namespace,
					Name:      managedAgentInstallationStateKey.Name,
					OwnerReferences: []metav1.OwnerReference{controllerOwnerReference(
						corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID,
					)},
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "datadog-operator",
					},
				},
				Data: managedAgentInstallationStateData(state),
			}, client.FieldOwner("fleet-daemon"))
		}
		if getErr != nil {
			return getErr
		}
		if err := d.validateManagedAgentInstallationStateOwner(ctx, current); err != nil {
			return err
		}
		if expectedOperationID != "" && current.Data[managedAgentInstallationStateOperationIDKey] != expectedOperationID {
			return nil
		}
		base := current.DeepCopy()
		current.Data = managedAgentInstallationStateData(state)
		return d.client.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}), client.FieldOwner("fleet-daemon"))
	})
}

func managedAgentInstallationStateData(state managedAgentInstallationPersistedState) map[string]string {
	bootstrap, _ := json.Marshal(state.Bootstrap)
	return map[string]string{
		managedAgentInstallationStateProviderKey:       string(state.Provider),
		managedAgentInstallationStateInstallationIDKey: state.InstallationID,
		managedAgentInstallationStateTargetIDKey:       state.TargetID,
		managedAgentInstallationStateOperationIDKey:    state.OperationID,
		managedAgentInstallationStateDigestKey:         state.Digest,
		managedAgentInstallationStateDesiredStateKey:   string(state.DesiredState),
		managedAgentInstallationStateBootstrapKey:      string(bootstrap),
		managedAgentInstallationStateAcknowledgedKey:   state.AcknowledgedOperationID,
		managedAgentInstallationStateConfigIDKey:       state.ConfigID,
		managedAgentInstallationStateTaskStateKey:      state.TaskState.String(),
		managedAgentInstallationStateErrorKey:          state.Error,
	}
}

func (d *Daemon) readManagedAgentInstallationState(ctx context.Context) (*managedAgentInstallationPersistedState, error) {
	configMap := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationStateKey, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read managed Agent installation state: %w", err)
	}
	if err := d.validateManagedAgentInstallationStateOwner(ctx, configMap); err != nil {
		return nil, err
	}
	taskState, err := parseManagedAgentInstallationTaskState(configMap.Data[managedAgentInstallationStateTaskStateKey])
	if err != nil {
		return nil, err
	}
	var bootstrap managedAgentInstallationBootstrap
	if err := json.Unmarshal([]byte(configMap.Data[managedAgentInstallationStateBootstrapKey]), &bootstrap); err != nil {
		return nil, fmt.Errorf("managed Agent installation state has invalid bootstrap: %w", err)
	}
	state := &managedAgentInstallationPersistedState{
		Provider:                remoteconfig.ManagedAgentInstallationProvider(configMap.Data[managedAgentInstallationStateProviderKey]),
		InstallationID:          configMap.Data[managedAgentInstallationStateInstallationIDKey],
		TargetID:                configMap.Data[managedAgentInstallationStateTargetIDKey],
		OperationID:             configMap.Data[managedAgentInstallationStateOperationIDKey],
		Digest:                  configMap.Data[managedAgentInstallationStateDigestKey],
		DesiredState:            managedAgentInstallationDesiredState(configMap.Data[managedAgentInstallationStateDesiredStateKey]),
		Bootstrap:               bootstrap,
		AcknowledgedOperationID: configMap.Data[managedAgentInstallationStateAcknowledgedKey],
		ConfigID:                configMap.Data[managedAgentInstallationStateConfigIDKey],
		TaskState:               taskState,
		Error:                   configMap.Data[managedAgentInstallationStateErrorKey],
	}
	if state.Provider == "" || state.InstallationID == "" || state.TargetID == "" || state.OperationID == "" || state.Digest == "" || state.ConfigID == "" {
		return nil, fmt.Errorf("managed Agent installation state is incomplete")
	}
	if err := validateManagedAgentInstallationBootstrap(state.Bootstrap); err != nil {
		return nil, err
	}
	if state.DesiredState != managedAgentInstallationDesiredStateInstalled && state.DesiredState != managedAgentInstallationDesiredStateAbsent {
		return nil, fmt.Errorf("managed Agent installation state has unsupported desired state %q", state.DesiredState)
	}
	return state, nil
}

func (d *Daemon) validateManagedAgentInstallationStateOwner(ctx context.Context, state *corev1.ConfigMap) error {
	intent := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationIntentKey, intent); err != nil {
		return fmt.Errorf("read managed Agent installation intent owner: %w", err)
	}
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID)
	if err := requireManagedAgentInstallationResourceOwner(state.OwnerReferences, wantOwner); err != nil {
		return fmt.Errorf("validate managed Agent installation state ownership: %w", err)
	}
	return nil
}

func parseManagedAgentInstallationTaskState(value string) (pbgo.TaskState, error) {
	switch value {
	case pbgo.TaskState_RUNNING.String():
		return pbgo.TaskState_RUNNING, nil
	case pbgo.TaskState_DONE.String():
		return pbgo.TaskState_DONE, nil
	case pbgo.TaskState_ERROR.String():
		return pbgo.TaskState_ERROR, nil
	case pbgo.TaskState_INVALID_STATE.String():
		return pbgo.TaskState_INVALID_STATE, nil
	default:
		return pbgo.TaskState(0), fmt.Errorf("managed Agent installation state has unsupported task state %q", value)
	}
}

func (d *Daemon) rehydrateManagedAgentInstallationState(ctx context.Context) error {
	state, err := d.readManagedAgentInstallationState(ctx)
	if err != nil || state == nil {
		return err
	}
	if state.Provider != d.managedAgentInstallationIdentity.Provider() || state.InstallationID != d.managedAgentInstallationIdentity.InstallationID() || state.TargetID != d.managedAgentInstallationIdentity.TargetID() {
		return fmt.Errorf("persisted managed Agent installation state belongs to a different installation")
	}
	var taskErr error
	if state.Error != "" {
		taskErr = fmt.Errorf("%s", state.Error)
	}
	d.taskMu.Lock()
	d.managedAgentInstallationTaskReserved = !(state.AcknowledgedOperationID == state.OperationID && state.DesiredState == managedAgentInstallationDesiredStateInstalled)
	d.setTaskState(packageDatadogOperator, state.OperationID, state.TaskState, taskErr)
	d.taskMu.Unlock()
	return nil
}

func (d *Daemon) runManagedAgentInstallationFenceMonitor(ctx context.Context) {
	ticker := time.NewTicker(managedAgentInstallationFenceMonitorInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.revalidateCompletedManagedAgentUninstall(ctx)
		}
	}
}

func (d *Daemon) revalidateCompletedManagedAgentUninstall(ctx context.Context) {
	state, err := d.readManagedAgentInstallationState(ctx)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to read managed Agent installation state during fence revalidation")
		return
	}
	if state == nil || state.DesiredState != managedAgentInstallationDesiredStateAbsent || state.TaskState != pbgo.TaskState_DONE {
		return
	}

	intent := managedAgentInstallationIntent{
		Version:                 managedAgentInstallationVersion,
		Provider:                state.Provider,
		InstallationID:          state.InstallationID,
		TargetID:                state.TargetID,
		OperationID:             state.OperationID,
		DesiredState:            state.DesiredState,
		AcknowledgedOperationID: state.AcknowledgedOperationID,
		Bootstrap:               state.Bootstrap,
	}
	command := newManagedAgentInstallationCommand(intent, json.RawMessage(`{}`), state.Digest)
	command.ConfigID = state.ConfigID

	d.transitionMu.Lock()
	_, verifyErr := d.verifyDatadogAgentUninstalled(ctx, command)
	d.transitionMu.Unlock()
	if verifyErr == nil {
		return
	}
	d.taskMu.Lock()
	d.setTaskState(packageDatadogOperator, state.OperationID, pbgo.TaskState_ERROR, verifyErr)
	d.taskMu.Unlock()
	state.TaskState = pbgo.TaskState_ERROR
	state.Error = boundedTaskErrorMessage(verifyErr)
	if err := d.writeManagedAgentInstallationState(ctx, *state); err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to persist managed Agent installation uninstall fence drift", "operationID", state.OperationID)
	}
}

func boundedTaskErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	const maxRunes = 1024
	message := strings.ToValidUTF8(err.Error(), "?")
	runes := []rune(message)
	if len(runes) <= maxRunes {
		return message
	}
	return string(runes[:maxRunes])
}
