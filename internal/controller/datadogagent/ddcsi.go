// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	storagev1 "k8s.io/api/storage/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
)

const (
	datadogCSIDriverKind       = "DatadogCSIDriver"
	datadogCSIDriverObjectName = "k8s.csi.datadoghq.com"
)

// reconcileDatadogCSIDriver creates or deletes a DatadogCSIDriver custom resource based on the
// DDA's spec.global.csi configuration.
//
// Behavior (backward-compatible with the long-standing `csi.enabled` semantics):
//
//   - `csi.enabled=false`  → cleanup the DDA-owned DatadogCSIDriver CR if any.
//   - `csi.enabled=true`   + `manageDatadogCSIDriver=false` → user opts out of operator
//     management; cleanup the DDA-owned CR so the user can take ownership of a DatadogCSIDriver
//     CR they maintain themselves (migration path for customizations not exposed on the DDA).
//   - `csi.enabled=true`   + operator flag `--datadogCSIDriverEnabled=false` (the default) →
//     no-op. The DDA expects a CSI driver installed externally (e.g. the Datadog CSI driver
//     Helm chart); the operator doesn't manage it.
//   - `csi.enabled=true`   + operator flag `--datadogCSIDriverEnabled=true` → operator manages
//     the driver. If our CR already exists, keep managing it (update path). If not, defer to a
//     pre-existing cluster-scoped `k8s.csi.datadoghq.com` CSIDriver when one is present (don't
//     collide with an external install); otherwise create our CR.
//
// This uses direct create/delete via the API client rather than the dependency store because the
// store (store.Store) only supports built-in Kubernetes types via a hardcoded ObjectKind enum,
// type-specific factory functions, and type-specific equality checks. Extending it for a single
// custom resource would require changes across multiple packages (pkg/kubernetes, pkg/equality,
// store). Direct management is simpler and follows the same pattern used for DatadogAgentInternal.
func (r *Reconciler) reconcileDatadogCSIDriver(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent) error {
	csiEnabled := instance.Spec.Global != nil &&
		instance.Spec.Global.CSI != nil &&
		apiutils.BoolValue(instance.Spec.Global.CSI.Enabled)

	// Cleanup the operator-owned CR when either CSI is disabled entirely, or when the user has
	// explicitly opted out of management (manageDatadogCSIDriver=false): typically a migration
	// where they want to take ownership of a DatadogCSIDriver CR they maintain themselves.
	// `||` short-circuits: if csiEnabled is false (which requires Global/CSI to be safely-nil-
	// guarded), we never dereference `.ManageDatadogCSIDriver`
	if !csiEnabled || !ptr.Deref(instance.Spec.Global.CSI.ManageDatadogCSIDriver, true) {
		return r.cleanupDatadogCSIDriver(ctx, logger, instance)
	}

	// csi.enabled=true is the long-standing signal that the Agent should use a CSI driver. The
	// operator only takes over installing it when explicitly opted in via the operator flag.
	// Otherwise we leave the driver to whoever installed it externally.
	if !r.options.DatadogCSIDriverEnabled {
		logger.V(1).Info("spec.global.csi.enabled is true but the DatadogCSIDriver controller is disabled on the operator (--datadogCSIDriverEnabled=false); the operator will not create the Datadog CSIDriver. Start the operator with --datadogCSIDriverEnabled=true to have the operator manage it automatically or install it externally.")
		return nil
	}

	return r.createOrUpdateDatadogCSIDriver(ctx, logger, instance)
}

// externalCSIDriverExists returns true if a cluster-scoped `k8s.csi.datadoghq.com` CSIDriver
// is already installed on the cluster (by any tool such as Helm, manual apply, etc.).
// Used only on the first-time create path to avoid colliding with an external install.
func (r *Reconciler) externalCSIDriverExists(ctx context.Context) (bool, error) {
	existing := &storagev1.CSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: datadogCSIDriverObjectName}, existing)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// buildDesiredDatadogCSIDriver builds the desired DatadogCSIDriver object from the DDA spec.
// The socket paths mirror the DDA's APM and DogStatsD UDS configuration so the CSI driver
// exposes the same host paths the Agent is configured to use. The node-agent tolerations are
// propagated through the CSI driver override so the driver pods are schedulable wherever the
// node Agent runs.
func (r *Reconciler) buildDesiredDatadogCSIDriver(instance *v2alpha1.DatadogAgent) (*v1alpha1.DatadogCSIDriver, error) {
	ddcsi := &v1alpha1.DatadogCSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:        instance.Name,
			Namespace:   instance.Namespace,
			Labels:      object.GetDefaultLabels(instance, instance.Name, common.GetAgentVersion(instance)),
			Annotations: object.GetDefaultAnnotations(instance),
		},
		Spec: v1alpha1.DatadogCSIDriverSpec{
			APMSocketPath: apmSocketPathFromDDA(instance),
			DSDSocketPath: dsdSocketPathFromDDA(instance),
		},
	}

	if tolerations := nodeAgentTolerationsFromDDA(instance); len(tolerations) > 0 {
		ddcsi.Spec.Override = &v1alpha1.DatadogCSIDriverOverride{
			Tolerations: tolerations,
		}
	}

	if err := controllerutil.SetControllerReference(instance, ddcsi, r.scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference on DatadogCSIDriver: %w", err)
	}
	return ddcsi, nil
}

