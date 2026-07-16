// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	pbgo "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/google/uuid"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	toolscache "k8s.io/client-go/tools/cache"
	k8sretry "k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/remoteconfig"
)

const (
	managedAgentInstallationIntentConfigMapName = "datadog-agent-managed-installation-intent"
	managedAgentInstallationIntentDataKey       = "intent.json"
	managedAgentInstallationStateConfigMapName  = "datadog-agent-managed-installation-state"
	managedAgentInstallationVersion             = "v1"
	managedAgentInstallationMaxIntentSize       = 256 * 1024

	managedAgentInstallationStateInstallationIDKey = "installation_id"
	managedAgentInstallationStateEKSARNHashKey     = "eks_arn_sha256"
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
	managedAgentInstallationIntentKey = types.NamespacedName{
		Namespace: fleetDatadogAgentNamespace,
		Name:      managedAgentInstallationIntentConfigMapName,
	}
	managedAgentInstallationStateKey = types.NamespacedName{
		Namespace: fleetDatadogAgentNamespace,
		Name:      managedAgentInstallationStateConfigMapName,
	}
	managedAgentInstallationFenceMonitorInterval = 10 * time.Second
	managedAgentInstallationRetryInterval        = time.Second
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
	Version                 string                               `json:"version"`
	InstallationID          string                               `json:"installationID"`
	TargetHash              string                               `json:"eksARNSHA256"`
	OperationID             string                               `json:"operationID"`
	DesiredState            managedAgentInstallationDesiredState `json:"desiredState"`
	AcknowledgedOperationID string                               `json:"acknowledgedOperationID,omitempty"`
	Bootstrap               managedAgentInstallationBootstrap    `json:"bootstrap"`
}

type normalizedManagedAgentInstallationIntent struct {
	Version        string                               `json:"version"`
	InstallationID string                               `json:"installationID"`
	TargetHash     string                               `json:"eksARNSHA256"`
	OperationID    string                               `json:"operationID"`
	DesiredState   managedAgentInstallationDesiredState `json:"desiredState"`
	Bootstrap      managedAgentInstallationBootstrap    `json:"bootstrap"`
}

type managedAgentInstallationIntentSnapshot struct {
	raw             []byte
	resourceVersion string
}

type managedAgentInstallationRequestMetadata struct {
	Digest                  string
	DesiredState            managedAgentInstallationDesiredState
	ConfigID                string
	TargetHash              string
	Bootstrap               managedAgentInstallationBootstrap
	AcknowledgedOperationID string
}

type managedAgentInstallationPersistedState struct {
	InstallationID          string
	TargetHash              string
	OperationID             string
	Digest                  string
	DesiredState            managedAgentInstallationDesiredState
	Bootstrap               managedAgentInstallationBootstrap
	AcknowledgedOperationID string
	ConfigID                string
	TaskState               pbgo.TaskState
	Error                   string
}

func ReconcileManagedAgentInstallationAcknowledgement(ctx context.Context, kubeClient client.Client, reader client.Reader, identity remoteconfig.ManagedAgentInstallationIdentity) error {
	intentConfigMap := &corev1.ConfigMap{}
	if err := reader.Get(ctx, managedAgentInstallationIntentKey, intentConfigMap); err != nil {
		return fmt.Errorf("read EKS add-on managed Agent installation intent before Remote Configuration startup: %w", err)
	}
	intent, _, digest, err := decodeManagedAgentInstallationIntent([]byte(intentConfigMap.Data[managedAgentInstallationIntentDataKey]), identity)
	if err != nil {
		return err
	}
	if intent.AcknowledgedOperationID == "" {
		return nil
	}
	daemon := &Daemon{
		client:                           kubeClient,
		apiReader:                        reader,
		managedAgentInstallationIdentity: identity,
	}
	current, err := daemon.readManagedAgentInstallationState(ctx)
	if err != nil {
		return err
	}
	if err := validateManagedAgentInstallationProgress(current, intent, digest); err != nil {
		return err
	}
	if current != nil && current.AcknowledgedOperationID == intent.AcknowledgedOperationID {
		return nil
	}
	return daemon.acknowledgeManagedAgentInstallationInstall(ctx, current, intent)
}

