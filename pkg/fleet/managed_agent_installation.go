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
	"errors"
	"fmt"
	"io"
	"strings"
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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	v2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

const (
	fleetManagedByLabel                       = "fleet.datadoghq.com/managed-by"
	fleetConfigIDLabel                        = "fleet.datadoghq.com/config-id"
	fleetManagedAgentInstallationStateLabel   = "fleet.datadoghq.com/managed-agent-installation-state"
	fleetInstallationIDLabel                  = "fleet.datadoghq.com/installation-id"
	fleetTargetIDLabel                        = "fleet.datadoghq.com/target-id"
	fleetConfigHashAnnotation                 = "fleet.datadoghq.com/config-hash"
	fleetExperimentHashAnnotation             = "fleet.datadoghq.com/experiment-config-hash"
	fleetPendingTargetUIDAnnotation           = "fleet.datadoghq.com/pending-target-uid"
	fleetCreateTaskIDAnnotation               = "fleet.datadoghq.com/create-task-id"
	fleetManagedByValue                       = "fleet-automation"
	fleetManagedAgentInstallationStatePartial = "partial"
	fleetManagedAgentInstallationStateReady   = "ready"
	fleetPartialConfigVersionPrefix           = "partial:"

	fleetDatadogAgentNamespace = "datadog"
	fleetDatadogAgentName      = "datadog-agent"
	fleetCredentialSecretName  = "datadog-secret"
	fleetCredentialAPIKey      = "api-key"

	datadogAgentReconcileErrorCondition = "DatadogAgentReconcileError"
	operatorStoreLabelKey               = "operator.datadoghq.com/managed-by-store"
	appPartOfLabelKey                   = "app.kubernetes.io/part-of"
	appManagedByLabelKey                = "app.kubernetes.io/managed-by"
	datadogAgentNameLabelKey            = "agent.datadoghq.com/datadogagent"
)

var managedAgentInstallationDeletePollInterval = time.Second
var managedAgentInstallationReadinessPollInterval = time.Second
var managedAgentInstallationOperationTimeout = 3 * time.Minute

