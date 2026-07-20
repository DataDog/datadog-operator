// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	k8sretry "k8s.io/client-go/util/retry"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	fleetManagedByLabel                        = "fleet.datadoghq.com/managed-by"
	fleetConfigIDLabel                         = "fleet.datadoghq.com/config-id"
	fleetManagedAgentInstallationStateLabel    = "fleet.datadoghq.com/managed-agent-installation-state"
	fleetManagedAgentInstallationProviderLabel = "fleet.datadoghq.com/managed-agent-installation-provider"
	fleetInstallationIDLabel                   = "fleet.datadoghq.com/installation-id"
	fleetTargetIDLabel                         = "fleet.datadoghq.com/target-id"
	fleetConfigHashAnnotation                  = "fleet.datadoghq.com/config-hash"
	fleetCreateTaskIDAnnotation                = "fleet.datadoghq.com/create-task-id"
	fleetManagedByValue                        = "fleet-automation"
	fleetManagedAgentInstallationStatePartial  = "partial"
	fleetManagedAgentInstallationStateReady    = "ready"
	fleetPartialConfigVersionPrefix            = "partial:"

	fleetDatadogAgentNamespace = "datadog"
	fleetDatadogAgentName      = "datadog-agent"
	fleetCredentialSecretName  = "datadog-secret"
	fleetCredentialAPIKey      = "api-key"
)

var managedAgentInstallationDeletePollInterval = time.Second
var managedAgentInstallationDeleteTimeout = 3 * time.Minute

var managedAgentInstallationCredentialKey = types.NamespacedName{
	Namespace: fleetDatadogAgentNamespace,
	Name:      fleetCredentialSecretName,
}

var allowedManagedAgentInstallationSites = map[string]struct{}{
	"datadoghq.com":     {},
	"datadoghq.eu":      {},
	"us3.datadoghq.com": {},
	"us5.datadoghq.com": {},
	"ddog-gov.com":      {},
	"ap1.datadoghq.com": {},
	"ap2.datadoghq.com": {},
}

type datadogAgentManagedAgentInstallationConfig struct {
	Spec *v2alpha1.DatadogAgentSpec `json:"spec"`
}

type managedAgentInstallationCredentialNotReadyError struct {
	msg string
}

func (e *managedAgentInstallationCredentialNotReadyError) Error() string {
	return e.msg
}

