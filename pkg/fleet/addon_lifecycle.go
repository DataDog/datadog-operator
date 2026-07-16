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
	addonLifecycleIntentConfigMapName = "datadog-agent-lifecycle-intent"
	addonLifecycleIntentDataKey       = "intent.json"
	addonLifecycleStateConfigMapName  = "datadog-agent-lifecycle-state"
	addonLifecycleVersion             = "v1"
	addonLifecycleMaxIntentSize       = 256 * 1024

	addonLifecycleStateInstallationIDKey = "installation_id"
	addonLifecycleStateEKSARNHashKey     = "eks_arn_sha256"
	addonLifecycleStateOperationIDKey    = "operation_id"
	addonLifecycleStateDigestKey         = "intent_digest"
	addonLifecycleStateDesiredStateKey   = "desired_state"
	addonLifecycleStateBootstrapKey      = "bootstrap"
	addonLifecycleStateAcknowledgedKey   = "acknowledged_operation_id"
	addonLifecycleStateConfigIDKey       = "config_id"
	addonLifecycleStateTaskStateKey      = "task_state"
	addonLifecycleStateErrorKey          = "error"
)

var (
	addonLifecycleIntentKey = types.NamespacedName{
		Namespace: fleetDatadogAgentNamespace,
		Name:      addonLifecycleIntentConfigMapName,
	}
	addonLifecycleStateKey = types.NamespacedName{
		Namespace: fleetDatadogAgentNamespace,
		Name:      addonLifecycleStateConfigMapName,
	}
	addonLifecycleFenceMonitorInterval = 10 * time.Second
	addonLifecycleRetryInterval        = time.Second
)

type addonLifecycleDesiredState string

const (
	addonLifecycleDesiredStateInstalled addonLifecycleDesiredState = "installed"
	addonLifecycleDesiredStateAbsent    addonLifecycleDesiredState = "absent"
)

type addonLifecycleBootstrap struct {
	ClusterName string `json:"clusterName"`
	Site        string `json:"site"`
}

type addonLifecycleIntent struct {
	Version                 string                     `json:"version"`
	InstallationID          string                     `json:"installationID"`
	EKSARNHash              string                     `json:"eksARNSHA256"`
	OperationID             string                     `json:"operationID"`
	DesiredState            addonLifecycleDesiredState `json:"desiredState"`
	AcknowledgedOperationID string                     `json:"acknowledgedOperationID,omitempty"`
	Bootstrap               addonLifecycleBootstrap    `json:"bootstrap"`
}

type normalizedAddonLifecycleIntent struct {
	Version        string                     `json:"version"`
	InstallationID string                     `json:"installationID"`
	EKSARNHash     string                     `json:"eksARNSHA256"`
	OperationID    string                     `json:"operationID"`
	DesiredState   addonLifecycleDesiredState `json:"desiredState"`
	Bootstrap      addonLifecycleBootstrap    `json:"bootstrap"`
}

type addonLifecycleIntentSnapshot struct {
	raw             []byte
	resourceVersion string
}

type addonLifecycleRequestMetadata struct {
	Digest                  string
	DesiredState            addonLifecycleDesiredState
	ConfigID                string
	EKSARNHash              string
	Bootstrap               addonLifecycleBootstrap
	AcknowledgedOperationID string
}

type addonLifecyclePersistedState struct {
	InstallationID          string
	EKSARNHash              string
	OperationID             string
	Digest                  string
	DesiredState            addonLifecycleDesiredState
	Bootstrap               addonLifecycleBootstrap
	AcknowledgedOperationID string
	ConfigID                string
	TaskState               pbgo.TaskState
	Error                   string
}

func ReconcileAddonLifecycleAcknowledgement(ctx context.Context, kubeClient client.Client, reader client.Reader, identity remoteconfig.LifecycleIdentity) error {
	intentConfigMap := &corev1.ConfigMap{}
	if err := reader.Get(ctx, addonLifecycleIntentKey, intentConfigMap); err != nil {
		return fmt.Errorf("read EKS add-on lifecycle intent before Remote Configuration startup: %w", err)
	}
	intent, _, digest, err := decodeAddonLifecycleIntent([]byte(intentConfigMap.Data[addonLifecycleIntentDataKey]), identity)
	if err != nil {
		return err
	}
	if intent.AcknowledgedOperationID == "" {
		return nil
	}
	daemon := &Daemon{
		client:            kubeClient,
		apiReader:         reader,
		lifecycleIdentity: identity,
	}
	current, err := daemon.readAddonLifecycleState(ctx)
	if err != nil {
		return err
	}
	if err := validateAddonLifecycleProgress(current, intent, digest); err != nil {
		return err
	}
	if current != nil && current.AcknowledgedOperationID == intent.AcknowledgedOperationID {
		return nil
	}
	return daemon.acknowledgeAddonLifecycleInstall(ctx, current, intent)
}