func (d *Daemon) installManagedAgentInstallationIntentForwarder(ctx context.Context) error {
	informer, err := d.cache.GetInformer(ctx, &corev1.ConfigMap{})
	if err != nil {
		return fmt.Errorf("get ConfigMap informer for EKS add-on managed Agent installation intent: %w", err)
	}
	informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			d.forwardManagedAgentInstallationIntent(ctx, obj)
		},
		UpdateFunc: func(_, newObj any) {
			d.forwardManagedAgentInstallationIntent(ctx, newObj)
		},
	})
	return nil
}

func (d *Daemon) forwardManagedAgentInstallationIntent(ctx context.Context, obj any) {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok || client.ObjectKeyFromObject(configMap) != managedAgentInstallationIntentKey {
		return
	}
	snapshot := managedAgentInstallationIntentSnapshot{
		raw:             []byte(configMap.Data[managedAgentInstallationIntentDataKey]),
		resourceVersion: configMap.ResourceVersion,
	}
	select {
	case d.managedAgentInstallationUpdates <- snapshot:
	case <-ctx.Done():
	}
}

func (d *Daemon) runManagedAgentInstallationIntentWorker(ctx context.Context) {
	logger := ctrl.LoggerFrom(ctx).WithName("eks-managed-agent-installation")
	var pending *managedAgentInstallationIntentSnapshot
	reload := false
	for {
		if pending == nil {
			if reload {
				snapshot, err := d.readCurrentManagedAgentInstallationIntent(ctx)
				if err != nil {
					logger.Error(err, "Failed to reload EKS add-on managed Agent installation intent")
					timer := time.NewTimer(managedAgentInstallationRetryInterval)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case snapshot := <-d.managedAgentInstallationUpdates:
						timer.Stop()
						pending = &snapshot
						reload = false
					case <-timer.C:
					}
					continue
				}
				pending = &snapshot
				reload = false
			} else {
				select {
				case <-ctx.Done():
					return
				case snapshot := <-d.managedAgentInstallationUpdates:
					pending = &snapshot
				case <-d.managedAgentInstallationRetries:
					reload = true
				}
			}
		}

		if pending == nil {
			continue
		}
		if err := d.handleManagedAgentInstallationIntent(ctx, *pending); err == nil {
			pending = nil
			continue
		} else {
			logger.Error(err, "Failed to reconcile EKS add-on managed Agent installation intent", "resourceVersion", pending.resourceVersion)
		}

		timer := time.NewTimer(managedAgentInstallationRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case snapshot := <-d.managedAgentInstallationUpdates:
			timer.Stop()
			pending = &snapshot
		case <-d.managedAgentInstallationRetries:
			timer.Stop()
			pending = nil
			reload = true
		case <-timer.C:
		}
	}
}

func (d *Daemon) readCurrentManagedAgentInstallationIntent(ctx context.Context) (managedAgentInstallationIntentSnapshot, error) {
	configMap := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationIntentKey, configMap); err != nil {
		return managedAgentInstallationIntentSnapshot{}, fmt.Errorf("read current EKS add-on managed Agent installation intent: %w", err)
	}
	return managedAgentInstallationIntentSnapshot{
		raw:             []byte(configMap.Data[managedAgentInstallationIntentDataKey]),
		resourceVersion: configMap.ResourceVersion,
	}, nil
}

func (d *Daemon) requestManagedAgentInstallationRetry() {
	if d.managedAgentInstallationRetries == nil {
		return
	}
	select {
	case d.managedAgentInstallationRetries <- struct{}{}:
	default:
	}
}