func (d *Daemon) installDatadogAgent(ctx context.Context, command managedAgentInstallationCommand) (*pendingOperation, error) {
	target := managedAgentInstallationTarget
	configID := command.Intent.OperationID
	spec, specErr := buildFleetDatadogAgentSpec(command.Config)
	if specErr != nil {
		return nil, fmt.Errorf("create DatadogAgent: %w", specErr)
	}

	if err := d.validateFleetCredentialSecret(ctx); err != nil {
		return nil, fmt.Errorf("create DatadogAgent: %w", err)
	}
	d.resetManagedAgentInstallationCredentialRetries()
	if _, err := d.validateManagedAgentInstallationTarget(ctx, target); err != nil {
		return nil, err
	}
	configHash, hashErr := fleetDatadogAgentSpecHash(spec)
	if hashErr != nil {
		return nil, fmt.Errorf("create DatadogAgent: hash config: %w", hashErr)
	}

	existing := &v2alpha1.DatadogAgent{}
	getErr := d.managedAgentInstallationReader().Get(ctx, target, existing)
	if getErr == nil {
		if err := d.validateFleetDatadogAgentInstallation(existing); err != nil {
			return nil, err
		}
		if err := validateFleetDatadogAgentInstallReplay(existing, configID, spec); err != nil {
			return nil, err
		}
		d.setPackageConfigVersions(packageDatadogOperator, fleetPartialConfigVersionPrefix+configID, "")
		if _, err := d.markFleetDatadogAgentPartial(ctx, target, existing.UID); err != nil {
			return nil, fmt.Errorf("create DatadogAgent: mark managed Agent installation partial before readiness revalidation: %w", err)
		}
		if err := d.ensureManagedAgentInstallationWindowsProfile(ctx, existing); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, command, existing.UID, err)
		}
		if err := d.markFleetDatadogAgentReady(ctx, target, existing.UID, configID); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, command, existing.UID, fmt.Errorf("create DatadogAgent: mark managed Agent installation ready: %w", err))
		}
		observed, conflictErr := d.validateManagedAgentInstallationTarget(ctx, target)
		if conflictErr != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, command, existing.UID, conflictErr)
		}
		if err := validateFleetDatadogAgentInstallCompletion(observed, existing.UID, configID); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, command, existing.UID, err)
		}
		if err := d.validateFleetDatadogAgentInstallation(observed); err != nil {
			return nil, err
		}
		if err := d.validateManagedAgentInstallationWindowsProfileExists(ctx, observed); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, command, existing.UID, err)
		}
		return nil, nil
	}
	if !apierrors.IsNotFound(getErr) {
		return nil, fmt.Errorf("create DatadogAgent: failed to check existing resource: %w", getErr)
	}

	dda := &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v2alpha1.GroupVersion.String(),
			Kind:       "DatadogAgent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      target.Name,
			Namespace: target.Namespace,
			Labels: map[string]string{
				fleetManagedByLabel:                        fleetManagedByValue,
				fleetConfigIDLabel:                         configID,
				fleetManagedAgentInstallationStateLabel:    fleetManagedAgentInstallationStatePartial,
				fleetManagedAgentInstallationProviderLabel: string(d.managedAgentInstallationIdentity.Provider()),
				fleetInstallationIDLabel:                   command.Intent.InstallationID,
				fleetTargetIDLabel:                         d.managedAgentInstallationIdentity.TargetID(),
			},
			Annotations: map[string]string{
				fleetConfigHashAnnotation:   configHash,
				fleetCreateTaskIDAnnotation: command.Intent.OperationID,
			},
		},
		Spec: *spec,
	}
	if createErr := d.client.Create(ctx, dda, client.FieldOwner("fleet-daemon")); createErr != nil {
		if !apierrors.IsAlreadyExists(createErr) && !isRetryable(createErr) {
			return nil, fmt.Errorf("create DatadogAgent: %w", createErr)
		}
		current := &v2alpha1.DatadogAgent{}
		if getErr := k8sretry.OnError(k8sretry.DefaultBackoff, func(getErr error) bool {
			return apierrors.IsNotFound(getErr) || isRetryable(getErr)
		}, func() error {
			return d.managedAgentInstallationReader().Get(ctx, target, current)
		}); getErr != nil {
			return nil, fmt.Errorf("create DatadogAgent: create returned %w and the resource could not be recovered: %s", createErr, getErr.Error())
		}
		if ownershipErr := validateFleetOwnedDatadogAgent(current, configID); ownershipErr != nil {
			return nil, ownershipErr
		}
		if ownershipErr := d.validateFleetDatadogAgentInstallation(current); ownershipErr != nil {
			return nil, ownershipErr
		}
		d.setPackageConfigVersions(packageDatadogOperator, fleetPartialConfigVersionPrefix+configID, "")
		if validateErr := validateFleetDatadogAgentInstallReplay(current, configID, spec); validateErr != nil {
			return nil, validateErr
		}
		_, conflictErr := d.validateManagedAgentInstallationTarget(ctx, target)
		if conflictErr != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, command, current.UID, conflictErr)
		}
		return nil, fmt.Errorf("create DatadogAgent: create returned %w; recovered Fleet-owned resource remains partial for retry or explicit uninstall", createErr)
	}
	d.publishFleetDatadogAgentManagedAgentInstallationState(packageDatadogOperator, dda, configID)
	_, conflictErr := d.validateManagedAgentInstallationTarget(ctx, target)
	if conflictErr != nil {
		return nil, d.retainFleetDatadogAgentPartial(ctx, command, dda.UID, conflictErr)
	}
	acceptedHash, acceptedHashErr := fleetDatadogAgentSpecHash(&dda.Spec)
	if acceptedHashErr != nil {
		return nil, fmt.Errorf("create DatadogAgent: hash accepted spec: %w", acceptedHashErr)
	}
	if err := d.recordFleetDatadogAgentSpecHash(ctx, target, dda.UID, configID, acceptedHash); err != nil {
		return nil, fmt.Errorf("create DatadogAgent: record accepted spec: %w", err)
	}
	if err := d.ensureManagedAgentInstallationWindowsProfile(ctx, dda); err != nil {
		return nil, d.retainFleetDatadogAgentPartial(ctx, command, dda.UID, err)
	}
	if err := d.markFleetDatadogAgentReady(ctx, target, dda.UID, configID); err != nil {
		return nil, fmt.Errorf("create DatadogAgent: mark managed Agent installation ready: %w", err)
	}
	observed, conflictErr := d.validateManagedAgentInstallationTarget(ctx, target)
	if conflictErr != nil {
		return nil, d.retainFleetDatadogAgentPartial(ctx, command, dda.UID, conflictErr)
	}
	if err := validateFleetDatadogAgentInstallCompletion(observed, dda.UID, configID); err != nil {
		return nil, d.retainFleetDatadogAgentPartial(ctx, command, dda.UID, err)
	}
	if err := d.validateFleetDatadogAgentInstallation(observed); err != nil {
		return nil, err
	}
	if err := d.validateManagedAgentInstallationWindowsProfileExists(ctx, observed); err != nil {
		return nil, d.retainFleetDatadogAgentPartial(ctx, command, dda.UID, err)
	}

	ctrl.LoggerFrom(ctx).Info("Created Fleet-managed DatadogAgent", "namespace", dda.Namespace, "name", dda.Name, "config", configID)
	return nil, nil
}

