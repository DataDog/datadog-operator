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
	"strings"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	managedAgentInstallationIntentConfigMapName = "datadog-agent-managed-installation-intent"
	managedAgentInstallationIntentDataKey       = "intent.json"
	managedAgentInstallationVersion             = "v1"
	managedAgentInstallationMaxIntentSize       = 256 * 1024
)

var (
	managedAgentInstallationIntentKey = types.NamespacedName{
		Namespace: fleetDatadogAgentNamespace,
		Name:      managedAgentInstallationIntentConfigMapName,
	}
	managedAgentInstallationRetryInterval         = time.Second
	managedAgentInstallationCredentialRetryDelays = []time.Duration{
		time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
	}
)

type managedAgentInstallationDesiredState string

const (
	managedAgentInstallationDesiredStateInstalled managedAgentInstallationDesiredState = "installed"
	managedAgentInstallationDesiredStateAbsent    managedAgentInstallationDesiredState = "absent"
)

type managedAgentInstallationBootstrap struct {
	ClusterName string `json:"clusterName"`
	Site        string `json:"site"`
}

type managedAgentInstallationIntent struct {
	Version                 string
	Provider                ManagedAgentInstallationProvider
	InstallationID          string
	TargetID                string
	OperationID             string
	DesiredState            managedAgentInstallationDesiredState
	AcknowledgedOperationID string
	Bootstrap               managedAgentInstallationBootstrap
}

type normalizedManagedAgentInstallationIntent struct {
	Version        string                               `json:"version"`
	Provider       ManagedAgentInstallationProvider     `json:"provider"`
	InstallationID string                               `json:"installationID"`
	TargetID       string                               `json:"targetID"`
	OperationID    string                               `json:"operationID"`
	DesiredState   managedAgentInstallationDesiredState `json:"desiredState"`
	Bootstrap      managedAgentInstallationBootstrap    `json:"bootstrap"`
}

type managedAgentInstallationIntentSnapshot struct {
	raw             []byte
	resourceVersion string
}

func (d *Daemon) installManagedAgentInstallationIntentForwarder(ctx context.Context) error {
	informer, err := d.cache.GetInformer(ctx, &corev1.ConfigMap{})
	if err != nil {
		return fmt.Errorf("get ConfigMap informer for managed Agent installation intent: %w", err)
	}
	informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			d.forwardManagedAgentInstallationIntent(obj)
		},
		UpdateFunc: func(_, newObj any) {
			d.forwardManagedAgentInstallationIntent(newObj)
		},
	})
	return nil
}

func (d *Daemon) installManagedAgentInstallationCredentialForwarder(ctx context.Context) error {
	informer, err := d.cache.GetInformer(ctx, &corev1.Secret{})
	if err != nil {
		return fmt.Errorf("get Secret informer for managed Agent installation credentials: %w", err)
	}
	informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: d.forwardManagedAgentInstallationCredential,
		UpdateFunc: func(oldObj, newObj any) {
			oldSecret, oldOK := oldObj.(*corev1.Secret)
			newSecret, newOK := newObj.(*corev1.Secret)
			if !oldOK || !newOK || len(oldSecret.Data[fleetCredentialAPIKey]) != 0 {
				return
			}
			d.forwardManagedAgentInstallationCredential(newSecret)
		},
	})
	return nil
}

func (d *Daemon) forwardManagedAgentInstallationIntent(obj any) {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok || client.ObjectKeyFromObject(configMap) != managedAgentInstallationIntentKey {
		return
	}
	d.resetManagedAgentInstallationCredentialRetries()
	d.requestManagedAgentInstallationRetry()
}

func (d *Daemon) forwardManagedAgentInstallationCredential(obj any) {
	secret, ok := obj.(*corev1.Secret)
	if !ok || client.ObjectKeyFromObject(secret) != managedAgentInstallationCredentialKey || len(secret.Data[fleetCredentialAPIKey]) == 0 {
		return
	}
	d.resetManagedAgentInstallationCredentialRetries()
	d.requestManagedAgentInstallationRetry()
}