var forbiddenManagedAgentInstallationConfigFields = map[string]struct{}{
	"apikey":                     {},
	"apisecret":                  {},
	"additionalconfigs":          {},
	"annotations":                {},
	"appkey":                     {},
	"appsecret":                  {},
	"clusteragenttoken":          {},
	"clusteragenttokensecret":    {},
	"command":                    {},
	"controllerexpandsecretref":  {},
	"controllerpublishsecretref": {},
	"credentials":                {},
	"createrbac":                 {},
	"configdata":                 {},
	"configdatamap":              {},
	"configmap":                  {},
	"configmapkeyref":            {},
	"customconfigurations":       {},
	"ddtraceconfigs":             {},
	"dnsconfig":                  {},
	"dnspolicy":                  {},
	"env":                        {},
	"envfrom":                    {},
	"extrachecksd":               {},
	"extraconfd":                 {},
	"extensionurl":               {},
	"hostnetwork":                {},
	"hostpid":                    {},
	"image":                      {},
	"imagetag":                   {},
	"labels":                     {},
	"nodeexpandsecretref":        {},
	"nodepublishsecretref":       {},
	"nodestagesecretref":         {},
	"secret":                     {},
	"secretbackend":              {},
	"secretkeyref":               {},
	"secretname":                 {},
	"secretref":                  {},
	"pullsecrets":                {},
	"registry":                   {},
	"runtimeclassname":           {},
	"securitycontext":            {},
	"secretproviderclass":        {},
	"serviceaccountannotations":  {},
	"serviceaccountname":         {},
	"serviceaccounttoken":        {},
	"volumes":                    {},
	"volumemounts":               {},
	"args":                       {},
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

func (d *Daemon) resolveManagedAgentInstallationOperation(req remoteAPIRequest, expected Operation) (resolvedOperation, error) {
	operationName := fmt.Sprintf("%s DatadogAgent", expected)
	if req.Package != packageDatadogOperator {
		return resolvedOperation{}, fmt.Errorf("%s: package must be %q, got %q", operationName, packageDatadogOperator, req.Package)
	}
	if err := validateManagedAgentInstallationParams(req.Params); err != nil {
		return resolvedOperation{}, fmt.Errorf("%s: invalid params: %w", operationName, err)
	}
	if req.Params.NamespacedName.Namespace != fleetDatadogAgentNamespace {
		return resolvedOperation{}, &stateDoesntMatchError{msg: fmt.Sprintf("%s: namespace must be %q, got %q", operationName, fleetDatadogAgentNamespace, req.Params.NamespacedName.Namespace)}
	}
	if req.Params.NamespacedName.Name != fleetDatadogAgentName {
		return resolvedOperation{}, &stateDoesntMatchError{msg: fmt.Sprintf("%s: name must be %q, got %q", operationName, fleetDatadogAgentName, req.Params.NamespacedName.Name)}
	}
	if req.Params.Version == "" {
		return resolvedOperation{}, fmt.Errorf("%s: version is required", operationName)
	}

	cfg, err := d.getConfig(req.Params.Version)
	if err != nil {
		return resolvedOperation{}, fmt.Errorf("%s: %w", operationName, err)
	}
	if len(cfg.Operations) != 1 {
		return resolvedOperation{}, fmt.Errorf("%s: config %s must have exactly 1 operation, got %d", operationName, cfg.ID, len(cfg.Operations))
	}
	if cfg.Operations[0].Operation != expected {
		return resolvedOperation{}, fmt.Errorf("%s: invalid operation: %s", operationName, cfg.Operations[0].Operation)
	}

	return resolvedOperation{
		NamespacedName: req.Params.NamespacedName,
		Config:         cfg.Operations[0].Config,
	}, nil
}

func validateManagedAgentInstallationParams(params operatorTaskParams) error {
	if err := validateParams(params); err != nil {
		return err
	}
	wantGVK := v2alpha1.GroupVersion.WithKind("DatadogAgent")
	if params.GroupVersionKind != wantGVK {
		return fmt.Errorf("params group_version_kind must be %s, got %s", wantGVK, params.GroupVersionKind)
	}
	return nil
}

func (d *Daemon) installDatadogAgent(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveManagedAgentInstallationOperation(req, OperationCreate)
	if err != nil {
		return nil, err
	}
	spec, err := buildFleetDatadogAgentSpec(op.Config)
	if err != nil {
		return nil, fmt.Errorf("create DatadogAgent: %w", err)
	}

	if err := d.validateFleetCredentialSecret(ctx); err != nil {
		return nil, fmt.Errorf("create DatadogAgent: %w", err)
	}
	if _, err := d.validateDatadogAgentInstallTarget(ctx, op.NamespacedName); err != nil {
		return nil, err
	}
	configHash, err := fleetDatadogAgentSpecHash(spec)
	if err != nil {
		return nil, fmt.Errorf("create DatadogAgent: hash config: %w", err)
	}

	existing := &v2alpha1.DatadogAgent{}
	err = d.managedAgentInstallationReader().Get(ctx, op.NamespacedName, existing)
	if err == nil {
		if err := d.validateFleetDatadogAgentInstallation(existing); err != nil {
			return nil, err
		}
		if err := validateFleetDatadogAgentInstallReplay(existing, req.Params.Version, spec); err != nil {
			return nil, err
		}
		d.setPackageConfigVersions(req.Package, fleetPartialConfigVersionPrefix+req.Params.Version, "")
		if _, err := d.markFleetDatadogAgentPartial(ctx, op.NamespacedName, existing.UID); err != nil {
			return nil, fmt.Errorf("create DatadogAgent: mark managed Agent installation partial before readiness revalidation: %w", err)
		}
		if err := d.ensureManagedAgentInstallationResources(ctx, req, existing); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, req, existing.UID, err)
		}
		if err := d.waitForManagedAgentInstallationResourcesReady(ctx, req, op.NamespacedName, existing.UID); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, req, existing.UID, fmt.Errorf("create DatadogAgent: %w", err))
		}
		if err := d.markFleetDatadogAgentReady(ctx, op.NamespacedName, existing.UID, req.Params.Version); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, req, existing.UID, fmt.Errorf("create DatadogAgent: mark managed Agent installation ready: %w", err))
		}
		observed, conflictErr := d.validateDatadogAgentInstallTarget(ctx, op.NamespacedName)
		if conflictErr != nil {
			return nil, d.handleDatadogAgentInstallCoexistenceConflict(ctx, req, existing, observed, conflictErr, false)
		}
		if err := validateFleetDatadogAgentInstallCompletion(observed, existing.UID, req.Params.Version); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, req, existing.UID, err)
		}
		if err := d.validateFleetDatadogAgentInstallation(observed); err != nil {
			return nil, err
		}
		if req.Addon != nil {
			if err := d.validateManagedAgentInstallationResourcesReady(ctx, observed); err != nil {
				return nil, d.retainFleetDatadogAgentPartial(ctx, req, existing.UID, err)
			}
		}
		if req.Addon == nil {
			d.setPackageConfigVersions(req.Package, req.Params.Version, "")
		}
		return nil, nil
	}
	if !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("create DatadogAgent: failed to check existing resource: %w", err)
	}

	dda := &v2alpha1.DatadogAgent{
		TypeMeta: metav1.TypeMeta{
			APIVersion: v2alpha1.GroupVersion.String(),
			Kind:       "DatadogAgent",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      op.NamespacedName.Name,
			Namespace: op.NamespacedName.Namespace,
			Labels: map[string]string{
				fleetManagedByLabel:                     fleetManagedByValue,
				fleetConfigIDLabel:                      req.Params.Version,
				fleetManagedAgentInstallationStateLabel: fleetManagedAgentInstallationStatePartial,
				fleetInstallationIDLabel:                req.Params.InstallationID,
				fleetTargetIDLabel:                      d.managedAgentInstallationIdentity.TargetID(),
			},
			Annotations: map[string]string{
				fleetConfigHashAnnotation:   configHash,
				fleetCreateTaskIDAnnotation: req.ID,
			},
		},
		Spec: *spec,
	}
	if err := d.client.Create(ctx, dda, client.FieldOwner("fleet-daemon")); err != nil {
		if !apierrors.IsAlreadyExists(err) && !isRetryable(err) {
			return nil, fmt.Errorf("create DatadogAgent: %w", err)
		}
		current := &v2alpha1.DatadogAgent{}
		if getErr := k8sretry.OnError(k8sretry.DefaultBackoff, func(getErr error) bool {
			return apierrors.IsNotFound(getErr) || isRetryable(getErr)
		}, func() error {
			return d.managedAgentInstallationReader().Get(ctx, op.NamespacedName, current)
		}); getErr != nil {
			return nil, fmt.Errorf("create DatadogAgent: create returned %v and the resource could not be recovered: %w", err, getErr)
		}
		if ownershipErr := validateFleetOwnedDatadogAgent(current, req.Params.Version); ownershipErr != nil {
			return nil, ownershipErr
		}
		if ownershipErr := d.validateFleetDatadogAgentInstallation(current); ownershipErr != nil {
			return nil, ownershipErr
		}
		d.setPackageConfigVersions(req.Package, fleetPartialConfigVersionPrefix+req.Params.Version, "")
		if validateErr := validateFleetDatadogAgentInstallReplay(current, req.Params.Version, spec); validateErr != nil {
			return nil, validateErr
		}
		observed, conflictErr := d.validateDatadogAgentInstallTarget(ctx, op.NamespacedName)
		if conflictErr != nil {
			return nil, d.handleDatadogAgentInstallCoexistenceConflict(ctx, req, current, observed, conflictErr, false)
		}
		return nil, fmt.Errorf("create DatadogAgent: create returned %v; recovered Fleet-owned resource remains partial for retry or explicit uninstall", err)
	}
	d.publishFleetDatadogAgentManagedAgentInstallationState(req.Package, dda, req.Params.Version)
	observed, conflictErr := d.validateDatadogAgentInstallTarget(ctx, op.NamespacedName)
	if conflictErr != nil {
		return nil, d.handleDatadogAgentInstallCoexistenceConflict(ctx, req, dda, observed, conflictErr, true)
	}
	acceptedHash, err := fleetDatadogAgentSpecHash(&dda.Spec)
	if err != nil {
		return nil, fmt.Errorf("create DatadogAgent: hash accepted spec: %w", err)
	}
	if err := d.recordFleetDatadogAgentSpecHash(ctx, op.NamespacedName, dda.UID, req.Params.Version, acceptedHash); err != nil {
		return nil, fmt.Errorf("create DatadogAgent: record accepted spec: %w", err)
	}
	if err := d.ensureManagedAgentInstallationResources(ctx, req, dda); err != nil {
		return nil, d.retainFleetDatadogAgentPartial(ctx, req, dda.UID, err)
	}
	if err := d.waitForManagedAgentInstallationResourcesReady(ctx, req, op.NamespacedName, dda.UID); err != nil {
		return nil, fmt.Errorf("create DatadogAgent: %w", err)
	}
	if err := d.markFleetDatadogAgentReady(ctx, op.NamespacedName, dda.UID, req.Params.Version); err != nil {
		return nil, fmt.Errorf("create DatadogAgent: mark managed Agent installation ready: %w", err)
	}
	observed, conflictErr = d.validateDatadogAgentInstallTarget(ctx, op.NamespacedName)
	if conflictErr != nil {
		return nil, d.handleDatadogAgentInstallCoexistenceConflict(ctx, req, dda, observed, conflictErr, true)
	}
	if err := validateFleetDatadogAgentInstallCompletion(observed, dda.UID, req.Params.Version); err != nil {
		return nil, d.retainFleetDatadogAgentPartial(ctx, req, dda.UID, err)
	}
	if err := d.validateFleetDatadogAgentInstallation(observed); err != nil {
		return nil, err
	}
	if req.Addon != nil {
		if err := d.validateManagedAgentInstallationResourcesReady(ctx, observed); err != nil {
			return nil, d.retainFleetDatadogAgentPartial(ctx, req, dda.UID, err)
		}
	}
	if req.Addon == nil {
		d.setPackageConfigVersions(req.Package, req.Params.Version, "")
	}

	ctrl.LoggerFrom(ctx).Info("Created Fleet-managed DatadogAgent", "namespace", dda.Namespace, "name", dda.Name, "config", req.Params.Version)
	return nil, nil
}