func (d *Daemon) retainFleetDatadogAgentPartial(ctx context.Context, command managedAgentInstallationCommand, uid types.UID, cause error) error {
	d.setPackageConfigVersions(packageDatadogOperator, fleetPartialConfigVersionPrefix+command.Intent.OperationID, "")
	configID, err := d.markFleetDatadogAgentPartial(ctx, managedAgentInstallationTarget, uid)
	if err != nil {
		return fmt.Errorf("%w; failed to retain the Fleet-managed DatadogAgent as partial: %w", cause, err)
	}
	d.setPackageConfigVersions(packageDatadogOperator, fleetPartialConfigVersionPrefix+configID, "")
	return cause

}

func (d *Daemon) uninstallDatadogAgent(ctx context.Context) (*pendingOperation, error) {
	target := managedAgentInstallationTarget
	if _, err := d.validateManagedAgentInstallationTarget(ctx, target); err != nil {
		return nil, err
	}

	dda := &v2alpha1.DatadogAgent{}
	getErr := d.managedAgentInstallationReader().Get(ctx, target, dda)
	if apierrors.IsNotFound(getErr) {
		if err := d.waitForManagedAgentInstallationResourcesAbsent(ctx, target, ""); err != nil {
			return nil, fmt.Errorf("delete DatadogAgent: waiting for remaining resource removal: %w", err)
		}
		return nil, nil
	}
	if getErr != nil {
		return nil, fmt.Errorf("delete DatadogAgent: failed to get resource: %w", getErr)
	}
	owned, ownershipErr := classifyFleetDatadogAgentOwnership(dda)
	if ownershipErr != nil {
		return nil, ownershipErr
	}
	if !owned {
		return nil, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not owned by Fleet Automation", dda.Namespace, dda.Name)}
	}
	if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
		return nil, err
	}
	if _, err := d.validateManagedAgentInstallationTarget(ctx, target); err != nil {
		return nil, err
	}

	preconditions := metav1.Preconditions{UID: &dda.UID}
	if err := d.client.Delete(ctx, dda, client.Preconditions(preconditions), client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete DatadogAgent: %w", err)
	}
	if err := d.waitForManagedAgentInstallationResourcesAbsent(ctx, target, dda.UID); err != nil {
		return nil, fmt.Errorf("delete DatadogAgent: waiting for resource removal: %w", err)
	}

	ctrl.LoggerFrom(ctx).Info("Deleted Fleet-managed DatadogAgent", "namespace", dda.Namespace, "name", dda.Name)
	return nil, nil
}

func buildFleetDatadogAgentSpec(raw json.RawMessage) (*v2alpha1.DatadogAgentSpec, error) {
	config, err := decodeRemoteDatadogAgentConfig(raw, true)
	if err != nil {
		return nil, err
	}
	if config.Spec.Global == nil {
		config.Spec.Global = &v2alpha1.GlobalConfig{}
	}
	config.Spec.Global.Credentials = &v2alpha1.DatadogCredentials{
		APISecret: &v2alpha1.SecretConfig{
			SecretName: fleetCredentialSecretName,
			KeyName:    fleetCredentialAPIKey,
		},
	}
	return config.Spec, nil
}

func decodeRemoteDatadogAgentConfig(raw json.RawMessage, requireSpec bool) (*datadogAgentManagedAgentInstallationConfig, error) {
	if err := rejectRemoteManagedAgentInstallationCredentialFields(raw); err != nil {
		return nil, err
	}
	var config datadogAgentManagedAgentInstallationConfig
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return nil, fmt.Errorf("invalid config: trailing JSON content")
	}
	if config.Spec == nil {
		if requireSpec {
			return nil, fmt.Errorf("config must contain spec")
		}
		return &config, nil
	}
	return &config, nil
}