func (d *Daemon) runManagedAgentInstallationIntentWorker(ctx context.Context) {
	logger := ctrl.LoggerFrom(ctx).WithName("managed-agent-installation").WithValues("provider", d.managedAgentInstallationIdentity.Provider())
	for {
		select {
		case <-ctx.Done():
			return
		case <-d.managedAgentInstallationUpdates:
		}

		for {
			snapshot, err := d.readCurrentManagedAgentInstallationIntent(ctx)
			if err == nil {
				err = d.handleManagedAgentInstallationIntent(ctx, snapshot)
			}
			if err == nil {
				break
			}
			if apierrors.IsNotFound(err) {
				break
			}
			logger.Error(err, "Failed to reconcile managed Agent installation intent", "resourceVersion", snapshot.resourceVersion)
			var stateErr *stateDoesntMatchError
			if errors.As(err, &stateErr) {
				break
			}
			timer := time.NewTimer(managedAgentInstallationRetryInterval)
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-d.managedAgentInstallationUpdates:
				timer.Stop()
			case <-timer.C:
			}
		}
	}
}

func (d *Daemon) readCurrentManagedAgentInstallationIntent(ctx context.Context) (managedAgentInstallationIntentSnapshot, error) {
	configMap := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationIntentKey, configMap); err != nil {
		return managedAgentInstallationIntentSnapshot{}, fmt.Errorf("read current managed Agent installation intent: %w", err)
	}
	return managedAgentInstallationIntentSnapshot{
		raw:             []byte(configMap.Data[managedAgentInstallationIntentDataKey]),
		resourceVersion: configMap.ResourceVersion,
	}, nil
}

func (d *Daemon) requestManagedAgentInstallationRetry() {
	if d.managedAgentInstallationUpdates == nil {
		return
	}
	select {
	case d.managedAgentInstallationUpdates <- struct{}{}:
	default:
	}
}

func (d *Daemon) requestManagedAgentInstallationRetryIfWaiting() {
	d.taskMu.Lock()
	waiting := d.managedAgentInstallationIntentsEnabled &&
		d.managedAgentInstallationWaitingForFleet &&
		!d.managedAgentInstallationActive
	d.taskMu.Unlock()
	if waiting {
		d.requestManagedAgentInstallationRetry()
	}
}

func (d *Daemon) requestManagedAgentInstallationRetryAfter() {
	d.requestManagedAgentInstallationRetryAfterDelay(managedAgentInstallationRetryInterval)
}

func (d *Daemon) requestManagedAgentInstallationRetryAfterDelay(delay time.Duration) {
	if d.managedAgentInstallationUpdates == nil {
		return
	}
	time.AfterFunc(delay, d.requestManagedAgentInstallationRetry)
}

func (d *Daemon) resetManagedAgentInstallationCredentialRetries() {
	d.taskMu.Lock()
	d.managedAgentInstallationCredentialRetryIndex = 0
	d.taskMu.Unlock()
}

func (d *Daemon) scheduleManagedAgentInstallationCredentialRetry() bool {
	d.taskMu.Lock()
	d.managedAgentInstallationTaskReserved = false
	if d.managedAgentInstallationCredentialRetryIndex >= len(managedAgentInstallationCredentialRetryDelays) {
		d.taskMu.Unlock()
		return false
	}
	delay := managedAgentInstallationCredentialRetryDelays[d.managedAgentInstallationCredentialRetryIndex]
	d.managedAgentInstallationCredentialRetryIndex++
	d.taskMu.Unlock()

	d.requestManagedAgentInstallationRetryAfterDelay(delay)
	return true
}

