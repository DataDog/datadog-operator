// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) internalReconcileV2(ctx context.Context, instance *v1alpha1.DatadogAgentInternal) (reconcile.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	logger.Info("Reconciling DatadogAgentInternal")
	// var result reconcile.Result

	// TODO: validate the resource
	// // 1. Validate the resource.
	// if err := datadoghqv2alpha1.ValidateDatadogAgent(instance); err != nil {
	// 	return result, err
	// }

	// 2. Handle finalizer logic.
	final := finalizer.NewFinalizer(logger, r.client, r.deleteResource(), defaultRequeuePeriod, defaultErrRequeuePeriod)
	if result, err := final.HandleFinalizer(ctx, instance, "", constants.DatadogAgentInternalFinalizer); utils.ShouldReturn(result, err) {
		return result, err
	}

	// 3. Set default values for GlobalConfig and Features
	instanceCopy := instance.DeepCopy()
	defaults.DefaultDatadogAgentSpec(&instanceCopy.Spec)

	// 4. Delegate to the main reconcile function.
	return r.reconcileInstanceV2(ctx, instanceCopy)
}

func (r *Reconciler) reconcileInstanceV2(ctx context.Context, instance *v1alpha1.DatadogAgentInternal) (reconcile.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	var result reconcile.Result
	newStatus := instance.Status.DeepCopy()
	now := metav1.NewTime(time.Now())

	configuredFeatures, enabledFeatures, requiredComponents, unsupportedFeatures := feature.BuildFeatures(instance, &instance.Spec, instance.Status.RemoteConfigConfiguration, reconcilerOptionsToFeatureOptions(ctx, r.apiReader))
	// update list of enabled features for metrics forwarder
	r.updateMetricsForwardersFeatures(instance, enabledFeatures)

	// Provider-support gate. An enabled feature the provider rejects blocks the whole reconcile
	// (mirrors Helm's install-time fail) before any dependency/resource is created; a degraded
	// feature only warns. Evaluated on the verdict BuildFeatures returned.
	if r.providerSupportBlocks(logger, instance, unsupportedFeatures, newStatus, now) {
		return r.updateStatusIfNeededV2(ctx, instance, newStatus, reconcile.Result{}, nil, now)
	}

	// 1. Manage dependencies.
	// set the original DDAI as the owner of dependencies
	depsStore, resourceManagers := r.setupDependencies(ctx, instance)

	var err error
	// only manage dependencies for default DDAIs
	if !isDDAILabeledWithProfile(instance) {
		if err = r.manageGlobalDependencies(ctx, instance, resourceManagers, requiredComponents); err != nil {
			return r.updateStatusIfNeededV2(ctx, instance, newStatus, reconcile.Result{}, err, now)
		}
		if err = r.manageFeatureDependencies(enabledFeatures, resourceManagers); err != nil {
			return r.updateStatusIfNeededV2(ctx, instance, newStatus, reconcile.Result{}, err, now)
		}
		if err = r.overrideDependencies(ctx, resourceManagers, instance); err != nil {
			return r.updateStatusIfNeededV2(ctx, instance, newStatus, reconcile.Result{}, err, now)
		}
		// 1. Apply and cleanup dependencies before reconciling components to ensure deps exist at reconciliation time.
		if err = r.applyAndCleanupDependencies(ctx, depsStore); err != nil {
			return r.updateStatusIfNeededV2(ctx, instance, newStatus, reconcile.Result{}, err, now)
		}
	}

	provider := instance.GetAnnotations()[kubernetes.ProviderAnnotationKey]

	// 2. Reconcile each component (DCA, CCR, OTel Gateway) using the component registry.
	// Profile DDAIs only manage the Node Agent DaemonSet, so skip components to avoid cleanup of un-related components / running un-necessary operations.
	if !isDDAILabeledWithProfile(instance) {
		params := &ReconcileComponentParams{
			DDAI:               instance,
			RequiredComponents: requiredComponents,
			Features:           append(configuredFeatures, enabledFeatures...),
			ResourceManagers:   resourceManagers,
			Status:             newStatus,
			Provider:           provider,
		}

		result, err = r.componentRegistry.ReconcileComponents(ctx, params)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(ctx, instance, newStatus, result, err, now)
		}
	}

	// 2.b. Node Agent. provider is read from the DDAI annotation above — the DDA
	// controller stamps gke-autopilot there for both the OOTB and experimental opt-in paths.
	result, err = r.reconcileV2Agent(ctx, requiredComponents, append(configuredFeatures, enabledFeatures...), instance, resourceManagers, newStatus, provider)
	if utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(ctx, instance, newStatus, result, err, now)
	}
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.AgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)

	// 3. Cleanup extraneous resources.
	if err = r.cleanupExtraneousResources(ctx, instance, newStatus, resourceManagers); err != nil {
		logger.Error(err, "Error cleaning up extraneous resources")
		return r.updateStatusIfNeededV2(ctx, instance, newStatus, result, err, now)
	}

	// 4. Cleanup stale dependencies (only for default DDAIs).
	if !isDDAILabeledWithProfile(instance) {
		if cleanupErrs := depsStore.Cleanup(ctx, r.client, true); len(cleanupErrs) > 0 {
			return r.updateStatusIfNeededV2(ctx, instance, newStatus, result, cleanupErrs[0], now)
		}
	}

	// Always requeue
	if result.IsZero() {
		result.RequeueAfter = defaultRequeuePeriod
	}
	return r.updateStatusIfNeededV2(ctx, instance, newStatus, result, err, now)
}