func rejectRemoteManagedAgentInstallationCredentialFields(raw json.RawMessage) error {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	rawSpec, ok := config["spec"]
	if !ok {
		return nil
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(rawSpec, &spec); err != nil {
		return fmt.Errorf("invalid config.spec: %w", err)
	}
	rawGlobal, ok := spec["global"]
	if !ok {
		return nil
	}
	var global map[string]json.RawMessage
	if err := json.Unmarshal(rawGlobal, &global); err != nil {
		return fmt.Errorf("invalid config.spec.global: %w", err)
	}
	if _, ok := global["credentials"]; ok {
		return fmt.Errorf("config must not contain spec.global.credentials")
	}
	if _, ok := global["clusterAgentToken"]; ok {
		return fmt.Errorf("config must not contain cluster Agent credentials")
	}
	if _, ok := global["clusterAgentTokenSecret"]; ok {
		return fmt.Errorf("config must not contain cluster Agent credentials")
	}
	return nil
}

func fleetDatadogAgentSpecHash(spec *v2alpha1.DatadogAgentSpec) (string, error) {
	encoded, err := json.Marshal(spec)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", sha256.Sum256(encoded)), nil
}

func validateFleetDatadogAgentInstallReplay(dda *v2alpha1.DatadogAgent, configID string, desired *v2alpha1.DatadogAgentSpec) error {
	if err := validateFleetOwnedDatadogAgent(dda, configID); err != nil {
		return err
	}
	if err := validateNoActiveFleetExperiment(dda); err != nil {
		return err
	}
	if !dda.DeletionTimestamp.IsZero() {
		return fmt.Errorf("create DatadogAgent: %s/%s is terminating", dda.Namespace, dda.Name)
	}
	if err := validateFleetDatadogAgentCredentials(dda); err != nil {
		return err
	}
	if !apiequality.Semantic.DeepDerivative(*desired, dda.Spec) {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s spec does not match Fleet config %q", dda.Namespace, dda.Name, configID)}
	}
	liveHash, err := fleetDatadogAgentSpecHash(&dda.Spec)
	if err != nil {
		return fmt.Errorf("hash live DatadogAgent spec: %w", err)
	}
	if dda.Annotations[fleetConfigHashAnnotation] != liveHash {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s spec changed after Fleet config %q was accepted", dda.Namespace, dda.Name, configID)}
	}
	return nil
}

func validateFleetDatadogAgentAcceptedSpec(dda *v2alpha1.DatadogAgent) error {
	return validateFleetDatadogAgentSpecHash(dda, fleetConfigHashAnnotation, "Fleet config")
}

func validateFleetDatadogAgentSpecHash(dda *v2alpha1.DatadogAgent, annotation, source string) error {
	if err := validateFleetDatadogAgentCredentials(dda); err != nil {
		return err
	}
	liveHash, err := fleetDatadogAgentSpecHash(&dda.Spec)
	if err != nil {
		return fmt.Errorf("hash live DatadogAgent spec: %w", err)
	}
	if dda.Annotations[annotation] != liveHash {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s spec does not match its accepted %s", dda.Namespace, dda.Name, source)}
	}
	return nil
}

func (d *Daemon) recordFleetDatadogAgentSpecHash(ctx context.Context, nsn types.NamespacedName, uid types.UID, configID, acceptedHash string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if uid == "" || dda.UID != uid {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before the accepted Fleet config hash could be recorded", dda.Namespace, dda.Name)}
		}
		if err := validateFleetOwnedDatadogAgent(dda, configID); err != nil {
			return err
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		liveHash, err := fleetDatadogAgentSpecHash(&dda.Spec)
		if err != nil {
			return err
		}
		if liveHash != acceptedHash {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s spec changed before the accepted Fleet config hash could be recorded", dda.Namespace, dda.Name)}
		}
		if dda.Annotations[fleetConfigHashAnnotation] == liveHash {
			return nil
		}
		base := dda.DeepCopy()
		if dda.Annotations == nil {
			dda.Annotations = make(map[string]string)
		}
		dda.Annotations[fleetConfigHashAnnotation] = liveHash
		return d.client.Patch(
			ctx,
			dda,
			client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
			client.FieldOwner("fleet-daemon"),
		)
	})
}