func (d *Daemon) installAddonLifecycleIntentForwarder(ctx context.Context) error {
	informer, err := d.cache.GetInformer(ctx, &corev1.ConfigMap{})
	if err != nil {
		return fmt.Errorf("get ConfigMap informer for EKS add-on lifecycle intent: %w", err)
	}
	informer.AddEventHandler(toolscache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			d.forwardAddonLifecycleIntent(ctx, obj)
		},
		UpdateFunc: func(_, newObj any) {
			d.forwardAddonLifecycleIntent(ctx, newObj)
		},
	})
	return nil
}

func (d *Daemon) forwardAddonLifecycleIntent(ctx context.Context, obj any) {
	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok || client.ObjectKeyFromObject(configMap) != addonLifecycleIntentKey {
		return
	}
	snapshot := addonLifecycleIntentSnapshot{
		raw:             []byte(configMap.Data[addonLifecycleIntentDataKey]),
		resourceVersion: configMap.ResourceVersion,
	}
	select {
	case d.addonLifecycleUpdates <- snapshot:
	case <-ctx.Done():
	}
}

func (d *Daemon) runAddonLifecycleIntentWorker(ctx context.Context) {
	logger := ctrl.LoggerFrom(ctx).WithName("eks-addon-lifecycle")
	var pending *addonLifecycleIntentSnapshot
	reload := false
	for {
		if pending == nil {
			if reload {
				snapshot, err := d.readCurrentAddonLifecycleIntent(ctx)
				if err != nil {
					logger.Error(err, "Failed to reload EKS add-on lifecycle intent")
					timer := time.NewTimer(addonLifecycleRetryInterval)
					select {
					case <-ctx.Done():
						timer.Stop()
						return
					case snapshot := <-d.addonLifecycleUpdates:
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
				case snapshot := <-d.addonLifecycleUpdates:
					pending = &snapshot
				case <-d.addonLifecycleRetries:
					reload = true
				}
			}
		}

		if pending == nil {
			continue
		}
		if err := d.handleAddonLifecycleIntent(ctx, *pending); err == nil {
			pending = nil
			continue
		} else {
			logger.Error(err, "Failed to reconcile EKS add-on lifecycle intent", "resourceVersion", pending.resourceVersion)
		}

		timer := time.NewTimer(addonLifecycleRetryInterval)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case snapshot := <-d.addonLifecycleUpdates:
			timer.Stop()
			pending = &snapshot
		case <-d.addonLifecycleRetries:
			timer.Stop()
			pending = nil
			reload = true
		case <-timer.C:
		}
	}
}

func (d *Daemon) readCurrentAddonLifecycleIntent(ctx context.Context) (addonLifecycleIntentSnapshot, error) {
	configMap := &corev1.ConfigMap{}
	if err := d.lifecycleReader().Get(ctx, addonLifecycleIntentKey, configMap); err != nil {
		return addonLifecycleIntentSnapshot{}, fmt.Errorf("read current EKS add-on lifecycle intent: %w", err)
	}
	return addonLifecycleIntentSnapshot{
		raw:             []byte(configMap.Data[addonLifecycleIntentDataKey]),
		resourceVersion: configMap.ResourceVersion,
	}, nil
}

func (d *Daemon) requestAddonLifecycleRetry() {
	if d.addonLifecycleRetries == nil {
		return
	}
	select {
	case d.addonLifecycleRetries <- struct{}{}:
	default:
	}
}

