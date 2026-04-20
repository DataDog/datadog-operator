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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

const (
	datadogCSIDriverKind = "DatadogCSIDriver"
	// helmManagedCSIDriverName is the name of the CSIDriver object installed by the Datadog CSI
	// driver Helm chart. When a CSIDriver with this name and a Helm managed-by label is present,
	// the operator defers to it and does not create its own DatadogCSIDriver.
	helmManagedCSIDriverName = "k8s.csi.datadoghq.com"
	helmManagedByLabelValue  = "Helm"
)

// reconcileDatadogCSIDriver creates or deletes a DatadogCSIDriver custom resource based on the
// DDA's spec.global.csi configuration.
//
// The operator creates a DatadogCSIDriver when spec.global.csi.enabled is true, unless a
// Helm-managed CSIDriver named `k8s.csi.datadoghq.com` already exists on the cluster, in which
// case we defer to it and do not manage a DatadogCSIDriver.
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

	if !csiEnabled {
		return r.cleanupDatadogCSIDriver(ctx, logger, instance)
	}

	helmManaged, err := r.helmManagedCSIDriverExists(ctx)
	if err != nil {
		return fmt.Errorf("failed to check for Helm-managed CSIDriver: %w", err)
	}
	if helmManaged {
		logger.V(1).Info("Helm-managed CSIDriver detected, skipping DatadogCSIDriver creation", "name", helmManagedCSIDriverName)
		return r.cleanupDatadogCSIDriver(ctx, logger, instance)
	}

	return r.createOrUpdateDatadogCSIDriver(ctx, logger, instance)
}

// helmManagedCSIDriverExists returns true if a CSIDriver named `k8s.csi.datadoghq.com` exists
// with the `app.kubernetes.io/managed-by=Helm` label, signaling that the Datadog CSI driver
// was installed via the Helm chart and the operator should defer to it.
func (r *Reconciler) helmManagedCSIDriverExists(ctx context.Context) (bool, error) {
	existing := &storagev1.CSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: helmManagedCSIDriverName}, existing)
	if apierrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return existing.Labels[kubernetes.AppKubernetesManageByLabelKey] == helmManagedByLabelValue, nil
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
	// Guard: check that the DatadogCSIDriver CRD is installed on the cluster.
	if !r.platformInfo.IsResourceSupported(datadogCSIDriverKind) {
		return fmt.Errorf("DatadogCSIDriver CRD is not installed on the cluster but spec.global.csi.enabled is true")
	}

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

		// Create a new DatadogCSIDriver.
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