func (d *Daemon) persistManagedAgentInstallationStableConfig(ctx context.Context, nsn types.NamespacedName, experimentID, configID string) error {
	if !d.managedAgentInstallationIdentity.Configured() {
		return nil
	}
	if experimentID == "" || configID == "" {
		return fmt.Errorf("promoted managed Agent installation config is incomplete")
	}
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		if err := validateFleetDatadogAgentExperimentOperationState(dda, pendingIntentPromote); err != nil {
			return err
		}
		if !experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhasePromoted) {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has not promoted experiment %q", dda.Namespace, dda.Name, experimentID)}
		}
		liveHash, err := fleetDatadogAgentSpecHash(&dda.Spec)
		if err != nil {
			return err
		}
		if dda.Labels[fleetConfigIDLabel] == configID && dda.Annotations[fleetConfigHashAnnotation] == liveHash {
			return nil
		}
		base := dda.DeepCopy()
		if dda.Labels == nil {
			dda.Labels = make(map[string]string)
		}
		if dda.Annotations == nil {
			dda.Annotations = make(map[string]string)
		}
		dda.Labels[fleetConfigIDLabel] = configID
		dda.Annotations[fleetConfigHashAnnotation] = liveHash
		return d.client.Patch(
			ctx,
			dda,
			client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
			client.FieldOwner("fleet-daemon"),
		)
	})
}

func (d *Daemon) publishFleetDatadogAgentManagedAgentInstallationState(packageName string, dda *v2alpha1.DatadogAgent, configID string) {
	reportedConfigID := configID
	if dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		reportedConfigID = fleetPartialConfigVersionPrefix + configID
	}
	d.setPackageConfigVersions(packageName, reportedConfigID, "")
}

func (d *Daemon) markFleetDatadogAgentReady(ctx context.Context, nsn types.NamespacedName, uid types.UID, configID string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if uid == "" || dda.UID != uid {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before its managed Agent installation state could be marked ready", dda.Namespace, dda.Name)}
		}
		if err := validateFleetOwnedDatadogAgent(dda, configID); err != nil {
			return err
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		if err := validateFleetDatadogAgentAcceptedSpec(dda); err != nil {
			return err
		}
		if dda.Labels[fleetManagedAgentInstallationStateLabel] == fleetManagedAgentInstallationStateReady {
			return nil
		}
		base := dda.DeepCopy()
		if dda.Labels == nil {
			dda.Labels = make(map[string]string)
		}
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStateReady
		return d.client.Patch(
			ctx,
			dda,
			client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
			client.FieldOwner("fleet-daemon"),
		)
	})
}

func (d *Daemon) markFleetDatadogAgentPartial(ctx context.Context, nsn types.NamespacedName, uid types.UID) (string, error) {
	var configID string
	err := k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if uid == "" || dda.UID != uid {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before its managed Agent installation state could be marked partial", dda.Namespace, dda.Name)}
		}
		if dda.Labels[fleetManagedByLabel] != fleetManagedByValue || dda.Labels[fleetConfigIDLabel] == "" {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has incomplete or conflicting Fleet ownership metadata", dda.Namespace, dda.Name)}
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		configID = dda.Labels[fleetConfigIDLabel]
		if dda.Labels[fleetManagedAgentInstallationStateLabel] == fleetManagedAgentInstallationStatePartial {
			return nil
		}
		base := dda.DeepCopy()
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStatePartial
		return d.client.Patch(
			ctx,
			dda,
			client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
			client.FieldOwner("fleet-daemon"),
		)
	})
	return configID, err
}

func (d *Daemon) datadogAgentInternalClusterResourcesRemoved(ctx context.Context, partOfLabelValue string) (bool, error) {
	matchingLabels := client.MatchingLabels{
		store.OperatorStoreLabelKey:              "true",
		kubernetes.AppKubernetesPartOfLabelKey:   partOfLabelValue,
		kubernetes.AppKubernetesManageByLabelKey: "datadog-operator",
	}
	lists := []client.ObjectList{
		&rbacv1.ClusterRoleList{},
		&rbacv1.ClusterRoleBindingList{},
		&apiregistrationv1.APIServiceList{},
	}
	for _, list := range lists {
		if err := d.managedAgentInstallationReader().List(ctx, list, matchingLabels); err != nil {
			return false, err
		}
		if meta.LenList(list) != 0 {
			return false, nil
		}
	}
	return true, nil
}

func managedAgentInstallationPartOfLabelValue(key types.NamespacedName) string {
	metadata := &metav1.PartialObjectMetadata{ObjectMeta: metav1.ObjectMeta{Namespace: key.Namespace, Name: key.Name}}
	return object.NewPartOfLabelValue(metadata).String()
}