func (d *Daemon) handleAddonLifecycleIntent(ctx context.Context, snapshot addonLifecycleIntentSnapshot) error {
	intent, config, digest, err := decodeAddonLifecycleIntent(snapshot.raw, d.lifecycleIdentity)
	if err != nil {
		return err
	}
	if err := d.refreshLifecycleUpdaterTags(ctx); err != nil {
		return err
	}
	configID := intent.OperationID

	current, err := d.readAddonLifecycleState(ctx)
	if err != nil {
		return err
	}
	if err := validateAddonLifecycleProgress(current, intent, digest); err != nil {
		d.taskMu.Lock()
		d.setTaskState(packageDatadogOperator, intent.OperationID, pbgo.TaskState_INVALID_STATE, err)
		d.taskMu.Unlock()
		return err
	}
	if intent.DesiredState == addonLifecycleDesiredStateInstalled && intent.AcknowledgedOperationID != "" {
		if current != nil && current.AcknowledgedOperationID == intent.AcknowledgedOperationID {
			return nil
		}
		return d.acknowledgeAddonLifecycleInstall(ctx, current, intent)
	}
	if current != nil && current.OperationID == intent.OperationID && current.TaskState != pbgo.TaskState_RUNNING {
		return nil
	}
	if err := d.waitForAddonLifecycleSlot(ctx, intent); err != nil {
		return err
	}
	current, err = d.readAddonLifecycleState(ctx)
	if err != nil {
		return err
	}
	if err := validateAddonLifecycleProgress(current, intent, digest); err != nil {
		return err
	}
	if current != nil && current.OperationID == intent.OperationID && current.TaskState != pbgo.TaskState_RUNNING {
		return nil
	}
	request := d.newAddonLifecycleRequest(intent, configID, digest)
	d.registerAddonLifecycleConfig(configID, config, intent.DesiredState)
	if err := d.handleLifecycleTask(ctx, request); err != nil {
		return err
	}
	return nil
}

func (d *Daemon) waitForAddonLifecycleSlot(ctx context.Context, intent addonLifecycleIntent) error {
	d.taskMu.Lock()
	if !d.lifecycleActive {
		d.taskMu.Unlock()
		return nil
	}
	activeOperationID := d.lifecycleOperationID
	cancel := d.lifecycleCancel
	done := d.lifecycleDone
	d.taskMu.Unlock()

	if intent.DesiredState == addonLifecycleDesiredStateAbsent && intent.OperationID != activeOperationID && cancel != nil {
		cancel()
	}
	if done == nil {
		return fmt.Errorf("active EKS add-on lifecycle operation has no completion signal")
	}
	select {
	case <-ctx.Done():
		return fmt.Errorf("waiting for active EKS add-on lifecycle operation: %w", ctx.Err())
	case <-done:
		return nil
	}
}

func (d *Daemon) acknowledgeAddonLifecycleInstall(ctx context.Context, current *addonLifecyclePersistedState, intent addonLifecycleIntent) error {
	if current == nil || current.DesiredState != addonLifecycleDesiredStateInstalled || current.OperationID != intent.OperationID || current.TaskState != pbgo.TaskState_DONE {
		return fmt.Errorf("EKS add-on lifecycle install cannot be acknowledged before matching completion")
	}
	if current.AcknowledgedOperationID != "" && current.AcknowledgedOperationID != intent.OperationID {
		return fmt.Errorf("EKS add-on lifecycle install is already acknowledged by a different operation")
	}
	dda := &v2alpha1.DatadogAgent{}
	if err := d.lifecycleReader().Get(ctx, types.NamespacedName{Namespace: fleetDatadogAgentNamespace, Name: fleetDatadogAgentName}, dda); err != nil {
		return fmt.Errorf("read DatadogAgent before bootstrap acknowledgement: %w", err)
	}
	if err := validateFleetDatadogAgentInstallCompletion(dda, dda.UID, current.ConfigID); err != nil {
		return err
	}
	if err := d.validateAddonLifecycleResourcesReady(ctx, dda); err != nil {
		return err
	}
	current.AcknowledgedOperationID = intent.OperationID
	if err := d.writeAddonLifecycleState(ctx, *current); err != nil {
		return fmt.Errorf("persist EKS add-on lifecycle acknowledgement: %w", err)
	}
	if err := d.refreshLifecycleUpdaterTags(ctx); err != nil {
		return err
	}
	d.taskMu.Lock()
	d.lifecycleTaskReserved = false
	d.taskMu.Unlock()
	return nil
}