// providerSupportBlocks acts on the provider-support verdict returned by BuildFeatures. Features
// the provider Rejects block the whole reconcile (a FeatureNotSupportedOnProvider condition +
// warning event are set, and true is returned). Degraded features emit a warning event but do not
// block. When nothing is rejected, any prior FeatureNotSupportedOnProvider condition is cleared to
// False. Each event is mirrored to the operator log (k8s events are namespace-scoped and easy to
// miss).
func (r *Reconciler) providerSupportBlocks(logger logr.Logger, instance *v1alpha1.DatadogAgentInternal, results []feature.ProviderSupportResult, newStatus *v1alpha1.DatadogAgentInternalStatus, now metav1.Time) bool {
	provider := instance.GetAnnotations()[kubernetes.ProviderAnnotationKey]

	var rejected, degraded []string
	for _, res := range results {
		switch res.Level {
		case feature.Rejected:
			rejected = append(rejected, string(res.ID))
		case feature.Degraded:
			degraded = append(degraded, string(res.ID))
		}
	}

	for _, id := range degraded {
		msg := fmt.Sprintf("Feature %q is not fully supported on provider %q; reconciling anyway", id, provider)
		logger.Info(msg, "feature", id, "provider", provider, "supportLevel", "degraded")
		r.recorder.Event(instance, corev1.EventTypeWarning, "FeatureDegradedOnProvider", msg)
	}

	if len(rejected) > 0 {
		msg := fmt.Sprintf("Features %v are not supported on provider %q; blocking reconcile", rejected, provider)
		err := fmt.Errorf("provider %q does not support features %v", provider, rejected)
		logger.Error(err, msg, "features", rejected, "provider", provider, "supportLevel", "rejected")
		r.recorder.Event(instance, corev1.EventTypeWarning, "FeatureNotSupportedOnProvider", msg)
		condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.FeatureNotSupportedOnProviderConditionType, metav1.ConditionTrue, "FeatureNotSupportedOnProvider", msg, false)
		return true
	}

	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.FeatureNotSupportedOnProviderConditionType, metav1.ConditionFalse, "AllFeaturesSupportedOnProvider", "All enabled features are supported on the provider", false)
	return false
}

func (r *Reconciler) updateStatusIfNeededV2(ctx context.Context, agentdeployment *v1alpha1.DatadogAgentInternal, newStatus *v1alpha1.DatadogAgentInternalStatus, result reconcile.Result, currentError error, now metav1.Time) (reconcile.Result, error) {
	logger := ctrl.LoggerFrom(ctx)
	if currentError == nil {
		condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionFalse, "DatadogAgent_reconcile_ok", "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgent_reconcile_error", "DatadogAgent reconcile error", false)
	}

	r.setMetricsForwarderStatusV2(ctx, agentdeployment, newStatus)

	if !IsEqualStatus(&agentdeployment.Status, newStatus) {
		updateAgentDeployment := agentdeployment.DeepCopy()
		updateAgentDeployment.Status = *newStatus
		if err := r.client.Status().Update(ctx, updateAgentDeployment); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("unable to update DatadogAgent status due to update conflict")
				return reconcile.Result{RequeueAfter: time.Second}, nil
			}
			logger.Error(err, "unable to update DatadogAgent status")
			return reconcile.Result{}, err
		}
	}

	return result, currentError
}

// setMetricsForwarderStatus sets the metrics forwarder status condition if enabled
func (r *Reconciler) setMetricsForwarderStatusV2(ctx context.Context, agentdeployment *v1alpha1.DatadogAgentInternal, newStatus *v1alpha1.DatadogAgentInternalStatus) {
	if r.options.OperatorMetricsEnabled {
		if forwarderCondition := r.forwarders.MetricsForwarderStatusForObj(agentdeployment); forwarderCondition != nil {
			condition.UpdateDatadogAgentInternalStatusConditions(
				newStatus,
				forwarderCondition.LastUpdateTime,
				forwarderCondition.ConditionType,
				condition.GetMetav1ConditionStatus(forwarderCondition.Status),
				forwarderCondition.Reason,
				forwarderCondition.Message,
				true,
			)
		} else {
			ctrl.LoggerFrom(ctx).V(1).Info("metrics conditions status could not be set")
		}
	}
}

func (r *Reconciler) updateMetricsForwardersFeatures(dda *v1alpha1.DatadogAgentInternal, features []feature.Feature) {
	if r.forwarders != nil {
		featureIDs := make([]string, len(features))
		for i, f := range features {
			featureIDs[i] = string(f.ID())
		}

		r.forwarders.SetEnabledFeatures(dda, featureIDs)
	}
}