func controllerOwnerReference(apiVersion, kind, name string, uid types.UID) metav1.OwnerReference {
	return metav1.OwnerReference{
		APIVersion:         apiVersion,
		Kind:               kind,
		Name:               name,
		UID:                uid,
		Controller:         ptr.To(true),
		BlockOwnerDeletion: ptr.To(true),
	}
}

func requireManagedAgentInstallationResourceOwner(owners []metav1.OwnerReference, want metav1.OwnerReference) error {
	for _, owner := range owners {
		if owner.APIVersion == want.APIVersion && owner.Kind == want.Kind && owner.Name == want.Name && owner.UID == want.UID && owner.Controller != nil && *owner.Controller {
			return nil
		}
	}
	return fmt.Errorf("resource is not controlled by %s %s with UID %s", want.Kind, want.Name, want.UID)
}

func (d *Daemon) validateFleetCredentialSecret(ctx context.Context) error {
	secret := &corev1.Secret{}
	nsn := managedAgentInstallationCredentialKey
	if err := d.managedAgentInstallationReader().Get(ctx, nsn, secret); err != nil {
		if apierrors.IsNotFound(err) {
			return &managedAgentInstallationCredentialNotReadyError{msg: fmt.Sprintf("credential Secret %s/%s is not ready", nsn.Namespace, nsn.Name)}
		}
		return fmt.Errorf("credential Secret %s/%s is not ready: %w", nsn.Namespace, nsn.Name, err)
	}
	if len(secret.Data[fleetCredentialAPIKey]) == 0 {
		return &managedAgentInstallationCredentialNotReadyError{msg: fmt.Sprintf("credential Secret %s/%s is missing non-empty key %q", nsn.Namespace, nsn.Name, fleetCredentialAPIKey)}
	}
	return nil
}

func (d *Daemon) managedAgentInstallationReader() client.Reader {
	if d.apiReader != nil {
		return d.apiReader
	}
	return d.client
}

func (d *Daemon) validateManagedAgentInstallationTarget(ctx context.Context, target types.NamespacedName) (*v2alpha1.DatadogAgent, error) {
	dda := &v2alpha1.DatadogAgent{}
	if err := d.managedAgentInstallationReader().Get(ctx, target, dda); err != nil {
		if apierrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("get managed DatadogAgent %s/%s: %w", target.Namespace, target.Name, err)
	}
	owned, err := classifyFleetDatadogAgentOwnership(dda)
	if err != nil {
		return dda, err
	}
	if !owned {
		return dda, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not owned by Fleet Automation", dda.Namespace, dda.Name)}
	}
	if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
		return dda, err
	}
	return dda, nil
}

func validateFleetDatadogAgentInstallCompletion(dda *v2alpha1.DatadogAgent, uid types.UID, configID string) error {
	if dda == nil {
		return &stateDoesntMatchError{msg: "Fleet-managed DatadogAgent disappeared before install completion"}
	}
	if dda.UID != uid {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before install completion", dda.Namespace, dda.Name)}
	}
	if err := validateFleetOwnedDatadogAgent(dda, configID); err != nil {
		return err
	}
	if err := validateFleetDatadogAgentAcceptedSpec(dda); err != nil {
		return err
	}
	if dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not ready at install completion", dda.Namespace, dda.Name)}
	}
	return nil
}

func (d *Daemon) waitForManagedAgentInstallationResourcesAbsent(ctx context.Context, target types.NamespacedName, deletedUID types.UID) error {
	return wait.PollUntilContextTimeout(ctx, managedAgentInstallationDeletePollInterval, managedAgentInstallationDeleteTimeout, true, func(ctx context.Context) (bool, error) {
		return d.managedAgentInstallationResourcesAbsent(ctx, target, deletedUID)
	})
}