func (d *Daemon) refreshLifecycleUpdaterTags(ctx context.Context) error {
	if d.rcClient == nil {
		return nil
	}
	if err := d.rcClient.RefreshUpdaterTags(ctx); err != nil {
		return fmt.Errorf("refresh Remote Configuration updater tags for EKS add-on lifecycle: %w", err)
	}
	return nil
}

func decodeAddonLifecycleIntent(raw []byte, identity remoteconfig.LifecycleIdentity) (addonLifecycleIntent, json.RawMessage, string, error) {
	if len(raw) == 0 {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("EKS add-on lifecycle intent is missing %q", addonLifecycleIntentDataKey)
	}
	if len(raw) > addonLifecycleMaxIntentSize {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("EKS add-on lifecycle intent exceeds %d bytes", addonLifecycleMaxIntentSize)
	}

	var intent addonLifecycleIntent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&intent); err != nil {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("decode EKS add-on lifecycle intent: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("decode EKS add-on lifecycle intent: trailing JSON content")
	}
	if intent.Version != addonLifecycleVersion {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("unsupported EKS add-on lifecycle version %q", intent.Version)
	}
	if err := validateCanonicalUUID("installation_id", intent.InstallationID); err != nil {
		return addonLifecycleIntent{}, nil, "", err
	}
	if intent.InstallationID != identity.InstallationID {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("EKS add-on lifecycle installation ID does not match the local installation")
	}
	if intent.EKSARNHash != identity.EKSARNHash {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("EKS add-on lifecycle ARN hash does not match the local installation")
	}
	if err := validateCanonicalUUID("operation_id", intent.OperationID); err != nil {
		return addonLifecycleIntent{}, nil, "", err
	}
	if intent.AcknowledgedOperationID != "" {
		if err := validateCanonicalUUID("acknowledged_operation_id", intent.AcknowledgedOperationID); err != nil {
			return addonLifecycleIntent{}, nil, "", err
		}
		if intent.DesiredState == addonLifecycleDesiredStateInstalled && intent.AcknowledgedOperationID != intent.OperationID {
			return addonLifecycleIntent{}, nil, "", fmt.Errorf("EKS add-on lifecycle acknowledgement must match the active install operation")
		}
	}
	if err := validateAddonLifecycleBootstrap(intent.Bootstrap); err != nil {
		return addonLifecycleIntent{}, nil, "", err
	}

	var normalizedConfig json.RawMessage
	switch intent.DesiredState {
	case addonLifecycleDesiredStateInstalled:
		var err error
		normalizedConfig, err = addonLifecycleBootstrapConfig(intent.Bootstrap)
		if err != nil {
			return addonLifecycleIntent{}, nil, "", err
		}
	case addonLifecycleDesiredStateAbsent:
		normalizedConfig = json.RawMessage(`{}`)
	default:
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("unsupported EKS add-on lifecycle desired state %q", intent.DesiredState)
	}

	normalized := normalizedAddonLifecycleIntent{
		Version:        intent.Version,
		InstallationID: intent.InstallationID,
		EKSARNHash:     intent.EKSARNHash,
		OperationID:    intent.OperationID,
		DesiredState:   intent.DesiredState,
		Bootstrap:      intent.Bootstrap,
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return addonLifecycleIntent{}, nil, "", fmt.Errorf("encode normalized EKS add-on lifecycle intent: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return intent, normalizedConfig, hex.EncodeToString(digest[:]), nil
}

func validateAddonLifecycleBootstrap(bootstrap addonLifecycleBootstrap) error {
	if strings.TrimSpace(bootstrap.ClusterName) != bootstrap.ClusterName || bootstrap.ClusterName == "" || len(bootstrap.ClusterName) > 100 {
		return fmt.Errorf("EKS add-on lifecycle bootstrap cluster name is invalid")
	}
	if _, ok := allowedLifecycleSites[bootstrap.Site]; !ok {
		return fmt.Errorf("EKS add-on lifecycle bootstrap site %q is unsupported", bootstrap.Site)
	}
	return nil
}

func addonLifecycleBootstrapConfig(bootstrap addonLifecycleBootstrap) (json.RawMessage, error) {
	clusterName := bootstrap.ClusterName
	site := bootstrap.Site
	linuxSelector := map[string]string{corev1.LabelOSStable: string(corev1.Linux)}
	config := datadogAgentLifecycleConfig{Spec: &v2alpha1.DatadogAgentSpec{
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
		return nil, fmt.Errorf("encode EKS add-on lifecycle bootstrap config: %w", err)
	}
	return encoded, nil
}

func validateCanonicalUUID(field, value string) error {
	parsed, err := uuid.Parse(value)
	if err != nil || parsed == uuid.Nil || parsed.String() != value {
		return fmt.Errorf("EKS add-on lifecycle %s must be a canonical non-zero UUID", field)
	}
	return nil
}

func validateAddonLifecycleProgress(current *addonLifecyclePersistedState, intent addonLifecycleIntent, digest string) error {
	if current == nil {
		if intent.DesiredState != addonLifecycleDesiredStateInstalled || intent.AcknowledgedOperationID != "" {
			return fmt.Errorf("the first EKS add-on lifecycle request must be an unacknowledged install")
		}
		return nil
	}
	if current.InstallationID != intent.InstallationID || current.EKSARNHash != intent.EKSARNHash {
		return fmt.Errorf("persisted EKS add-on lifecycle state belongs to a different installation")
	}
	if intent.OperationID == current.OperationID {
		if digest != current.Digest {
			return fmt.Errorf("EKS add-on lifecycle operation %q was replayed with different content", intent.OperationID)
		}
		if current.DesiredState == addonLifecycleDesiredStateInstalled {
			switch {
			case current.AcknowledgedOperationID == "" && (intent.AcknowledgedOperationID == "" || intent.AcknowledgedOperationID == intent.OperationID):
				return nil
			case current.AcknowledgedOperationID == intent.OperationID && intent.AcknowledgedOperationID == intent.OperationID:
				return nil
			default:
				return fmt.Errorf("EKS add-on lifecycle install acknowledgement regressed or changed")
			}
		}
		if intent.AcknowledgedOperationID != current.AcknowledgedOperationID {
			return fmt.Errorf("EKS add-on lifecycle uninstall acknowledgement changed")
		}
		return nil
	}
	if current.DesiredState == addonLifecycleDesiredStateAbsent {
		return fmt.Errorf("EKS add-on lifecycle install cannot follow an accepted uninstall")
	}
	if intent.DesiredState != addonLifecycleDesiredStateAbsent {
		return fmt.Errorf("a new EKS add-on lifecycle operation must uninstall the existing installation")
	}
	if intent.AcknowledgedOperationID != current.AcknowledgedOperationID {
		return fmt.Errorf("EKS add-on lifecycle uninstall must retain the install acknowledgement")
	}
	return nil
}

func (d *Daemon) newAddonLifecycleRequest(intent addonLifecycleIntent, configID, digest string) remoteAPIRequest {
	stableConfig, experimentConfig := d.getPackageConfigVersions(packageDatadogOperator)
	clientID := ""
	if d.rcClient != nil {
		clientID = d.rcClient.GetClientID()
	}
	method := methodInstallDatadogAgent
	if intent.DesiredState == addonLifecycleDesiredStateAbsent {
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
		Addon: &addonLifecycleRequestMetadata{
			Digest:                  digest,
			DesiredState:            intent.DesiredState,
			ConfigID:                configID,
			EKSARNHash:              intent.EKSARNHash,
			Bootstrap:               intent.Bootstrap,
			AcknowledgedOperationID: intent.AcknowledgedOperationID,
		},
	}
}

func (d *Daemon) registerAddonLifecycleConfig(configID string, config json.RawMessage, desiredState addonLifecycleDesiredState) {
	operation := OperationCreate
	if desiredState == addonLifecycleDesiredStateAbsent {
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

func addonLifecycleStateFromRequest(req remoteAPIRequest, taskState pbgo.TaskState, taskErr error) addonLifecyclePersistedState {
	state := addonLifecyclePersistedState{
		InstallationID:          req.Params.InstallationID,
		EKSARNHash:              req.Addon.EKSARNHash,
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

func (d *Daemon) recordAddonLifecycleResult(ctx context.Context, req remoteAPIRequest, taskState pbgo.TaskState, taskErr error) error {
	if req.Addon == nil {
		return nil
	}
	if err := d.writeAddonLifecycleResult(ctx, addonLifecycleStateFromRequest(req, taskState, taskErr)); err != nil {
		return fmt.Errorf("persist EKS add-on lifecycle result for operation %s: %w", req.Params.OperationID, err)
	}
	return nil
}

func (d *Daemon) writeAddonLifecycleState(ctx context.Context, state addonLifecyclePersistedState) error {
	return d.writeAddonLifecycleStateForOperation(ctx, state, "")
}

func (d *Daemon) writeAddonLifecycleResult(ctx context.Context, state addonLifecyclePersistedState) error {
	return d.writeAddonLifecycleStateForOperation(ctx, state, state.OperationID)
}

func (d *Daemon) writeAddonLifecycleStateForOperation(ctx context.Context, state addonLifecyclePersistedState, expectedOperationID string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		current := &corev1.ConfigMap{}
		err := d.client.Get(ctx, addonLifecycleStateKey, current)
		if apierrors.IsNotFound(err) {
			if expectedOperationID != "" {
				return fmt.Errorf("EKS add-on lifecycle state is missing for operation %s", expectedOperationID)
			}
			intent := &corev1.ConfigMap{}
			if err := d.lifecycleReader().Get(ctx, addonLifecycleIntentKey, intent); err != nil {
				return fmt.Errorf("read EKS add-on lifecycle intent owner: %w", err)
			}
			return d.client.Create(ctx, &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: addonLifecycleStateKey.Namespace,
					Name:      addonLifecycleStateKey.Name,
					OwnerReferences: []metav1.OwnerReference{controllerOwnerReference(
						corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID,
					)},
					Labels: map[string]string{
						"app.kubernetes.io/managed-by": "datadog-operator",
					},
				},
				Data: addonLifecycleStateData(state),
			}, client.FieldOwner("fleet-daemon"))
		}
		if err != nil {
			return err
		}
		if err := d.validateAddonLifecycleStateOwner(ctx, current); err != nil {
			return err
		}
		if expectedOperationID != "" && current.Data[addonLifecycleStateOperationIDKey] != expectedOperationID {
			return nil
		}
		base := current.DeepCopy()
		current.Data = addonLifecycleStateData(state)
		return d.client.Patch(ctx, current, client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}), client.FieldOwner("fleet-daemon"))
	})
}

func addonLifecycleStateData(state addonLifecyclePersistedState) map[string]string {
	bootstrap, _ := json.Marshal(state.Bootstrap)
	return map[string]string{
		addonLifecycleStateInstallationIDKey: state.InstallationID,
		addonLifecycleStateEKSARNHashKey:     state.EKSARNHash,
		addonLifecycleStateOperationIDKey:    state.OperationID,
		addonLifecycleStateDigestKey:         state.Digest,
		addonLifecycleStateDesiredStateKey:   string(state.DesiredState),
		addonLifecycleStateBootstrapKey:      string(bootstrap),
		addonLifecycleStateAcknowledgedKey:   state.AcknowledgedOperationID,
		addonLifecycleStateConfigIDKey:       state.ConfigID,
		addonLifecycleStateTaskStateKey:      state.TaskState.String(),
		addonLifecycleStateErrorKey:          state.Error,
	}
}

func (d *Daemon) readAddonLifecycleState(ctx context.Context) (*addonLifecyclePersistedState, error) {
	configMap := &corev1.ConfigMap{}
	if err := d.lifecycleReader().Get(ctx, addonLifecycleStateKey, configMap); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read EKS add-on lifecycle state: %w", err)
	}
	if err := d.validateAddonLifecycleStateOwner(ctx, configMap); err != nil {
		return nil, err
	}
	taskState, err := parseAddonLifecycleTaskState(configMap.Data[addonLifecycleStateTaskStateKey])
	if err != nil {
		return nil, err
	}
	var bootstrap addonLifecycleBootstrap
	if err := json.Unmarshal([]byte(configMap.Data[addonLifecycleStateBootstrapKey]), &bootstrap); err != nil {
		return nil, fmt.Errorf("EKS add-on lifecycle state has invalid bootstrap: %w", err)
	}
	state := &addonLifecyclePersistedState{
		InstallationID:          configMap.Data[addonLifecycleStateInstallationIDKey],
		EKSARNHash:              configMap.Data[addonLifecycleStateEKSARNHashKey],
		OperationID:             configMap.Data[addonLifecycleStateOperationIDKey],
		Digest:                  configMap.Data[addonLifecycleStateDigestKey],
		DesiredState:            addonLifecycleDesiredState(configMap.Data[addonLifecycleStateDesiredStateKey]),
		Bootstrap:               bootstrap,
		AcknowledgedOperationID: configMap.Data[addonLifecycleStateAcknowledgedKey],
		ConfigID:                configMap.Data[addonLifecycleStateConfigIDKey],
		TaskState:               taskState,
		Error:                   configMap.Data[addonLifecycleStateErrorKey],
	}
	if state.InstallationID == "" || state.EKSARNHash == "" || state.OperationID == "" || state.Digest == "" || state.ConfigID == "" {
		return nil, fmt.Errorf("EKS add-on lifecycle state is incomplete")
	}
	if err := validateAddonLifecycleBootstrap(state.Bootstrap); err != nil {
		return nil, err
	}
	if state.DesiredState != addonLifecycleDesiredStateInstalled && state.DesiredState != addonLifecycleDesiredStateAbsent {
		return nil, fmt.Errorf("EKS add-on lifecycle state has unsupported desired state %q", state.DesiredState)
	}
	return state, nil
}