func (d *Daemon) handleDatadogAgentInstallCoexistenceConflict(ctx context.Context, req remoteAPIRequest, created, observed *v2alpha1.DatadogAgent, conflictErr error, createdByThisInvocation bool) error {
	var coexistenceErr *datadogAgentCoexistenceError
	canRollback := errors.As(conflictErr, &coexistenceErr) &&
		createdByThisInvocation &&
		observed != nil &&
		observed.UID == created.UID &&
		observed.Annotations[fleetCreateTaskIDAnnotation] == req.ID
	if !canRollback {
		return d.retainFleetDatadogAgentPartial(ctx, req, created.UID, fmt.Errorf("%w; this invocation cannot safely roll back the DatadogAgent, leaving it for explicit uninstall", conflictErr))
	}
	if rollbackErr := d.rollbackFleetDatadogAgentCreation(ctx, client.ObjectKeyFromObject(observed), observed.UID, observed.ResourceVersion, req.Params.Version, req.ID); rollbackErr != nil {
		return d.retainFleetDatadogAgentPartial(ctx, req, created.UID, fmt.Errorf("%w; failed to roll back concurrently conflicting Fleet DatadogAgent: %v", conflictErr, rollbackErr))
	}
	d.setPackageConfigVersions(req.Package, "", "")
	return conflictErr
}

func (d *Daemon) retainFleetDatadogAgentPartial(ctx context.Context, req remoteAPIRequest, uid types.UID, cause error) error {
	d.setPackageConfigVersions(req.Package, fleetPartialConfigVersionPrefix+req.Params.Version, "")
	configID, err := d.markFleetDatadogAgentPartial(ctx, req.Params.NamespacedName, uid)
	if err != nil {
		return fmt.Errorf("%w; failed to retain the Fleet-managed DatadogAgent as partial: %v", cause, err)
	}
	d.setPackageConfigVersions(req.Package, fleetPartialConfigVersionPrefix+configID, "")
	return cause

}

