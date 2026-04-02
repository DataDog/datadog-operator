// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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

// buildDesiredDatadogCSIDriver builds the desired DatadogCSIDriver object from the DDA spec.
// Currently the spec is empty (the DatadogCSIDriver controller applies its own defaults).
// In a follow-up, this will construct the spec based on relevant DDA fields.
func (r *Reconciler) buildDesiredDatadogCSIDriver(instance *v2alpha1.DatadogAgent) (*v1alpha1.DatadogCSIDriver, error) {
	ddcsi := &v1alpha1.DatadogCSIDriver{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}
	if err := controllerutil.SetControllerReference(instance, ddcsi, r.scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference on DatadogCSIDriver: %w", err)
	}
	return ddcsi, nil
}

// createOrUpdateDatadogCSIDriver ensures a DatadogCSIDriver resource exists with the desired spec.
// Like DatadogAgentInternal, this treats the DatadogCSIDriver as a desired-state object: if someone
// modifies it, the operator will reconcile it back to the desired state on the next loop.
func (r *Reconciler) createOrUpdateDatadogCSIDriver(ctx context.Context, logger logr.Logger, instance *v2alpha1.DatadogAgent) error {
	// Guard: check that the DatadogCSIDriver CRD is installed on the cluster.
	if !r.platformInfo.IsResourceSupported(datadogCSIDriverKind) {
		return fmt.Errorf("DatadogCSIDriver CRD is not installed on the cluster but spec.global.csi.createDatadogCSIDriver is enabled")
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
	if !apiequality.Semantic.DeepEqual(existing.Spec, desired.Spec) {
		logger.Info("Updating DatadogCSIDriver", "name", desired.Name, "namespace", desired.Namespace)
		if err := kubernetes.UpdateFromObject(ctx, r.client, desired, existing.ObjectMeta); err != nil {
			return fmt.Errorf("failed to update DatadogCSIDriver %s/%s: %w", desired.Namespace, desired.Name, err)
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