func (d *Daemon) handleManagedAgentInstallationIntent(ctx context.Context, snapshot managedAgentInstallationIntentSnapshot) error {
	intent, config, digest, decodeErr := decodeManagedAgentInstallationIntent(snapshot.raw, d.managedAgentInstallationIdentity)
	if decodeErr != nil {
		return &stateDoesntMatchError{msg: decodeErr.Error()}
	}
	current, stateErr := d.readManagedAgentInstallationState(ctx)
	if stateErr != nil {
		return stateErr
	}
	if err := validateManagedAgentInstallationProgress(current, intent, digest); err != nil {
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, intent.OperationID, pbgo.TaskState_INVALID_STATE, err)
		d.taskMu.Unlock()
		return &stateDoesntMatchError{msg: err.Error()}
	}
	if intent.DesiredState == managedAgentInstallationDesiredStateInstalled && intent.AcknowledgedOperationID != "" {
		if current != nil && current.AcknowledgedOperationID == intent.AcknowledgedOperationID {
			return d.reconcileAcknowledgedManagedAgentInstallation(ctx, current)
		}
		return d.acknowledgeManagedAgentInstallationInstall(ctx, current, intent)
	}
	if intent.DesiredState == managedAgentInstallationDesiredStateAbsent {
		d.reserveManagedAgentInstallationTaskSlot()
		if err := d.refreshManagedAgentInstallationUpdaterTags(ctx); err != nil {
			return err
		}
	}
	if current != nil && current.OperationID == intent.OperationID && current.TaskState != pbgo.TaskState_RUNNING {
		return d.reconcileTerminalManagedAgentInstallation(ctx, current, intent, config, digest)
	}
	if err := d.waitForManagedAgentInstallationSlot(ctx, intent); err != nil {
		return err
	}
	current, stateErr = d.readManagedAgentInstallationState(ctx)
	if stateErr != nil {
		return stateErr
	}
	if err := validateManagedAgentInstallationProgress(current, intent, digest); err != nil {
		return &stateDoesntMatchError{msg: err.Error()}
	}
	if current != nil && current.OperationID == intent.OperationID && current.TaskState != pbgo.TaskState_RUNNING {
		return d.reconcileTerminalManagedAgentInstallation(ctx, current, intent, config, digest)
	}
	command := newManagedAgentInstallationCommand(intent, config, digest)
	if err := d.handleManagedAgentInstallationCommand(ctx, command); err != nil {
		return err
	}
	return nil
}

func (d *Daemon) reserveManagedAgentInstallationTaskSlot() {
	d.transitionMu.Lock()
	d.taskMu.Lock()
	d.managedAgentInstallationTaskReserved = true
	d.taskMu.Unlock()
	d.transitionMu.Unlock()
}

func (d *Daemon) waitForManagedAgentInstallationSlot(ctx context.Context, intent managedAgentInstallationIntent) error {
	d.taskMu.Lock()
	if !d.managedAgentInstallationActive {
		d.taskMu.Unlock()
		return nil
	}
	activeOperationID := d.managedAgentInstallationOperationID
	cancel := d.managedAgentInstallationCancel
	done := d.managedAgentInstallationDone
	d.taskMu.Unlock()

	if intent.DesiredState == managedAgentInstallationDesiredStateAbsent && intent.OperationID != activeOperationID && cancel != nil {
		cancel()
	}
	if done == nil {
		return fmt.Errorf("active managed Agent installation operation has no completion signal")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("waiting for active managed Agent installation operation: %w", ctx.Err())
	case <-done:
		return nil
	}
}

