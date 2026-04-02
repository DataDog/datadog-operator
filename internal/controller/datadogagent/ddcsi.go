// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
)

const datadogCSIDriverKind = "DatadogCSIDriver"

// reconcileDatadogCSIDriver creates or deletes a DatadogCSIDriver custom resource based on the
// DDA's spec.global.csi configuration.
//
// This uses direct create/delete via the API client rather than the dependency store because the
// store (store.Store) only supports built-in Kubernetes types via a hardcoded ObjectKind enum,
// type-specific factory functions, and type-specific equality checks. Extending it for a single
// custom resource would require changes across multiple packages (pkg/kubernetes, pkg/equality,
// store). Direct management is simpler and follows the same pattern used for DatadogAgentInternal.
func (r *Reconciler) reconcileDatadogCSIDriver(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent) error {
	csiEnabled := instance.Spec.Global != nil &&
		instance.Spec.Global.CSI != nil &&
		apiutils.BoolValue(instance.Spec.Global.CSI.Enabled) &&
		apiutils.BoolValue(instance.Spec.Global.CSI.CreateDatadogCSIDriver)

	if csiEnabled {
		return r.createOrUpdateDatadogCSIDriver(ctx, logger, instance)
	}
	return r.cleanupDatadogCSIDriver(ctx, logger, instance)
}

// createOrUpdateDatadogCSIDriver ensures a DatadogCSIDriver resource exists for the given DDA.
func (r *Reconciler) createOrUpdateDatadogCSIDriver(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent) error {
	// Guard: check that the DatadogCSIDriver CRD is installed on the cluster.
	if !r.platformInfo.IsResourceSupported(datadogCSIDriverKind) {
		return fmt.Errorf("DatadogCSIDriver CRD is not installed on the cluster but spec.global.csi.createDatadogCSIDriver is enabled")
	}

	ddcsiName := instance.Name
	ddcsiNamespace := instance.Namespace

	existing := &v1alpha1.DatadogCSIDriver{}
	err := r.client.Get(ctx, types.NamespacedName{Name: ddcsiName, Namespace: ddcsiNamespace}, existing)
	if err == nil {
		// Already exists — nothing to update currently (empty spec, controller applies defaults).
		logger.V(1).Info("DatadogCSIDriver already exists, skipping update", "name", ddcsiName)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to get DatadogCSIDriver %s/%s: %w", ddcsiNamespace, ddcsiName, err)
	}

	// Create a new DatadogCSIDriver with owner reference to the DDA.
	// The DatadogCSIDriver controller will apply its own defaults to the empty spec.
	ddcsi := &v1alpha1.DatadogCSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ddcsiName,
			Namespace: ddcsiNamespace,
		},
	}
	if err := controllerutil.SetControllerReference(instance, ddcsi, r.scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on DatadogCSIDriver: %w", err)
	}

	logger.Info("Creating DatadogCSIDriver", "name", ddcsiName, "namespace", ddcsiNamespace)
	if err := r.client.Create(ctx, ddcsi); err != nil {
		return fmt.Errorf("failed to create DatadogCSIDriver %s/%s: %w", ddcsiNamespace, ddcsiName, err)
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