func (d *Daemon) handleManagedAgentInstallationIntent(ctx context.Context, snapshot managedAgentInstallationIntentSnapshot) error {
	intent, config, digest, err := decodeManagedAgentInstallationIntent(snapshot.raw, d.managedAgentInstallationIdentity)
	if err != nil {
		return err
	}
	if err := d.refreshManagedAgentInstallationUpdaterTags(ctx); err != nil {
		return err
	}
	configID := intent.OperationID

	current, err := d.readManagedAgentInstallationState(ctx)
	if err != nil {
		return err
	}
	if err := validateManagedAgentInstallationProgress(current, intent, digest); err != nil {
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, intent.OperationID, pbgo.TaskState_INVALID_STATE, err)
		d.taskMu.Unlock()
		return err
	}
	if intent.DesiredState == managedAgentInstallationDesiredStateInstalled && intent.AcknowledgedOperationID != "" {
		if current != nil && current.AcknowledgedOperationID == intent.AcknowledgedOperationID {
			return nil
		}
		return d.acknowledgeManagedAgentInstallationInstall(ctx, current, intent)
	}
	if current != nil && current.OperationID == intent.OperationID && current.TaskState != pbgo.TaskState_RUNNING {
		return nil
	}
	if err := d.waitForManagedAgentInstallationSlot(ctx, intent); err != nil {
		return err
	}
	current, err = d.readManagedAgentInstallationState(ctx)
	if err != nil {
		return err
	}
	if err := validateManagedAgentInstallationProgress(current, intent, digest); err != nil {
		return err
	}
	if current != nil && current.OperationID == intent.OperationID && current.TaskState != pbgo.TaskState_RUNNING {
		return nil
	}
	request := d.newManagedAgentInstallationRequest(intent, configID, digest)
	d.registerManagedAgentInstallationConfig(configID, config, intent.DesiredState)
	if err := d.handleManagedAgentInstallationTask(ctx, request); err != nil {
		return err
	}
	return nil
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
		return fmt.Errorf("active EKS add-on managed Agent installation operation has no completion signal")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("waiting for active EKS add-on managed Agent installation operation: %w", ctx.Err())
	case <-done:
		return nil
	}
}