func (d *Daemon) acknowledgeManagedAgentInstallationInstall(ctx context.Context, current *managedAgentInstallationPersistedState, intent managedAgentInstallationIntent) error {
	if current == nil || current.DesiredState != managedAgentInstallationDesiredStateInstalled || current.OperationID != intent.OperationID || current.TaskState != pbgo.TaskState_DONE {
		return fmt.Errorf("managed Agent installation install cannot be acknowledged before matching completion")
	}
	if current.AcknowledgedOperationID != "" && current.AcknowledgedOperationID != intent.OperationID {
		return fmt.Errorf("managed Agent installation install is already acknowledged by a different operation")
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.managedAgentInstallationReader().Get(ctx, types.NamespacedName{Namespace: fleetDatadogAgentNamespace, Name: fleetDatadogAgentName}, dda); err != nil {
		return fmt.Errorf("read DatadogAgent before bootstrap acknowledgement: %w", err)
	}
	if err := validateFleetDatadogAgentInstallCompletion(dda, dda.UID, current.OperationID); err != nil {
		return err
	}
	if err := d.validateManagedAgentInstallationWindowsProfileExists(ctx, dda); err != nil {
		return err
	}
	current.AcknowledgedOperationID = intent.OperationID
	if err := d.writeManagedAgentInstallationState(ctx, *current); err != nil {
		return fmt.Errorf("persist managed Agent installation acknowledgement: %w", err)
	}
	return d.reconcileAcknowledgedManagedAgentInstallation(ctx, current)
}

func (d *Daemon) reconcileAcknowledgedManagedAgentInstallation(ctx context.Context, current *managedAgentInstallationPersistedState) error {
	if current == nil || current.DesiredState != managedAgentInstallationDesiredStateInstalled ||
		current.TaskState != pbgo.TaskState_DONE || current.AcknowledgedOperationID != current.OperationID {
		return fmt.Errorf("managed Agent installation acknowledgement state is incomplete")
	}

	dda, err := d.validateAcknowledgedManagedAgentInstallation(ctx)
	if err != nil {
		return err
	}

	d.taskMu.Lock()
	wasReserved := d.managedAgentInstallationTaskReserved
	d.managedAgentInstallationTaskReserved = false
	d.taskMu.Unlock()

	if wasReserved && d.statusUpdates != nil {
		select {
		case d.statusUpdates <- newDDAStatusSnapshot(dda):
		default:
		}
	}
	return d.refreshManagedAgentInstallationUpdaterTags(ctx)
}

func (d *Daemon) validateAcknowledgedManagedAgentInstallation(ctx context.Context) (*v2alpha1.DatadogAgent, error) {
	dda, err := d.validateManagedAgentInstallationTarget(ctx, managedAgentInstallationTarget)
	if err != nil {
		return nil, err
	}
	if dda == nil {
		return nil, fmt.Errorf("managed Agent installation target is absent after install acknowledgement")
	}
	if readyErr := validateFleetDatadogAgentManagedAgentInstallationReady(dda); readyErr != nil {
		return nil, readyErr
	}
	if err := d.validateManagedAgentInstallationWindowsProfileExists(ctx, dda); err != nil {
		return nil, err
	}
	return dda, nil
}

func (d *Daemon) reconcileTerminalManagedAgentInstallation(
	ctx context.Context,
	current *managedAgentInstallationPersistedState,
	intent managedAgentInstallationIntent,
	config json.RawMessage,
	digest string,
) error {
	switch current.TaskState {
	case pbgo.TaskState_DONE:
		return d.handleManagedAgentInstallationCommand(ctx, newManagedAgentInstallationCommand(intent, config, digest))
	case pbgo.TaskState_ERROR, pbgo.TaskState_INVALID_STATE:
		var taskErr error
		if current.Error != "" {
			taskErr = errors.New(current.Error)
		}
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, current.OperationID, current.TaskState, taskErr)
		d.taskMu.Unlock()
		return nil
	default:
		return fmt.Errorf("managed Agent installation operation %s has unsupported terminal state %s", current.OperationID, current.TaskState)
	}
}

func (d *Daemon) refreshManagedAgentInstallationUpdaterTags(ctx context.Context) error {
	if d.rcClient == nil {
		return nil
	}
	refresher, ok := d.rcClient.(interface {
		RefreshUpdaterTags(context.Context) error
	})
	if !ok {
		return fmt.Errorf("Remote Configuration client does not support updater tag refresh")
	}
	if err := refresher.RefreshUpdaterTags(ctx); err != nil {
		return fmt.Errorf("refresh Remote Configuration updater tags for managed Agent installation: %w", err)
	}
	return nil
}