func (d *Daemon) rollbackFleetDatadogAgentCreation(ctx context.Context, nsn types.NamespacedName, uid types.UID, resourceVersion, configID, createTaskID string) error {
	current := &v2alpha1.DatadogAgent{}
	if err := d.managedAgentInstallationReader().Get(ctx, nsn, current); err != nil {
		if apierrors.IsNotFound(err) {
			cleanupTargets, listErr := d.fleetDatadogAgentInternalCleanupTargets(ctx, nsn, uid)
			if listErr != nil {
				return listErr
			}
			return d.waitForFleetDatadogAgentCleanup(ctx, nsn, uid, cleanupTargets)
		}
		return err
	}
	if current.UID != uid {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before concurrent-install rollback", current.Namespace, current.Name)}
	}
	if current.ResourceVersion != resourceVersion {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s changed after the concurrent-install conflict was observed", current.Namespace, current.Name)}
	}
	owned, err := classifyFleetDatadogAgentOwnership(current)
	if err != nil {
		return err
	}
	if !owned || current.Labels[fleetConfigIDLabel] != configID {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s no longer has the Fleet ownership created by config %q", current.Namespace, current.Name, configID)}
	}
	if err := d.validateFleetDatadogAgentInstallation(current); err != nil {
		return err
	}
	if current.Annotations[fleetCreateTaskIDAnnotation] != createTaskID {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s no longer has create provenance for task %q", current.Namespace, current.Name, createTaskID)}
	}
	cleanupTargets, err := d.fleetDatadogAgentInternalCleanupTargets(ctx, nsn, uid)
	if err != nil {
		return err
	}
	preconditions := managedAgentInstallationDeletePreconditions(current.UID, current.ResourceVersion)
	if err := d.client.Delete(ctx, current, client.Preconditions(preconditions), client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !apierrors.IsNotFound(err) {
		return err
	}
	return d.waitForFleetDatadogAgentCleanup(ctx, nsn, uid, cleanupTargets)
}

func (d *Daemon) uninstallDatadogAgent(ctx context.Context, req remoteAPIRequest) (*pendingOperation, error) {
	op, err := d.resolveManagedAgentInstallationOperation(req, OperationDelete)
	if err != nil {
		return nil, err
	}
	if err := d.validateFleetDatadogAgentTarget(ctx, op.NamespacedName); err != nil {
		return nil, err
	}

	dda := &v2alpha1.DatadogAgent{}
	err = d.managedAgentInstallationReader().Get(ctx, op.NamespacedName, dda)
	if apierrors.IsNotFound(err) {
		activatedFence, fenceErr := d.activateUninstallFence(ctx, req)
		if fenceErr != nil {
			return nil, fmt.Errorf("delete DatadogAgent: activate uninstall fence: %w", fenceErr)
		}
		if req.Addon != nil {
			absentDDA := &v2alpha1.DatadogAgent{ObjectMeta: metav1.ObjectMeta{Namespace: op.NamespacedName.Namespace, Name: op.NamespacedName.Name}}
			if err := d.deleteManagedAgentInstallationWindowsProfile(ctx, absentDDA); err != nil {
				return nil, err
			}
		}
		cleanupTargets, listErr := d.fleetDatadogAgentInternalCleanupTargets(ctx, op.NamespacedName, "")
		if listErr != nil {
			return nil, fmt.Errorf("delete DatadogAgent: find remaining internal resources: %w", listErr)
		}
		for i, target := range cleanupTargets {
			updated, deleteErr := d.deleteFleetDatadogAgentInternal(ctx, target)
			if deleteErr != nil {
				return nil, fmt.Errorf("delete DatadogAgent: delete remaining DatadogAgentInternal %s/%s: %w", target.Namespace, target.Name, deleteErr)
			}
			cleanupTargets[i] = updated
		}
		if waitErr := d.waitForFleetDatadogAgentCleanup(ctx, op.NamespacedName, "", cleanupTargets); waitErr != nil {
			return nil, fmt.Errorf("delete DatadogAgent: waiting for remaining resource removal: %w", waitErr)
		}
		if req.Addon != nil {
			if err := d.waitForManagedAgentInstallationWindowsProfileAbsent(ctx); err != nil {
				return nil, fmt.Errorf("delete DatadogAgent: waiting for Windows profile removal: %w", err)
			}
		}
		if err := d.validateFleetDatadogAgentTargetAbsent(ctx, op.NamespacedName); err != nil {
			return nil, err
		}
		if err := d.verifyDatadogAgentManagedAgentInstallationResourcesAbsent(ctx, op.NamespacedName); err != nil {
			return nil, err
		}
		if err := d.verifyUnchangedActiveUninstallFence(ctx, req, activatedFence); err != nil {
			return nil, fmt.Errorf("delete DatadogAgent: revalidate uninstall fence: %w", err)
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("delete DatadogAgent: failed to get resource: %w", err)
	}
	owned, err := classifyFleetDatadogAgentOwnership(dda)
	if err != nil {
		return nil, err
	}
	if !owned {
		return nil, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not owned by Fleet Automation", dda.Namespace, dda.Name)}
	}
	if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
		return nil, err
	}
	if err := validateNoActiveFleetExperiment(dda); err != nil {
		return nil, err
	}
	activatedFence, err := d.activateUninstallFence(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("delete DatadogAgent: activate uninstall fence: %w", err)
	}
	if req.Addon != nil {
		if err := d.deleteManagedAgentInstallationWindowsProfile(ctx, dda); err != nil {
			return nil, err
		}
	}
	cleanupTargets, err := d.fleetDatadogAgentInternalCleanupTargets(ctx, op.NamespacedName, dda.UID)
	if err != nil {
		return nil, fmt.Errorf("delete DatadogAgent: find internal resources: %w", err)
	}
	if err := d.validateFleetDatadogAgentTarget(ctx, op.NamespacedName); err != nil {
		return nil, err
	}

	preconditions := managedAgentInstallationDeletePreconditions(dda.UID, dda.ResourceVersion)
	if err := d.client.Delete(ctx, dda, client.Preconditions(preconditions), client.PropagationPolicy(metav1.DeletePropagationForeground)); err != nil && !apierrors.IsNotFound(err) {
		return nil, fmt.Errorf("delete DatadogAgent: %w", err)
	}
	if err := d.waitForFleetDatadogAgentCleanup(ctx, op.NamespacedName, dda.UID, cleanupTargets); err != nil {
		return nil, fmt.Errorf("delete DatadogAgent: waiting for resource removal: %w", err)
	}
	if req.Addon != nil {
		if err := d.waitForManagedAgentInstallationWindowsProfileAbsent(ctx); err != nil {
			return nil, fmt.Errorf("delete DatadogAgent: waiting for Windows profile removal: %w", err)
		}
	}
	if err := d.validateFleetDatadogAgentTargetAbsent(ctx, op.NamespacedName); err != nil {
		return nil, err
	}
	if err := d.verifyDatadogAgentManagedAgentInstallationResourcesAbsent(ctx, op.NamespacedName); err != nil {
		return nil, err
	}
	if err := d.verifyUnchangedActiveUninstallFence(ctx, req, activatedFence); err != nil {
		return nil, fmt.Errorf("delete DatadogAgent: revalidate uninstall fence: %w", err)
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
	if err := rejectRemoteCredentialSelectors(raw); err != nil {
		return nil, err
	}
	if err := rejectDestructiveManagedAgentInstallationNulls(raw); err != nil {
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
	if err := validateRemoteManagedAgentInstallationSpec(config.Spec); err != nil {
		return nil, err
	}
	return &config, nil
}

func rejectDestructiveManagedAgentInstallationNulls(raw json.RawMessage) error {
	var config map[string]json.RawMessage
	if err := json.Unmarshal(raw, &config); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	rawSpec, ok := config["spec"]
	if !ok {
		return nil
	}
	if bytes.Equal(bytes.TrimSpace(rawSpec), []byte("null")) {
		return fmt.Errorf("config.spec must not be null")
	}
	var spec map[string]json.RawMessage
	if err := json.Unmarshal(rawSpec, &spec); err != nil {
		return fmt.Errorf("invalid config.spec: %w", err)
	}
	if rawGlobal, ok := spec["global"]; ok && bytes.Equal(bytes.TrimSpace(rawGlobal), []byte("null")) {
		return fmt.Errorf("config.spec.global must not be null")
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

func validateFleetDatadogAgentAcceptedExperimentSpec(dda *v2alpha1.DatadogAgent) error {
	return validateFleetDatadogAgentSpecHash(dda, fleetExperimentHashAnnotation, "Fleet experiment")
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

func (d *Daemon) publishFleetDatadogAgentManagedAgentInstallationState(packageName string, dda *v2alpha1.DatadogAgent, configID string) {
	reportedConfigID := configID
	if dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		reportedConfigID = fleetPartialConfigVersionPrefix + configID
	}
	d.setPackageConfigVersions(packageName, reportedConfigID, "")
}

func (d *Daemon) markFleetDatadogAgentReady(ctx context.Context, nsn types.NamespacedName, uid types.UID, configID string) error {
	return d.markFleetDatadogAgentReadyWithValidation(ctx, nsn, uid, configID, nil)
}

func (d *Daemon) markFleetDatadogAgentReadyAfterExperiment(ctx context.Context, nsn types.NamespacedName, uid types.UID, configID string, task pendingOperation) error {
	return d.markFleetDatadogAgentReadyWithValidation(ctx, nsn, uid, configID, func(dda *v2alpha1.DatadogAgent) error {
		done, err := evaluatePendingTask(newDDAStatusSnapshot(dda), task)
		if err != nil {
			return err
		}
		if !done {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has not completed the pending Fleet experiment", dda.Namespace, dda.Name)}
		}
		return nil
	})
}

func (d *Daemon) markFleetDatadogAgentReadyWithValidation(ctx context.Context, nsn types.NamespacedName, uid types.UID, configID string, validate func(*v2alpha1.DatadogAgent) error) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if uid == "" || dda.UID != uid {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before its managed Agent installation state could be marked ready", dda.Namespace, dda.Name)}
		}
		if validate != nil {
			if err := validate(dda); err != nil {
				return err
			}
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
		ready, observation, err := fleetDatadogAgentReadiness(dda, uid)
		if err != nil {
			return err
		}
		if !ready {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is no longer ready: %s", dda.Namespace, dda.Name, observation)}
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

func (d *Daemon) markFleetDatadogAgentExperimentReady(ctx context.Context, nsn types.NamespacedName, uid types.UID, experimentID string) (string, error) {
	var configID string
	err := k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if uid == "" || dda.UID != uid {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced while finishing the Fleet experiment", dda.Namespace, dda.Name)}
		}
		owned, err := classifyFleetDatadogAgentOwnership(dda)
		if err != nil {
			return err
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		if !owned {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is no longer managed by Fleet Automation", dda.Namespace, dda.Name)}
		}
		if !experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhaseRunning) {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s experiment %q is no longer running", dda.Namespace, dda.Name, experimentID)}
		}
		if err := validateFleetDatadogAgentAcceptedExperimentSpec(dda); err != nil {
			return err
		}
		configID = dda.Labels[fleetConfigIDLabel]
		if dda.Labels[fleetManagedAgentInstallationStateLabel] == fleetManagedAgentInstallationStateReady {
			return nil
		}
		base := dda.DeepCopy()
		dda.Labels[fleetManagedAgentInstallationStateLabel] = fleetManagedAgentInstallationStateReady
		return d.client.Patch(
			ctx,
			dda,
			client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
			client.FieldOwner("fleet-daemon"),
		)
	})
	return configID, err
}

func (d *Daemon) recordFleetExperimentSpecHash(ctx context.Context, nsn types.NamespacedName, uid types.UID, acceptedHash string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if uid == "" || dda.UID != uid {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before the accepted Fleet experiment hash could be recorded", dda.Namespace, dda.Name)}
		}
		owned, err := classifyFleetDatadogAgentOwnership(dda)
		if err != nil {
			return err
		}
		if !owned {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is no longer managed by Fleet Automation", dda.Namespace, dda.Name)}
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		if err := validateFleetDatadogAgentCredentials(dda); err != nil {
			return err
		}
		liveHash, err := fleetDatadogAgentSpecHash(&dda.Spec)
		if err != nil {
			return err
		}
		if liveHash != acceptedHash {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s spec changed before the accepted Fleet experiment hash could be recorded", dda.Namespace, dda.Name)}
		}
		if dda.Annotations[fleetExperimentHashAnnotation] == acceptedHash {
			return nil
		}
		base := dda.DeepCopy()
		if dda.Annotations == nil {
			dda.Annotations = make(map[string]string)
		}
		dda.Annotations[fleetExperimentHashAnnotation] = acceptedHash
		return d.client.Patch(
			ctx,
			dda,
			client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
			client.FieldOwner("fleet-daemon"),
		)
	})
}

func (d *Daemon) waitForFleetDatadogAgentReady(ctx context.Context, nsn types.NamespacedName, uid types.UID) error {
	lastObservation := "waiting for the DatadogAgent reconciler"
	err := wait.PollUntilContextTimeout(ctx, managedAgentInstallationReadinessPollInterval, managedAgentInstallationOperationTimeout, true, func(ctx context.Context) (bool, error) {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return false, err
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return false, err
		}
		ready, observation, err := fleetDatadogAgentReadiness(dda, uid)
		if observation != "" {
			lastObservation = observation
		}
		return ready, err
	})
	if err != nil {
		return fmt.Errorf("waiting for DatadogAgent %s/%s readiness (%s): %w", nsn.Namespace, nsn.Name, lastObservation, err)
	}
	return nil
}

func fleetDatadogAgentReadiness(dda *v2alpha1.DatadogAgent, uid types.UID) (bool, string, error) {
	if dda.UID != uid {
		return false, "", &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced while waiting for install readiness", dda.Namespace, dda.Name)}
	}
	if !dda.DeletionTimestamp.IsZero() {
		return false, "", &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s began terminating while waiting for install readiness", dda.Namespace, dda.Name)}
	}
	reconcileCondition := meta.FindStatusCondition(dda.Status.Conditions, datadogAgentReconcileErrorCondition)
	if reconcileCondition == nil || reconcileCondition.ObservedGeneration != dda.Generation {
		return false, fmt.Sprintf("generation %d has not been reconciled", dda.Generation), nil
	}
	if reconcileCondition.Status != metav1.ConditionFalse {
		return false, fmt.Sprintf("reconcile condition is %s: %s", reconcileCondition.Status, reconcileCondition.Message), nil
	}
	if dda.Status.Agent == nil && len(dda.Status.AgentList) == 0 {
		return false, "the reconciler has not observed the required Agent workload", nil
	}
	agentStatuses := dda.Status.AgentList
	if len(agentStatuses) == 0 {
		agentStatuses = []*v2alpha1.DaemonSetStatus{dda.Status.Agent}
	}
	hasScheduledAgent := false
	for _, status := range agentStatuses {
		if status == nil {
			return false, "an Agent DaemonSet status is missing", nil
		}
		if status.Desired > 0 {
			hasScheduledAgent = true
		}
		if !daemonSetStatusReady(status) {
			return false, fmt.Sprintf("Agent DaemonSet %q is not ready", status.DaemonsetName), nil
		}
	}
	if !hasScheduledAgent {
		return false, "no Agent DaemonSet has scheduled pods", nil
	}
	if dda.Status.ClusterAgent == nil {
		return false, "the reconciler has not observed the Cluster Agent workload", nil
	}
	clusterAgent := dda.Status.ClusterAgent
	if clusterAgent.Replicas == 0 || clusterAgent.UpdatedReplicas != clusterAgent.Replicas ||
		clusterAgent.ReadyReplicas != clusterAgent.Replicas || clusterAgent.AvailableReplicas != clusterAgent.Replicas ||
		clusterAgent.UnavailableReplicas != 0 {
		return false, "the Cluster Agent workload is not ready", nil
	}
	return true, "", nil
}

func daemonSetStatusReady(status *v2alpha1.DaemonSetStatus) bool {
	return status != nil &&
		status.Current == status.Desired &&
		status.Ready == status.Desired &&
		status.Available == status.Desired &&
		status.UpToDate == status.Desired
}

type fleetDatadogAgentInternalCleanupTarget struct {
	types.NamespacedName
	UID              types.UID
	ResourceVersion  string
	OwnerKey         types.NamespacedName
	OwnerUID         types.UID
	PartOfLabelValue string
}

func (d *Daemon) fleetDatadogAgentInternalCleanupTargets(ctx context.Context, ddaKey types.NamespacedName, ddaUID types.UID) ([]fleetDatadogAgentInternalCleanupTarget, error) {
	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err := d.managedAgentInstallationReader().List(ctx, ddais, client.MatchingLabels{fleetManagedByLabel: fleetManagedByValue}); err != nil {
		return nil, err
	}
	targets := make([]fleetDatadogAgentInternalCleanupTarget, 0, len(ddais.Items))
	for i := range ddais.Items {
		ddai := &ddais.Items[i]
		for _, owner := range ddai.OwnerReferences {
			if owner.Kind != "DatadogAgent" || owner.Name != ddaKey.Name || ddai.Namespace != ddaKey.Namespace {
				continue
			}
			if ddaUID != "" && owner.UID != ddaUID {
				continue
			}
			if err := d.validateFleetDatadogAgentInternalInstallation(ddai); err != nil {
				return nil, err
			}
			targets = append(targets, fleetDatadogAgentInternalCleanupTarget{
				NamespacedName:   client.ObjectKeyFromObject(ddai),
				UID:              ddai.UID,
				ResourceVersion:  ddai.ResourceVersion,
				OwnerKey:         ddaKey,
				OwnerUID:         owner.UID,
				PartOfLabelValue: managedAgentInstallationDatadogAgentInternalPartOfLabelValue(ddai),
			})
			break
		}
	}
	return targets, nil
}

func (d *Daemon) deleteFleetDatadogAgentInternal(ctx context.Context, target fleetDatadogAgentInternalCleanupTarget) (fleetDatadogAgentInternalCleanupTarget, error) {
	err := k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		current := &v1alpha1.DatadogAgentInternal{}
		if err := d.managedAgentInstallationReader().Get(ctx, target.NamespacedName, current); err != nil {
			if apierrors.IsNotFound(err) {
				return nil
			}
			return err
		}
		if current.UID != target.UID {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentInternal %s/%s was replaced while uninstalling", current.Namespace, current.Name)}
		}
		if current.Labels[fleetManagedByLabel] != fleetManagedByValue {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentInternal %s/%s ownership changed while uninstalling", current.Namespace, current.Name)}
		}
		if err := d.validateFleetDatadogAgentInternalInstallation(current); err != nil {
			return err
		}
		owned := false
		for _, owner := range current.OwnerReferences {
			if owner.Kind == "DatadogAgent" && owner.Name == target.OwnerKey.Name && current.Namespace == target.OwnerKey.Namespace && owner.UID == target.OwnerUID {
				owned = true
				break
			}
		}
		if !owned {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentInternal %s/%s owner changed while uninstalling", current.Namespace, current.Name)}
		}
		target.ResourceVersion = current.ResourceVersion
		target.PartOfLabelValue = managedAgentInstallationDatadogAgentInternalPartOfLabelValue(current)
		preconditions := managedAgentInstallationDeletePreconditions(current.UID, current.ResourceVersion)
		return d.client.Delete(ctx, current, client.Preconditions(preconditions), client.PropagationPolicy(metav1.DeletePropagationForeground))
	})
	if apierrors.IsNotFound(err) {
		err = nil
	}
	return target, err
}

func (d *Daemon) cleanupOrphanedFleetDatadogAgentInternals(ctx context.Context, ddas *v2alpha1.DatadogAgentList) error {
	liveOwners := make(map[types.UID]types.NamespacedName, len(ddas.Items))
	liveKeys := make(map[types.NamespacedName]types.UID, len(ddas.Items))
	for i := range ddas.Items {
		dda := &ddas.Items[i]
		key := client.ObjectKeyFromObject(dda)
		liveOwners[dda.UID] = key
		liveKeys[key] = dda.UID
	}
	ddais := &v1alpha1.DatadogAgentInternalList{}
	if err := d.managedAgentInstallationReader().List(ctx, ddais, client.MatchingLabels{fleetManagedByLabel: fleetManagedByValue}); err != nil {
		return err
	}
	for i := range ddais.Items {
		ddai := &ddais.Items[i]
		if err := d.validateFleetDatadogAgentInternalInstallation(ddai); err != nil {
			return err
		}
		var owner *metav1.OwnerReference
		for j := range ddai.OwnerReferences {
			if ddai.OwnerReferences[j].Kind == "DatadogAgent" {
				owner = &ddai.OwnerReferences[j]
				break
			}
		}
		if owner == nil {
			return fmt.Errorf("Fleet-managed DatadogAgentInternal %s/%s has no DatadogAgent owner", ddai.Namespace, ddai.Name)
		}
		if _, ok := liveOwners[owner.UID]; ok {
			continue
		}
		ownerKey := types.NamespacedName{Namespace: ddai.Namespace, Name: owner.Name}
		if replacementUID, ok := liveKeys[ownerKey]; ok && replacementUID != owner.UID {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced while Fleet cleanup was incomplete", ownerKey.Namespace, ownerKey.Name)}
		}
		target := fleetDatadogAgentInternalCleanupTarget{
			NamespacedName:   client.ObjectKeyFromObject(ddai),
			UID:              ddai.UID,
			ResourceVersion:  ddai.ResourceVersion,
			OwnerKey:         ownerKey,
			OwnerUID:         owner.UID,
			PartOfLabelValue: managedAgentInstallationDatadogAgentInternalPartOfLabelValue(ddai),
		}
		target, err := d.deleteFleetDatadogAgentInternal(ctx, target)
		if err != nil {
			return err
		}
		if err := d.waitForFleetDatadogAgentCleanup(ctx, ownerKey, "", []fleetDatadogAgentInternalCleanupTarget{target}); err != nil {
			return err
		}
	}
	return nil
}

func (d *Daemon) waitForFleetDatadogAgentCleanup(ctx context.Context, ddaKey types.NamespacedName, ddaUID types.UID, ddais []fleetDatadogAgentInternalCleanupTarget) error {
	tracked := make(map[string]fleetDatadogAgentInternalCleanupTarget, len(ddais))
	for _, target := range ddais {
		tracked[fleetDatadogAgentInternalCleanupTargetKey(target)] = target
	}
	emptyObservations := 0
	return wait.PollUntilContextTimeout(ctx, managedAgentInstallationDeletePollInterval, managedAgentInstallationOperationTimeout, true, func(ctx context.Context) (bool, error) {
		dda := &v2alpha1.DatadogAgent{}
		err := d.managedAgentInstallationReader().Get(ctx, ddaKey, dda)
		if err == nil {
			if ddaUID == "" || dda.UID != ddaUID {
				return false, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced while uninstalling", dda.Namespace, dda.Name)}
			}
			return false, nil
		}
		if !apierrors.IsNotFound(err) {
			return false, err
		}

		currentTargets, err := d.fleetDatadogAgentInternalCleanupTargets(ctx, ddaKey, ddaUID)
		if err != nil {
			return false, err
		}
		if len(currentTargets) != 0 {
			emptyObservations = 0
			for _, target := range currentTargets {
				updated, err := d.deleteFleetDatadogAgentInternal(ctx, target)
				if err != nil {
					return false, err
				}
				tracked[fleetDatadogAgentInternalCleanupTargetKey(updated)] = updated
			}
			return false, nil
		}

		for _, target := range tracked {
			ddai := &v1alpha1.DatadogAgentInternal{}
			err := d.managedAgentInstallationReader().Get(ctx, target.NamespacedName, ddai)
			if err == nil {
				if ddai.UID != target.UID {
					return false, &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentInternal %s/%s was replaced while uninstalling", ddai.Namespace, ddai.Name)}
				}
				return false, nil
			}
			if !apierrors.IsNotFound(err) {
				return false, err
			}
			removed, err := d.datadogAgentInternalClusterResourcesRemoved(ctx, target.PartOfLabelValue)
			if err != nil || !removed {
				return false, err
			}
		}
		removed, err := d.datadogAgentInternalClusterResourcesRemoved(ctx, managedAgentInstallationPartOfLabelValue(ddaKey))
		if err != nil || !removed {
			return false, err
		}
		emptyObservations++
		return emptyObservations >= 2, nil
	})
}

func fleetDatadogAgentInternalCleanupTargetKey(target fleetDatadogAgentInternalCleanupTarget) string {
	return target.Namespace + "/" + target.Name + "/" + string(target.UID)
}

func (d *Daemon) datadogAgentInternalClusterResourcesRemoved(ctx context.Context, partOfLabelValue string) (bool, error) {
	matchingLabels := client.MatchingLabels{
		operatorStoreLabelKey: "true",
		appPartOfLabelKey:     partOfLabelValue,
		appManagedByLabelKey:  "datadog-operator",
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
	escape := func(value string) string { return strings.ReplaceAll(value, "-", "--") }
	return escape(key.Namespace) + "-" + escape(key.Name)
}

func managedAgentInstallationDeletePreconditions(uid types.UID, resourceVersion string) metav1.Preconditions {
	preconditions := metav1.Preconditions{}
	if uid != "" {
		preconditions.UID = &uid
	}
	if resourceVersion != "" {
		preconditions.ResourceVersion = &resourceVersion
	}
	return preconditions
}

func managedAgentInstallationDatadogAgentInternalPartOfLabelValue(ddai *v1alpha1.DatadogAgentInternal) string {
	ddaName := ddai.Labels[datadogAgentNameLabelKey]
	if ddaName == "" {
		ddaName = ddai.Name
	}
	return managedAgentInstallationPartOfLabelValue(types.NamespacedName{Namespace: ddai.Namespace, Name: ddaName})
}

func rejectRemoteCredentialSelectors(raw json.RawMessage) error {
	var value any
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("invalid config: %w", err)
	}
	return walkRemoteConfig(value, "config")
}

func walkRemoteConfig(value any, path string) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			lowerKey := strings.ToLower(key)
			if _, forbidden := forbiddenManagedAgentInstallationConfigFields[lowerKey]; forbidden {
				return fmt.Errorf("%s.%s is not allowed in a Fleet managed Agent installation config", path, key)
			}
			if err := walkRemoteConfig(nested, path+"."+key); err != nil {
				return err
			}
		}
	case []any:
		for i, nested := range typed {
			if err := walkRemoteConfig(nested, fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateRemoteManagedAgentInstallationSpec(spec *v2alpha1.DatadogAgentSpec) error {
	for component, override := range spec.Override {
		if override == nil {
			continue
		}
		if component == v2alpha1.NodeAgentComponentName && override.Disabled != nil && *override.Disabled {
			return fmt.Errorf("config must not disable the node Agent")
		}
		for name, container := range override.Containers {
			if container == nil {
				continue
			}
			resourceOnly := &v2alpha1.DatadogAgentGenericContainer{Resources: container.Resources}
			if !apiequality.Semantic.DeepEqual(container, resourceOnly) {
				return fmt.Errorf("config must not override Agent container %q except for resource requirements", name)
			}
		}
	}
	if spec.Features != nil {
		if spec.Features.ExternalMetricsServer != nil && spec.Features.ExternalMetricsServer.Endpoint != nil {
			return fmt.Errorf("config must not override the external metrics endpoint")
		}
		if logCollection := spec.Features.LogCollection; logCollection != nil &&
			(logCollection.ContainerLogsPath != nil || logCollection.PodLogsPath != nil ||
				logCollection.ContainerSymlinksPath != nil || logCollection.TempStoragePath != nil) {
			return fmt.Errorf("config must not override log collection host paths")
		}
		if apm := spec.Features.APM; apm != nil && apm.UnixDomainSocketConfig != nil && apm.UnixDomainSocketConfig.Path != nil {
			return fmt.Errorf("config must not override the APM Unix domain socket host path")
		}
		if dogstatsd := spec.Features.Dogstatsd; dogstatsd != nil && dogstatsd.UnixDomainSocketConfig != nil && dogstatsd.UnixDomainSocketConfig.Path != nil {
			return fmt.Errorf("config must not override the DogStatsD Unix domain socket host path")
		}
	}
	if spec.Global == nil {
		return nil
	}
	if spec.Global.Kubelet != nil {
		return fmt.Errorf("config must not override kubelet connection settings")
	}
	if spec.Global.DockerSocketPath != nil || spec.Global.CriSocketPath != nil {
		return fmt.Errorf("config must not override container runtime socket host paths")
	}
	if spec.Global.Credentials != nil {
		return fmt.Errorf("config must not contain spec.global.credentials")
	}
	if spec.Global.ClusterAgentToken != nil || spec.Global.ClusterAgentTokenSecret != nil {
		return fmt.Errorf("config must not contain cluster Agent credentials")
	}
	if spec.Global.Endpoint != nil || spec.Global.SecretBackend != nil || spec.Global.Env != nil {
		return fmt.Errorf("config must not contain endpoint, secret backend, or environment credential sources")
	}
	if spec.Global.Site != nil {
		if _, ok := allowedManagedAgentInstallationSites[*spec.Global.Site]; !ok {
			return fmt.Errorf("config contains unsupported Datadog site %q", *spec.Global.Site)
		}
	}
	return nil
}

func (d *Daemon) validateFleetCredentialSecret(ctx context.Context) error {
	secret := &corev1.Secret{}
	nsn := types.NamespacedName{Namespace: fleetDatadogAgentNamespace, Name: fleetCredentialSecretName}
	if err := d.managedAgentInstallationReader().Get(ctx, nsn, secret); err != nil {
		return fmt.Errorf("credential Secret %s/%s is not ready: %w", nsn.Namespace, nsn.Name, err)
	}
	if len(secret.Data[fleetCredentialAPIKey]) == 0 {
		return fmt.Errorf("credential Secret %s/%s is missing non-empty key %q", nsn.Namespace, nsn.Name, fleetCredentialAPIKey)
	}
	return nil
}

func (d *Daemon) managedAgentInstallationReader() client.Reader {
	if d.apiReader != nil {
		return d.apiReader
	}
	return d.client
}

type datadogAgentCoexistenceError struct {
	err *stateDoesntMatchError
}

func (e *datadogAgentCoexistenceError) Error() string {
	return e.err.Error()
}

func (e *datadogAgentCoexistenceError) Unwrap() error {
	return e.err
}

func (d *Daemon) validateDatadogAgentInstallTarget(ctx context.Context, target types.NamespacedName) (*v2alpha1.DatadogAgent, error) {
	ddas := &v2alpha1.DatadogAgentList{}
	if err := d.managedAgentInstallationReader().List(ctx, ddas); err != nil {
		return nil, fmt.Errorf("list DatadogAgents before Fleet install: %w", err)
	}
	var observed *v2alpha1.DatadogAgent
	var conflict *stateDoesntMatchError
	for i := range ddas.Items {
		dda := &ddas.Items[i]
		if client.ObjectKeyFromObject(dda) == target {
			observed = dda.DeepCopy()
			continue
		}
		if conflict != nil {
			continue
		}
		if dda.Labels[fleetManagedByLabel] == fleetManagedByValue {
			conflict = &stateDoesntMatchError{msg: fmt.Sprintf(
				"Fleet Automation already manages DatadogAgent %s/%s; refusing to manage a second resource",
				dda.Namespace,
				dda.Name,
			)}
			continue
		}
		conflict = &stateDoesntMatchError{msg: fmt.Sprintf(
			"cluster already contains unmanaged DatadogAgent %s/%s; refusing Fleet install without modifying it",
			dda.Namespace,
			dda.Name,
		)}
	}
	if conflict != nil {
		return observed, &datadogAgentCoexistenceError{err: conflict}
	}
	return observed, nil
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
	ready, observation, err := fleetDatadogAgentReadiness(dda, uid)
	if err != nil {
		return err
	}
	if !ready {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not ready at install completion: %s", dda.Namespace, dda.Name, observation)}
	}
	if dda.Labels[fleetManagedAgentInstallationStateLabel] != fleetManagedAgentInstallationStateReady {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is not ready at install completion", dda.Namespace, dda.Name)}
	}
	return nil
}

func (d *Daemon) validateFleetDatadogAgentTarget(ctx context.Context, target types.NamespacedName) error {
	ddas := &v2alpha1.DatadogAgentList{}
	if err := d.managedAgentInstallationReader().List(ctx, ddas); err != nil {
		return fmt.Errorf("list Fleet-managed DatadogAgents: %w", err)
	}
	for i := range ddas.Items {
		dda := &ddas.Items[i]
		if dda.Labels[fleetManagedByLabel] != fleetManagedByValue {
			return &stateDoesntMatchError{msg: fmt.Sprintf(
				"cluster contains unmanaged DatadogAgent %s/%s; refusing Fleet uninstall without modifying any DatadogAgent",
				dda.Namespace,
				dda.Name,
			)}
		}
		if client.ObjectKeyFromObject(dda) == target {
			continue
		}
		return &stateDoesntMatchError{msg: fmt.Sprintf(
			"Fleet Automation already manages DatadogAgent %s/%s; refusing to manage a second resource",
			dda.Namespace,
			dda.Name,
		)}
	}
	return nil
}

func (d *Daemon) validateFleetDatadogAgentTargetAbsent(ctx context.Context, target types.NamespacedName) error {
	ddas := &v2alpha1.DatadogAgentList{}
	if err := d.managedAgentInstallationReader().List(ctx, ddas); err != nil {
		return fmt.Errorf("list DatadogAgents after Fleet uninstall: %w", err)
	}
	for i := range ddas.Items {
		dda := &ddas.Items[i]
		if client.ObjectKeyFromObject(dda) == target {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was recreated while uninstalling", dda.Namespace, dda.Name)}
		}
		if dda.Labels[fleetManagedByLabel] == fleetManagedByValue {
			return &stateDoesntMatchError{msg: fmt.Sprintf(
				"Fleet Automation manages DatadogAgent %s/%s after uninstalling %s/%s",
				dda.Namespace,
				dda.Name,
				target.Namespace,
				target.Name,
			)}
		}
		return &stateDoesntMatchError{msg: fmt.Sprintf(
			"cluster contains unmanaged DatadogAgent %s/%s after uninstalling %s/%s",
			dda.Namespace,
			dda.Name,
			target.Namespace,
			target.Name,
		)}
	}
	return nil
}

func (d *Daemon) updateFleetConfigIDLabel(ctx context.Context, nsn types.NamespacedName, uid types.UID, configID, experimentID string) error {
	return k8sretry.RetryOnConflict(k8sretry.DefaultBackoff, func() error {
		dda := &v2alpha1.DatadogAgent{}
		if err := d.managedAgentInstallationReader().Get(ctx, nsn, dda); err != nil {
			return err
		}
		if uid == "" || dda.UID != uid {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s was replaced before Fleet config promotion", dda.Namespace, dda.Name)}
		}
		owned, err := classifyFleetDatadogAgentOwnership(dda)
		if err != nil {
			return err
		}
		if !experimentHasPhase(dda, experimentID, v2alpha1.ExperimentPhasePromoted) {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s experiment %q is no longer promoted", dda.Namespace, dda.Name, experimentID)}
		}
		if !owned {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is no longer managed by Fleet Automation", dda.Namespace, dda.Name)}
		}
		if err := d.validateFleetDatadogAgentInstallation(dda); err != nil {
			return err
		}
		currentConfigID := dda.Labels[fleetConfigIDLabel]
		liveHash, err := fleetDatadogAgentSpecHash(&dda.Spec)
		if err != nil {
			return fmt.Errorf("hash DatadogAgent spec: %w", err)
		}
		if currentConfigID == configID && dda.Annotations[fleetConfigHashAnnotation] == liveHash {
			return nil
		}
		if dda.Annotations[fleetExperimentHashAnnotation] == "" {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s is missing the accepted Fleet experiment spec hash", dda.Namespace, dda.Name)}
		}
		if dda.Annotations[fleetExperimentHashAnnotation] != liveHash {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s spec changed after the Fleet experiment was accepted", dda.Namespace, dda.Name)}
		}
		base := dda.DeepCopy()
		if dda.Labels == nil {
			dda.Labels = make(map[string]string)
		}
		dda.Labels[fleetConfigIDLabel] = configID
		if dda.Annotations == nil {
			dda.Annotations = make(map[string]string)
		}
		dda.Annotations[fleetConfigHashAnnotation] = liveHash
		delete(dda.Annotations, fleetExperimentHashAnnotation)
		return d.client.Patch(
			ctx,
			dda,
			client.MergeFromWithOptions(base, client.MergeFromWithOptimisticLock{}),
			client.FieldOwner("fleet-daemon"),
		)
	})
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
	_, hasInstallationID := dda.Labels[fleetInstallationIDLabel]
	_, hasTargetID := dda.Labels[fleetTargetIDLabel]
	_, hasConfigHash := dda.Annotations[fleetConfigHashAnnotation]
	_, hasExperimentHash := dda.Annotations[fleetExperimentHashAnnotation]
	_, hasCreateTaskID := dda.Annotations[fleetCreateTaskIDAnnotation]
	if !hasManagedBy && !hasConfigID && !hasManagedAgentInstallationState && !hasInstallationID && !hasTargetID && !hasConfigHash && !hasExperimentHash && !hasCreateTaskID {
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
	_, hasInstallationID := dda.Labels[fleetInstallationIDLabel]
	_, hasTargetID := dda.Labels[fleetTargetIDLabel]
	_, hasConfigHash := dda.Annotations[fleetConfigHashAnnotation]
	_, hasExperimentHash := dda.Annotations[fleetExperimentHashAnnotation]
	_, hasCreateTaskID := dda.Annotations[fleetCreateTaskIDAnnotation]
	if !hasManagedBy && !hasConfigID && !hasManagedAgentInstallationState && !hasInstallationID && !hasTargetID && !hasConfigHash && !hasExperimentHash && !hasCreateTaskID {
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
	installationID := dda.Labels[fleetInstallationIDLabel]
	targetID := dda.Labels[fleetTargetIDLabel]
	if !d.managedAgentInstallationIdentity.Configured() {
		if installationID != "" || targetID != "" {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s has managed Agent installation identity metadata but the local managed Agent installation identity is not configured", dda.Namespace, dda.Name)}
		}
		return nil
	}
	if installationID != d.managedAgentInstallationIdentity.InstallationID {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s belongs to a different managed installation", dda.Namespace, dda.Name)}
	}
	if targetID != d.managedAgentInstallationIdentity.TargetID() {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgent %s/%s belongs to a different managed target", dda.Namespace, dda.Name)}
	}
	return nil
}

func (d *Daemon) validateFleetDatadogAgentInternalInstallation(ddai *v1alpha1.DatadogAgentInternal) error {
	installationID := ddai.Labels[fleetInstallationIDLabel]
	targetID := ddai.Labels[fleetTargetIDLabel]
	if !d.managedAgentInstallationIdentity.Configured() {
		if installationID != "" || targetID != "" {
			return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentInternal %s/%s has managed Agent installation identity metadata but the local managed Agent installation identity is not configured", ddai.Namespace, ddai.Name)}
		}
		return nil
	}
	if installationID != d.managedAgentInstallationIdentity.InstallationID {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentInternal %s/%s belongs to a different managed installation", ddai.Namespace, ddai.Name)}
	}
	if targetID != d.managedAgentInstallationIdentity.TargetID() {
		return &stateDoesntMatchError{msg: fmt.Sprintf("DatadogAgentInternal %s/%s belongs to a different managed target", ddai.Namespace, ddai.Name)}
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
