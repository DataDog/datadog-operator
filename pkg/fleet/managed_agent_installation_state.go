// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"errors"
	"fmt"
	"strings"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8sretry "k8s.io/client-go/util/retry"
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
	managedAgentInstallationStateAcknowledgedKey   = "acknowledged_operation_id"
	managedAgentInstallationStateTaskStateKey      = "task_state"
	managedAgentInstallationStateErrorKey          = "error"
	managedAgentInstallationAckTagPrefix           = "managed_agent_installation_ack:"
)

var (
	managedAgentInstallationStateKey = types.NamespacedName{
		Namespace: fleetDatadogAgentNamespace,
		Name:      managedAgentInstallationStateConfigMapName,
	}
)

type managedAgentInstallationPersistedState struct {
	Provider                remoteconfig.ManagedAgentInstallationProvider
	InstallationID          string
	TargetID                string
	OperationID             string
	Digest                  string
	DesiredState            managedAgentInstallationDesiredState
	AcknowledgedOperationID string
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
	intent, _, digest, err := decodeManagedAgentInstallationIntent([]byte(intentConfigMap.Data[managedAgentInstallationIntentDataKey]), identity)
	if err != nil {
		return nil, err
	}
	if intent.AcknowledgedOperationID == "" {
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
		state.AcknowledgedOperationID != intent.AcknowledgedOperationID {
		return nil, fmt.Errorf("managed Agent installation acknowledgement state is not consistent with the current intent")
	}

	tags := []string{managedAgentInstallationAckTagPrefix + intent.AcknowledgedOperationID}
	switch intent.DesiredState {
	case managedAgentInstallationDesiredStateInstalled:
		if intent.OperationID != intent.AcknowledgedOperationID ||
			state.OperationID != intent.AcknowledgedOperationID ||
			state.DesiredState != managedAgentInstallationDesiredStateInstalled ||
			state.TaskState != pbgo.TaskState_DONE {
			return nil, fmt.Errorf("managed Agent installation acknowledgement state is not yet consistent with the acknowledged install intent")
		}
		if _, err := daemon.validateAcknowledgedManagedAgentInstallation(ctx); err != nil {
			return nil, fmt.Errorf("validate managed Agent installation acknowledgement for Remote Configuration updater tags: %w", err)
		}
		return append(tags, "operator_config_updates:ready"), nil
	case managedAgentInstallationDesiredStateAbsent:
		acknowledgedInstall := state.DesiredState == managedAgentInstallationDesiredStateInstalled &&
			state.OperationID == intent.AcknowledgedOperationID &&
			state.TaskState == pbgo.TaskState_DONE
		currentUninstall := state.DesiredState == managedAgentInstallationDesiredStateAbsent &&
			state.OperationID == intent.OperationID &&
			state.Digest == digest
		if !acknowledgedInstall && !currentUninstall {
			return nil, fmt.Errorf("managed Agent installation acknowledgement state is not consistent with the uninstall intent")
		}
		return tags, nil
	default:
		return nil, nil
	}
}

func managedAgentInstallationStateFromCommand(command managedAgentInstallationCommand, taskState pbgo.TaskState, taskErr error) managedAgentInstallationPersistedState {
	state := managedAgentInstallationPersistedState{
		Provider:                command.Intent.Provider,
		InstallationID:          command.Intent.InstallationID,
		TargetID:                command.Intent.TargetID,
		OperationID:             command.Intent.OperationID,
		Digest:                  command.Digest,
		DesiredState:            command.Intent.DesiredState,
		AcknowledgedOperationID: command.Intent.AcknowledgedOperationID,
		TaskState:               taskState,
	}
	if taskErr != nil {
		state.Error = boundedTaskErrorMessage(taskErr)
	}
	return state
}

func (d *Daemon) recordManagedAgentInstallationResult(ctx context.Context, command managedAgentInstallationCommand, taskState pbgo.TaskState, taskErr error) error {
	if err := d.writeManagedAgentInstallationResult(ctx, managedAgentInstallationStateFromCommand(command, taskState, taskErr)); err != nil {
		return fmt.Errorf("persist managed Agent installation result for operation %s: %w", command.Intent.OperationID, err)
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
			owner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID)
			owner.BlockOwnerDeletion = nil
			return d.client.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace:       managedAgentInstallationStateKey.Namespace,
					Name:            managedAgentInstallationStateKey.Name,
					OwnerReferences: []metav1.OwnerReference{owner},
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
	return map[string]string{
		managedAgentInstallationStateProviderKey:       string(state.Provider),
		managedAgentInstallationStateInstallationIDKey: state.InstallationID,
		managedAgentInstallationStateTargetIDKey:       state.TargetID,
		managedAgentInstallationStateOperationIDKey:    state.OperationID,
		managedAgentInstallationStateDigestKey:         state.Digest,
		managedAgentInstallationStateDesiredStateKey:   string(state.DesiredState),
		managedAgentInstallationStateAcknowledgedKey:   state.AcknowledgedOperationID,
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
	state := &managedAgentInstallationPersistedState{
		Provider:                remoteconfig.ManagedAgentInstallationProvider(configMap.Data[managedAgentInstallationStateProviderKey]),
		InstallationID:          configMap.Data[managedAgentInstallationStateInstallationIDKey],
		TargetID:                configMap.Data[managedAgentInstallationStateTargetIDKey],
		OperationID:             configMap.Data[managedAgentInstallationStateOperationIDKey],
		Digest:                  configMap.Data[managedAgentInstallationStateDigestKey],
		DesiredState:            managedAgentInstallationDesiredState(configMap.Data[managedAgentInstallationStateDesiredStateKey]),
		AcknowledgedOperationID: configMap.Data[managedAgentInstallationStateAcknowledgedKey],
		TaskState:               taskState,
		Error:                   configMap.Data[managedAgentInstallationStateErrorKey],
	}
	if state.Provider == "" || state.InstallationID == "" || state.TargetID == "" || state.OperationID == "" || state.Digest == "" {
		return nil, fmt.Errorf("managed Agent installation state is incomplete")
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
	if state.TaskState == pbgo.TaskState_DONE {
		return nil
	}
	var taskErr error
	if state.Error != "" {
		taskErr = errors.New(state.Error)
	}
	d.taskMu.Lock()
	d.setTaskState(packageDatadogOperator, state.OperationID, state.TaskState, taskErr)
	d.taskMu.Unlock()
	return nil
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