func (d *Daemon) managedAgentInstallationResourcesAbsent(ctx context.Context, target types.NamespacedName, deletedUID types.UID) (bool, error) {
	dda := &v2alpha1.DatadogAgent{}
	if err := d.managedAgentInstallationReader().Get(ctx, target, dda); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("get DatadogAgent %s/%s after managed Agent uninstall: %w", target.Namespace, target.Name, err)
		}
	} else {
		if deletedUID == "" || dda.UID != deletedUID {
			return false, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was recreated while uninstalling", dda.Namespace, dda.Name)}
		}
		return false, nil
	}

	profile := &v1alpha1.DatadogAgentProfile{}
	if err := d.managedAgentInstallationReader().Get(ctx, managedAgentInstallationWindowsProfileKey, profile); err != nil {
		if !apierrors.IsNotFound(err) {
			return false, fmt.Errorf("read Windows DatadogAgentProfile after managed Agent uninstall: %w", err)
		}
	} else {
		return false, nil
	}

	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err := d.managedAgentInstallationReader().List(ctx, ddais, client.MatchingLabels{
		fleetManagedAgentInstallationProviderLabel: string(d.managedAgentInstallationIdentity.Provider()),
		fleetInstallationIDLabel:                   d.managedAgentInstallationIdentity.InstallationID(),
		fleetTargetIDLabel:                         d.managedAgentInstallationIdentity.TargetID(),
	}); err != nil {
		return false, fmt.Errorf("list managed DatadogAgentInternal resources: %w", err)
	}
	if len(ddais.Items) != 0 {
		return false, nil
	}
	removed, err := d.datadogAgentInternalClusterResourcesRemoved(ctx, managedAgentInstallationPartOfLabelValue(target))
	if err != nil {
		return false, fmt.Errorf("list managed DatadogAgent dependents: %w", err)
	}
	return removed, nil
}

func validateFleetDatadogAgentCredentials(dda *v2alpha1.DatadogAgent) error {
	if dda.Spec.Global == nil || dda.Spec.Global.Credentials == nil || dda.Spec.Global.Credentials.APISecret == nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is missing the Fleet-managed API Secret reference", dda.Namespace, dda.Name)}
	}
	credentials := dda.Spec.Global.Credentials
	apiSecret := credentials.APISecret
	if apiSecret.SecretName != fleetCredentialSecretName || apiSecret.KeyName != fleetCredentialAPIKey ||
		credentials.APIKey != nil || credentials.AppKey != nil || credentials.AppSecret != nil {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s does not use the Fleet-managed API Secret reference", dda.Namespace, dda.Name)}
	}
	return nil
}

func validateFleetDatadogAgentManagedAgentInstallationReady(dda *v2alpha1.DatadogAgent) error {
	return validateFleetDatadogAgentExperimentOperationState(dda, pendingIntentStart)
}

func validateFleetDatadogAgentExperimentOperationState(dda *v2alpha1.DatadogAgent, intent pendingIntent) error {
	owned, err := classifyFleetDatadogAgentOwnership(dda)
	if err != nil {
		return err
	}
	if !owned {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not managed by Fleet Automation", dda.Namespace, dda.Name)}
	}
	state := dda.Labels[fleetManagedAgentInstallationStateLabel]
	if state == fleetManagedAgentInstallationStateReady {
		return nil
	}
	if intent == pendingIntentStop && state == fleetManagedAgentInstallationStatePartial {
		exp := dda.Status.Experiment
		current, pending := pendingOperationFromAnnotations(client.ObjectKeyFromObject(dda), dda.Annotations)
		if (exp != nil && exp.ID != "" && !isTerminalPhase(exp.Phase)) ||
			dda.Annotations[v2alpha1.AnnotationExperimentID] != "" ||
			(pending && current.intent == pendingIntentStop) {
			return nil
		}
	}
	if intent == pendingIntentPromote && state == fleetManagedAgentInstallationStatePartial {
		current, pending := pendingOperationFromAnnotations(client.ObjectKeyFromObject(dda), dda.Annotations)
		if pending && current.intent == pendingIntentPromote &&
			experimentHasPhase(dda, current.experimentID, v2alpha1.ExperimentPhasePromoted) {
			return nil
		}
	}
	if state != fleetManagedAgentInstallationStateReady {
		return &stateDoesntMatchError{msg: fmt.Sprintf("Fleet-managed DatadogAgent %s/%s has not completed its install gate", dda.Namespace, dda.Name)}
	}
	return nil
}