// apmSocketPathFromDDA returns the APM UDS path configured on the DDA, or nil if unset.
func apmSocketPathFromDDA(instance *v2alpha1.DatadogAgent) *string {
	if instance.Spec.Features == nil || instance.Spec.Features.APM == nil ||
		instance.Spec.Features.APM.UnixDomainSocketConfig == nil {
		return nil
	}
	return instance.Spec.Features.APM.UnixDomainSocketConfig.Path
}

// dsdSocketPathFromDDA returns the DogStatsD UDS path configured on the DDA, or nil if unset.
func dsdSocketPathFromDDA(instance *v2alpha1.DatadogAgent) *string {
	if instance.Spec.Features == nil || instance.Spec.Features.Dogstatsd == nil ||
		instance.Spec.Features.Dogstatsd.UnixDomainSocketConfig == nil {
		return nil
	}
	return instance.Spec.Features.Dogstatsd.UnixDomainSocketConfig.Path
}

// nodeAgentTolerationsFromDDA returns the tolerations configured for the nodeAgent component
// override on the DDA, or nil if none are set.
func nodeAgentTolerationsFromDDA(instance *v2alpha1.DatadogAgent) []corev1.Toleration {
	override := instance.Spec.Override[v2alpha1.NodeAgentComponentName]
	if override == nil {
		return nil
	}
	return override.Tolerations
}

// createOrUpdateDatadogCSIDriver ensures a DatadogCSIDriver resource exists with the desired spec.
// Like DatadogAgentInternal, this treats the DatadogCSIDriver as a desired-state object: if someone
// modifies it, the operator will reconcile it back to the desired state on the next loop.
func (r *Reconciler) createOrUpdateDatadogCSIDriver(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent) error {
	desired, err := r.buildDesiredDatadogCSIDriver(instance)
	if err != nil {
		return err
	}

	existing := &v1alpha1.DatadogCSIDriver{}
	err = r.client.Get(ctx, types.NamespacedName{Name: desired.Name, Namespace: desired.Namespace}, existing)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return fmt.Errorf("failed to get DatadogCSIDriver %s/%s: %w", desired.Namespace, desired.Name, err)
		}

		// First-time create: if a cluster-scoped CSIDriver is already installed externally, defer
		// to it rather than creating our CR (avoids colliding on the cluster-scoped object name).
		// Once our CR exists, subsequent reconciles skip this check and go through the update path.
		externalExists, checkErr := r.externalCSIDriverExists(ctx)
		if checkErr != nil {
			return fmt.Errorf("failed to check for external CSIDriver: %w", checkErr)
		}
		if externalExists {
			// Debug level: fires every reconcile while the external install remains — steady-state
			// info, not an actionable event.
			logger.V(1).Info("External CSIDriver detected, not creating DatadogCSIDriver", "name", datadogCSIDriverObjectName)
			return nil
		}

		logger.Info("Creating DatadogCSIDriver", "name", desired.Name, "namespace", desired.Namespace)
		if err := r.client.Create(ctx, desired); err != nil {
			return fmt.Errorf("failed to create DatadogCSIDriver %s/%s: %w", desired.Namespace, desired.Name, err)
		}
		return nil
	}

	// Update if the spec has drifted from the desired state.
	//
	// Use a JSON merge patch rather than a full Update so only fields we actually own (.spec,
	// labels, annotations) appear in the wire payload. Finalizers added by the DatadogCSIDriver
	// controller (`finalizer.datadoghq.com/csi-driver`) and `.status` are not touched on
	// `existing`, so they are absent from the diff and preserved on the server. Without this,
	// a spec-drift update could wipe the finalizer and a subsequent DDA delete / CSI disable
	// landing before it was re-added would leak the cluster-scoped `k8s.csi.datadoghq.com`.
	if !apiequality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		patchBase := existing.DeepCopy()
		existing.Spec = desired.Spec
		existing.Labels = desired.Labels
		existing.Annotations = desired.Annotations

		logger.Info("Updating DatadogCSIDriver", "name", desired.Name, "namespace", desired.Namespace)
		if err := r.client.Patch(ctx, existing, client.MergeFrom(patchBase)); err != nil {
			return fmt.Errorf("failed to patch DatadogCSIDriver %s/%s: %w", desired.Namespace, desired.Name, err)
		}
	}

	return nil
}

// cleanupDatadogCSIDriver deletes the DDA-owned DatadogCSIDriver if it exists.
func (r *Reconciler) cleanupDatadogCSIDriver(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent) error {
	// If the CRD is not installed, there is nothing to clean up.
	if !r.platformInfo.IsResourceSupported(datadogCSIDriverKind) {
		return nil
	}

	ddcsiName := instance.Name
	ddcsiNamespace := instance.Namespace

	existing := &v1alpha1.DatadogCSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: ddcsiName, Namespace: ddcsiNamespace}, existing)
	if apierrors.IsNotFound(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("failed to get DatadogCSIDriver %s/%s for cleanup: %w", ddcsiNamespace, ddcsiName, err)
	}

	// Only delete if this DDA owns it (via controller reference).
	if !metav1.IsControlledBy(existing, instance) {
		logger.V(1).Info("DatadogCSIDriver exists but is not owned by this DatadogAgent, skipping cleanup", "name", ddcsiName)
		return nil
	}

	logger.Info("Deleting DatadogCSIDriver", "name", ddcsiName, "namespace", ddcsiNamespace)
	if err := r.client.Delete(ctx, existing); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete DatadogCSIDriver %s/%s: %w", ddcsiNamespace, ddcsiName, err)
	}
	return nil
}