func decodeManagedAgentInstallationIntent(raw []byte, identity ManagedAgentInstallationIdentity) (managedAgentInstallationIntent, json.RawMessage, string, error) {
	switch identity.Provider() {
	case ManagedAgentInstallationProviderEKS:
		return decodeEKSManagedAgentInstallationIntent(raw, identity)
	default:
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("unsupported managed Agent installation provider %q", identity.Provider())
	}
}

func validateManagedAgentInstallationBootstrap(bootstrap managedAgentInstallationBootstrap) error {
	if strings.TrimSpace(bootstrap.ClusterName) != bootstrap.ClusterName || bootstrap.ClusterName == "" || len(bootstrap.ClusterName) > 100 {
		return fmt.Errorf("managed Agent installation bootstrap cluster name is invalid")
	}
	if _, ok := allowedManagedAgentInstallationSites[bootstrap.Site]; !ok {
		return fmt.Errorf("managed Agent installation bootstrap site %q is unsupported", bootstrap.Site)
	}
	return nil
}

func managedAgentInstallationBootstrapConfig(bootstrap managedAgentInstallationBootstrap) (json.RawMessage, error) {
	clusterName := bootstrap.ClusterName
	site := bootstrap.Site
	linuxSelector := map[string]string{corev1.LabelOSStable: string(corev1.Linux)}
	config := datadogAgentManagedAgentInstallationConfig{Spec: &v2alpha1.DatadogAgentSpec{
		Global: &v2alpha1.GlobalConfig{
			ClusterName: &clusterName,
			Site:        &site,
		},
		Override: map[v2alpha1.ComponentName]*v2alpha1.DatadogAgentComponentOverride{
			v2alpha1.NodeAgentComponentName:    {NodeSelector: linuxSelector},
			v2alpha1.ClusterAgentComponentName: {NodeSelector: linuxSelector},
		},
	}}
	encoded, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("encode managed Agent installation bootstrap config: %w", err)
	}
	return encoded, nil
}

func validateCanonicalUUID(field, value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil || parsed.String() != value {
		return fmt.Errorf("managed Agent installation %s must be a canonical non-zero UUID", field)
	}
	return nil
}

func validateManagedAgentInstallationProgress(current *managedAgentInstallationPersistedState, intent managedAgentInstallationIntent, digest string) error {
	if current == nil {
		if intent.DesiredState != managedAgentInstallationDesiredStateInstalled || intent.AcknowledgedOperationID != "" {
			return fmt.Errorf("the first managed Agent installation request must be an unacknowledged install")
		}
		return nil
	}
	if current.Provider != intent.Provider || current.InstallationID != intent.InstallationID || current.TargetID != intent.TargetID {
		return fmt.Errorf("persisted managed Agent installation state belongs to a different installation")
	}
	if intent.OperationID == current.OperationID {
		if digest != current.Digest {
			return fmt.Errorf("managed Agent installation operation %q was replayed with different content", intent.OperationID)
		}
		if current.DesiredState == managedAgentInstallationDesiredStateInstalled {
			switch {
			case current.AcknowledgedOperationID == "" && (intent.AcknowledgedOperationID == "" || intent.AcknowledgedOperationID == intent.OperationID):
				return nil
			case current.AcknowledgedOperationID == intent.OperationID && intent.AcknowledgedOperationID == intent.OperationID:
				return nil
			default:
				return fmt.Errorf("managed Agent installation install acknowledgement regressed or changed")
			}
		}
		if intent.AcknowledgedOperationID != current.AcknowledgedOperationID {
			return fmt.Errorf("managed Agent installation uninstall acknowledgement changed")
		}
		return nil
	}
	if current.DesiredState == managedAgentInstallationDesiredStateAbsent {
		return fmt.Errorf("managed Agent installation install cannot follow an accepted uninstall")
	}
	if intent.DesiredState != managedAgentInstallationDesiredStateAbsent {
		return fmt.Errorf("a new managed Agent installation operation must uninstall the existing installation")
	}
	if intent.AcknowledgedOperationID != current.AcknowledgedOperationID {
		return fmt.Errorf("managed Agent installation uninstall must retain the install acknowledgement")
	}
	return nil
}