func classifyFleetDatadogAgentOwnership(dda *v2alpha1.DatadogAgent) (bool, error) {
	managedBy, hasManagedBy := dda.Labels[fleetManagedByLabel]
	configID, hasConfigID := dda.Labels[fleetConfigIDLabel]
	managedAgentInstallationState, hasManagedAgentInstallationState := dda.Labels[fleetManagedAgentInstallationStateLabel]
	_, hasProvider := dda.Labels[fleetManagedAgentInstallationProviderLabel]
	_, hasInstallationID := dda.Labels[fleetInstallationIDLabel]
	_, hasTargetID := dda.Labels[fleetTargetIDLabel]
	_, hasConfigHash := dda.Annotations[fleetConfigHashAnnotation]
	_, hasCreateTaskID := dda.Annotations[fleetCreateTaskIDAnnotation]
	if !hasManagedBy && !hasConfigID && !hasManagedAgentInstallationState && !hasProvider && !hasInstallationID && !hasTargetID && !hasConfigHash && !hasCreateTaskID {
		return false, nil
	}
	if managedBy != fleetManagedByValue || configID == "" || !hasConfigHash ||
		(managedAgentInstallationState != fleetManagedAgentInstallationStatePartial && managedAgentInstallationState != fleetManagedAgentInstallationStateReady) {
		return false, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has incomplete or conflicting Fleet ownership metadata", dda.Namespace, dda.Name)}
	}
	return true, nil
}

func classifyFleetDatadogAgentOwnershipForRehydration(dda *v2alpha1.DatadogAgent) (bool, error) {
	managedBy, hasManagedBy := dda.Labels[fleetManagedByLabel]
	configID, hasConfigID := dda.Labels[fleetConfigIDLabel]
	_, hasManagedAgentInstallationState := dda.Labels[fleetManagedAgentInstallationStateLabel]
	_, hasProvider := dda.Labels[fleetManagedAgentInstallationProviderLabel]
	_, hasInstallationID := dda.Labels[fleetInstallationIDLabel]
	_, hasTargetID := dda.Labels[fleetTargetIDLabel]
	_, hasConfigHash := dda.Annotations[fleetConfigHashAnnotation]
	_, hasCreateTaskID := dda.Annotations[fleetCreateTaskIDAnnotation]
	if !hasManagedBy && !hasConfigID && !hasManagedAgentInstallationState && !hasProvider && !hasInstallationID && !hasTargetID && !hasConfigHash && !hasCreateTaskID {
		return false, nil
	}
	if managedBy != fleetManagedByValue || configID == "" {
		return false, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has incomplete or conflicting Fleet ownership metadata", dda.Namespace, dda.Name)}
	}
	return true, nil
}

func validateFleetOwnedDatadogAgent(dda *v2alpha1.DatadogAgent, expectedConfigID string) error {
	if dda.Labels[fleetManagedByLabel] != fleetManagedByValue || dda.Labels[fleetConfigIDLabel] == "" {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not owned by Fleet Automation", dda.Namespace, dda.Name)}
	}
	if expectedConfigID != "" && dda.Labels[fleetConfigIDLabel] != expectedConfigID {
		return &stateDoesntMatchError{msg: fmt.Sprintf(
			"DatadogAgent %s/%s is managed by Fleet Automation with config %q, not %q; use an update operation",
			dda.Namespace,
			dda.Name,
			dda.Labels[fleetConfigIDLabel],
			expectedConfigID,
		)}
	}
	return nil
}

func (d *Daemon) validateFleetDatadogAgentInstallation(dda *v2alpha1.DatadogAgent) error {
	provider := dda.Labels[fleetManagedAgentInstallationProviderLabel]
	installationID := dda.Labels[fleetInstallationIDLabel]
	targetID := dda.Labels[fleetTargetIDLabel]
	if !d.managedAgentInstallationIdentity.Configured() {
		if provider != "" || installationID != "" || targetID != "" {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has managed Agent installation identity metadata but the local managed Agent installation identity is not configured", dda.Namespace, dda.Name)}
		}
		return nil
	}
	if provider != string(d.managedAgentInstallationIdentity.Provider()) {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s belongs to a different managed installation provider", dda.Namespace, dda.Name)}
	}
	if installationID != d.managedAgentInstallationIdentity.InstallationID() {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s belongs to a different managed installation", dda.Namespace, dda.Name)}
	}
	if targetID != d.managedAgentInstallationIdentity.TargetID() {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s belongs to a different managed target", dda.Namespace, dda.Name)}
	}
	return nil
}

func validateNoActiveFleetExperiment(dda *v2alpha1.DatadogAgent) error {
	if _, ok := pendingOperationFromAnnotations(client.ObjectKeyFromObject(dda), dda.Annotations); ok {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has a pending Fleet experiment task", dda.Namespace, dda.Name)}
	}
	if dda.Status.Experiment != nil && !isTerminalPhase(dda.Status.Experiment.Phase) {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has an active Fleet experiment", dda.Namespace, dda.Name)}
	}
	return nil
}