func (d *Daemon) validateAddonLifecycleStateOwner(ctx context.Context, state *corev1.ConfigMap) error {
	intent := &corev1.ConfigMap{}
	if err := d.lifecycleReader().Get(ctx, addonLifecycleIntentKey, intent); err != nil {
		return fmt.Errorf("read EKS add-on lifecycle intent owner: %w", err)
	}
	wantOwner := controllerOwnerReference(corev1.SchemeGroupVersion.String(), "ConfigMap", intent.Name, intent.UID)
	if err := requireLifecycleResourceOwner(state.OwnerReferences, wantOwner); err != nil {
		return fmt.Errorf("validate EKS add-on lifecycle state ownership: %w", err)
	}
	return nil
}

func parseAddonLifecycleTaskState(value string) (pbgo.TaskState, error) {
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
		return pbgo.TaskState(0), fmt.Errorf("EKS add-on lifecycle state has unsupported task state %q", value)
	}
}

func (d *Daemon) rehydrateAddonLifecycleState(ctx context.Context) error {
	state, err := d.readAddonLifecycleState(ctx)
	if err != nil || state == nil {
		return err
	}
	if state.InstallationID != d.lifecycleIdentity.InstallationID || state.EKSARNHash != d.lifecycleIdentity.EKSARNHash {
		return fmt.Errorf("persisted EKS add-on lifecycle state belongs to a different installation")
	}
	var taskErr error
	if state.Error != "" {
		taskErr = fmt.Errorf("%s", state.Error)
	}
	d.taskMu.Lock()
	d.lifecycleTaskReserved = !(state.AcknowledgedOperationID == state.OperationID && state.DesiredState == addonLifecycleDesiredStateInstalled)
	d.setTaskState(packageDatadogOperator, state.OperationID, state.TaskState, taskErr)
	d.taskMu.Unlock()
	return nil
}

func (d *Daemon) runAddonLifecycleFenceMonitor(ctx context.Context) {
	ticker := time.NewTicker(addonLifecycleFenceMonitorInterval)
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
	state, err := d.readAddonLifecycleState(ctx)
	if err != nil {
		ctrl.LoggerFrom(ctx).Error(err, "Failed to read EKS add-on lifecycle state during fence revalidation")
		return
	}
	if state == nil || state.DesiredState != addonLifecycleDesiredStateAbsent || state.TaskState != pbgo.TaskState_DONE {
		return
	}

	intent := addonLifecycleIntent{
		Version:                 addonLifecycleVersion,
		InstallationID:          state.InstallationID,
		EKSARNHash:              state.EKSARNHash,
		OperationID:             state.OperationID,
		DesiredState:            state.DesiredState,
		AcknowledgedOperationID: state.AcknowledgedOperationID,
		Bootstrap:               state.Bootstrap,
	}
	request := d.newAddonLifecycleRequest(intent, state.ConfigID, state.Digest)
	d.registerAddonLifecycleConfig(state.ConfigID, json.RawMessage(`{}`), addonLifecycleDesiredStateAbsent)

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
	if err := d.writeAddonLifecycleState(ctx, *state); err != nil {
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