func (d *Daemon) acknowledgeManagedAgentInstallationInstall(ctx context.Context, current *managedAgentInstallationPersistedState, intent managedAgentInstallationIntent) error {
	if current == nil || current.DesiredState != managedAgentInstallationDesiredStateInstalled || current.OperationID != intent.OperationID || current.TaskState != pbgo.TaskState_DONE {
		return fmt.Errorf("EKS add-on managed Agent installation install cannot be acknowledged before matching completion")
	}
	if current.AcknowledgedOperationID != "" && current.AcknowledgedOperationID != intent.OperationID {
		return fmt.Errorf("EKS add-on managed Agent installation install is already acknowledged by a different operation")
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.managedAgentInstallationReader().Get(ctx, types.NamespacedName{Namespace: fleetDatadogAgentNamespace, Name: fleetDatadogAgentName}, dda); err != nil {
		return fmt.Errorf("read DatadogAgent before bootstrap acknowledgement: %w", err)
	}
	if err := validateFleetDatadogAgentInstallCompletion(dda, dda.UID, current.ConfigID); err != nil {
		return err
	}
	if err := d.validateManagedAgentInstallationResourcesReady(ctx, dda); err != nil {
		return err
	}
	current.AcknowledgedOperationID = intent.OperationID
	if err := d.writeManagedAgentInstallationState(ctx, *current); err != nil {
		return fmt.Errorf("persist EKS add-on managed Agent installation acknowledgement: %w", err)
	}
	if err := d.refreshManagedAgentInstallationUpdaterTags(ctx); err != nil {
		return err
	}
	d.taskMu.Lock()
	d.managedAgentInstallationTaskReserved = false
	d.taskMu.Unlock()
	return nil
}

func (d *Daemon) refreshManagedAgentInstallationUpdaterTags(ctx context.Context) error {
	if d.rcClient == nil {
		return nil
	}
	if err := d.rcClient.RefreshUpdaterTags(ctx); err != nil {
		return fmt.Errorf("refresh Remote Configuration updater tags for EKS add-on managed Agent installation: %w", err)
	}
	return nil
}

func decodeManagedAgentInstallationIntent(raw []byte, identity remoteconfig.ManagedAgentInstallationIdentity) (managedAgentInstallationIntent, json.RawMessage, string, error) {
	if len(raw) == 0 {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS add-on managed Agent installation intent is missing %q", managedAgentInstallationIntentDataKey)
	}
	if len(raw) > managedAgentInstallationMaxIntentSize {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS add-on managed Agent installation intent exceeds %d bytes", managedAgentInstallationMaxIntentSize)
	}

	var intent managedAgentInstallationIntent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intent); err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("decode EKS add-on managed Agent installation intent: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("decode EKS add-on managed Agent installation intent: trailing JSON content")
	}
	if intent.Version != managedAgentInstallationVersion {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("unsupported EKS add-on managed Agent installation version %q", intent.Version)
	}
	if err := validateCanonicalUUID("installation_id", intent.InstallationID); err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}
	if intent.InstallationID != identity.InstallationID {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS add-on managed Agent installation ID does not match the local installation")
	}
	if intent.TargetHash != identity.TargetHash {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS add-on managed Agent installation ARN hash does not match the local installation")
	}
	if err := validateCanonicalUUID("operation_id", intent.OperationID); err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}
	if intent.AcknowledgedOperationID != "" {
		if err := validateCanonicalUUID("acknowledged_operation_id", intent.AcknowledgedOperationID); err != nil {
			return managedAgentInstallationIntent{}, nil, "", err
		}
		if intent.DesiredState == managedAgentInstallationDesiredStateInstalled && intent.AcknowledgedOperationID != intent.OperationID {
			return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS add-on managed Agent installation acknowledgement must match the active install operation")
		}
	}
	if err := validateManagedAgentInstallationBootstrap(intent.Bootstrap); err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}

	var normalizedConfig json.RawMessage
	switch intent.DesiredState {
	case managedAgentInstallationDesiredStateInstalled:
		var err error
		normalizedConfig, err = managedAgentInstallationBootstrapConfig(intent.Bootstrap)
		if err != nil {
			return managedAgentInstallationIntent{}, nil, "", err
		}
	case managedAgentInstallationDesiredStateAbsent:
		normalizedConfig = json.RawMessage(`{}`)
	default:
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("unsupported EKS add-on managed Agent installation desired state %q", intent.DesiredState)
	}

	normalized := normalizedManagedAgentInstallationIntent{
		Version:        intent.Version,
		InstallationID: intent.InstallationID,
		TargetHash:     intent.TargetHash,
		OperationID:    intent.OperationID,
		DesiredState:   intent.DesiredState,
		Bootstrap:      intent.Bootstrap,
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("encode normalized EKS add-on managed Agent installation intent: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return intent, normalizedConfig, hex.EncodeToString(digest[:]), nil
}

func validateManagedAgentInstallationBootstrap(bootstrap managedAgentInstallationBootstrap) error {
	if strings.TrimSpace(bootstrap.ClusterName) != bootstrap.ClusterName || bootstrap.ClusterName == "" || len(bootstrap.ClusterName) > 100 {
		return fmt.Errorf("EKS add-on managed Agent installation bootstrap cluster name is invalid")
	}
	if _, ok := allowedManagedAgentInstallationSites[bootstrap.Site]; !ok {
		return fmt.Errorf("EKS add-on managed Agent installation bootstrap site %q is unsupported", bootstrap.Site)
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
		return nil, fmt.Errorf("encode EKS add-on managed Agent installation bootstrap config: %w", err)
	}
	return encoded, nil
}

func validateCanonicalUUID(field, value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil || parsed.String() != value {
		return fmt.Errorf("EKS add-on managed Agent installation %s must be a canonical non-zero UUID", field)
	}
	return nil
}

func validateManagedAgentInstallationProgress(current *managedAgentInstallationPersistedState, intent managedAgentInstallationIntent, digest string) error {
	if current == nil {
		if intent.DesiredState != managedAgentInstallationDesiredStateInstalled || intent.AcknowledgedOperationID != "" {
			return fmt.Errorf("the first EKS add-on managed Agent installation request must be an unacknowledged install")
		}
		return nil
	}
	if current.InstallationID != intent.InstallationID || current.TargetHash != intent.TargetHash {
		return fmt.Errorf("persisted EKS add-on managed Agent installation state belongs to a different installation")
	}
	if intent.OperationID == current.OperationID {
		if digest != current.Digest {
			return fmt.Errorf("EKS add-on managed Agent installation operation %q was replayed with different content", intent.OperationID)
		}
		if current.DesiredState == managedAgentInstallationDesiredStateInstalled {
			switch {
			case current.AcknowledgedOperationID == "" && (intent.AcknowledgedOperationID == "" || intent.AcknowledgedOperationID == intent.OperationID):
				return nil
			case current.AcknowledgedOperationID == intent.OperationID && intent.AcknowledgedOperationID == intent.OperationID:
				return nil
			default:
				return fmt.Errorf("EKS add-on managed Agent installation install acknowledgement regressed or changed")
			}
		}
		if intent.AcknowledgedOperationID != current.AcknowledgedOperationID {
			return fmt.Errorf("EKS add-on managed Agent installation uninstall acknowledgement changed")
		}
		return nil
	}
	if current.DesiredState == managedAgentInstallationDesiredStateAbsent {
		return fmt.Errorf("EKS add-on managed Agent installation install cannot follow an accepted uninstall")
	}
	if intent.DesiredState != managedAgentInstallationDesiredStateAbsent {
		return fmt.Errorf("a new EKS add-on managed Agent installation operation must uninstall the existing installation")
	}
	if intent.AcknowledgedOperationID != current.AcknowledgedOperationID {
		return fmt.Errorf("EKS add-on managed Agent installation uninstall must retain the install acknowledgement")
	}
	return nil
}

func (d *Daemon) newManagedAgentInstallationRequest(intent managedAgentInstallationIntent, configID, digest string) remoteAPIRequest {
	stableConfig, experimentConfig := d.getPackageConfigVersions(packageDatadogOperator)
	clientID := ""
	if d.rcClient != nil {
		clientID = d.rcClient.GetClientID()
	}
	method := methodInstallDatadogAgent
	if intent.DesiredState == managedAgentInstallationDesiredStateAbsent {
		method = methodUninstallDatadogAgent
	}
	return remoteAPIRequest{
		ID:      intent.OperationID,
		Package: packageDatadogOperator,
		ExpectedState: expectedState{
			StableConfig:     stableConfig,
			ExperimentConfig: experimentConfig,
			ClientID:         clientID,
		},
		Method: method,
		Params: operatorTaskParams{
			Version:          configID,
			GroupVersionKind: v2alpha1.GroupVersion.WithKind("DatadogAgent"),
			NamespacedName:   types.NamespacedName{Namespace: fleetDatadogAgentNamespace, Name: fleetDatadogAgentName},
			OperationID:      intent.OperationID,
			InstallationID:   intent.InstallationID,
		},
		Addon: &managedAgentInstallationRequestMetadata{
			Digest:                  digest,
			DesiredState:            intent.DesiredState,
			ConfigID:                configID,
			TargetHash:              intent.TargetHash,
			Bootstrap:               intent.Bootstrap,
			AcknowledgedOperationID: intent.AcknowledgedOperationID,
		},
	}
}

func (d *Daemon) registerManagedAgentInstallationConfig(configID string, config json.RawMessage, desiredState managedAgentInstallationDesiredState) {
	operation := OperationCreate
	if desiredState == managedAgentInstallationDesiredStateAbsent {
		operation = OperationDelete
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.addonConfigs = map[string]installerConfig{
		configID: {
			ID: configID,
			Operations: []fleetManagementOperation{{
				Operation: operation,
				Config:    config,
			}},
		},
	}
}

func managedAgentInstallationStateFromRequest(req remoteAPIRequest, taskState pbgo.TaskState, taskErr error) managedAgentInstallationPersistedState {
	state := managedAgentInstallationPersistedState{
		InstallationID:          req.Params.InstallationID,
		TargetHash:              req.Addon.TargetHash,
		OperationID:             req.Params.OperationID,
		Digest:                  req.Addon.Digest,
		DesiredState:            req.Addon.DesiredState,
		Bootstrap:               req.Addon.Bootstrap,
		AcknowledgedOperationID: req.Addon.AcknowledgedOperationID,
		ConfigID:                req.Addon.ConfigID,
		TaskState:               taskState,
	}
	if taskErr != nil {
		state.Error = boundedTaskErrorMessage(taskErr)
	}
	return state
}

func (d *Daemon) recordManagedAgentInstallationResult(ctx context.Context, req remoteAPIRequest, taskState pbgo.TaskState, taskErr error) error {
	if req.Addon == nil {
		return nil
	}
	if err := d.writeManagedAgentInstallationResult(ctx, managedAgentInstallationStateFromRequest(req, taskState, taskErr)); err != nil {
		return fmt.Errorf("persist EKS add-on managed Agent installation result for operation %s: %w", req.Params.OperationID, err)
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
		err := d.client.Get(ctx, managedAgentInstallationStateKey, current)
		if apierrors.IsNotFound(err) {
			if expectedOperationID != "" {
				return fmt.Errorf("EKS add-on managed Agent installation state is missing for operation %s", expectedOperationID)
			}
			intent := &corev1.ConfigMap{}
			if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationIntentKey, intent); err != nil {
				return fmt.Errorf("read EKS add-on managed Agent installation intent owner: %w", err)
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
		if err != nil {
			return err
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
		managedAgentInstallationStateInstallationIDKey: state.InstallationID,
		managedAgentInstallationStateEKSARNHashKey:     state.TargetHash,
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
		return nil, fmt.Errorf("read EKS add-on managed Agent installation state: %w", err)
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
		return nil, fmt.Errorf("EKS add-on managed Agent installation state has invalid bootstrap: %w", err)
	}
	state := &managedAgentInstallationPersistedState{
		InstallationID:          configMap.Data[managedAgentInstallationStateInstallationIDKey],
		TargetHash:              configMap.Data[managedAgentInstallationStateEKSARNHashKey],
		OperationID:             configMap.Data[managedAgentInstallationStateOperationIDKey],
		Digest:                  configMap.Data[managedAgentInstallationStateDigestKey],
		DesiredState:            managedAgentInstallationDesiredState(configMap.Data[managedAgentInstallationStateDesiredStateKey]),
		Bootstrap:               bootstrap,
		AcknowledgedOperationID: configMap.Data[managedAgentInstallationStateAcknowledgedKey],
		ConfigID:                configMap.Data[managedAgentInstallationStateConfigIDKey],
		TaskState:               taskState,
		Error:                   configMap.Data[managedAgentInstallationStateErrorKey],
	}
	if state.InstallationID == "" || state.TargetHash == "" || state.OperationID == "" || state.Digest == "" || state.ConfigID == "" {
		return nil, fmt.Errorf("EKS add-on managed Agent installation state is incomplete")
	}
	if err := validateManagedAgentInstallationBootstrap(state.Bootstrap); err != nil {
		return nil, err
	}
	if state.DesiredState != managedAgentInstallationDesiredStateInstalled && state.DesiredState != managedAgentInstallationDesiredStateAbsent {
		return nil, fmt.Errorf("EKS add-on managed Agent installation state has unsupported desired state %q", state.DesiredState)
	}
	return state, nil
}

func (d *Daemon) validateManagedAgentInstallationStateOwner(ctx context.Context, state *corev1.ConfigMap) error {
	intent := &corev1.ConfigMap{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationIntentKey, intent); err != nil {
		return fmt.Errorf("read EKS add-on managed Agent installation intent owner: %w", err)
	}
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID)
	if err := requireManagedAgentInstallationResourceOwner(state.OwnerReferences, wantOwner); err != nil {
		return fmt.Errorf("validate EKS add-on managed Agent installation state ownership: %w", err)
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
		return pbgo.TaskState(0), fmt.Errorf("EKS add-on managed Agent installation state has unsupported task state %q", value)
	}
}

func (d *Daemon) rehydrateManagedAgentInstallationState(ctx context.Context) error {
	state, err := d.readManagedAgentInstallationState(ctx)
	if err != nil || state == nil {
		return err
	}
	if state.InstallationID != d.managedAgentInstallationIdentity.InstallationID || state.TargetHash != d.managedAgentInstallationIdentity.TargetHash {
		return fmt.Errorf("persisted EKS add-on managed Agent installation state belongs to a different installation")
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
			d.revalidateCompletedAddonUninstall(ctx)
		}
	}
}

func (d *Daemon) revalidateCompletedAddonUninstall(ctx context.Context) {
	state, err := d.readManagedAgentInstallationState(ctx)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to read EKS add-on managed Agent installation state during fence revalidation")
		return
	}
	if state == nil || state.DesiredState != managedAgentInstallationDesiredStateAbsent || state.TaskState != pbgo.TaskState_DONE {
		return
	}

	intent := managedAgentInstallationIntent{
		Version:                 managedAgentInstallationVersion,
		InstallationID:          state.InstallationID,
		TargetHash:              state.TargetHash,
		OperationID:             state.OperationID,
		DesiredState:            state.DesiredState,
		AcknowledgedOperationID: state.AcknowledgedOperationID,
		Bootstrap:               state.Bootstrap,
	}
	request := d.newManagedAgentInstallationRequest(intent, state.ConfigID, state.Digest)
	d.registerManagedAgentInstallationConfig(state.ConfigID, json.RawMessage(`{}`), managedAgentInstallationDesiredStateAbsent)

	d.transitionMu.Lock()
	_, verifyErr := d.verifyDatadogAgentUninstalled(ctx, request)
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
		ctrl.LoggerFrom(ctx).Error(err, "Failed to persist EKS add-on uninstall fence drift", "operationID", state.OperationID)
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
