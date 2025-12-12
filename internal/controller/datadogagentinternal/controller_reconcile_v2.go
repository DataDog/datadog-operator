// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	pkgutils "github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
)

func (r *Reconciler) internalReconcileV2(ctx context.Context, instance *v1alpha1.DatadogAgentInternal) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("datadogagent", pkgutils.GetNamespacedName(instance))
	reqLogger.Info("Reconciling DatadogAgentInternal")
	// var result reconcile.Result

	// TODO: validate the resource
	// // 1. Validate the resource.
	// if err := datadoghqv2alpha1.ValidateDatadogAgent(instance); err != nil {
	// 	return result, err
	// }

	// 2. Handle finalizer logic.
	if result, err := r.handleFinalizer(reqLogger, instance, r.finalizeDDAI); utils.ShouldReturn(result, err) {
		return result, err
	}

	// 3. Set default values for GlobalConfig and Features
	instanceCopy := instance.DeepCopy()
	defaults.DefaultDatadogAgentSpec(&instanceCopy.Spec)

	// 4. Delegate to the main reconcile function.
	return r.reconcileInstanceV2(ctx, reqLogger, instanceCopy)
}

func (r *Reconciler) reconcileInstanceV2(ctx context.Context, logger logr.Logger, instance *v1alpha1.DatadogAgentInternal) (reconcile.Result, error) {
	var result reconcile.Result
	newStatus := instance.Status.DeepCopy()
	now := metav1.NewTime(time.Now())

	configuredFeatures, enabledFeatures, requiredComponents := feature.BuildFeatures(instance, &instance.Spec, instance.Status.RemoteConfigConfiguration, reconcilerOptionsToFeatureOptions(&r.options, r.log))
	// update list of enabled features for metrics forwarder
	r.updateMetricsForwardersFeatures(instance, enabledFeatures)

	// 1. Manage dependencies.
	// set the original DDAI as the owner of dependencies
	depsStore, resourceManagers := r.setupDependencies(instance, logger)

	var err error
	// only manage global dependencies for default DDAIs (profile DDAIs should share these)
	if !isDDAILabeledWithProfile(instance) {
		if err = r.manageGlobalDependencies(logger, instance, resourceManagers, requiredComponents); err != nil {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, reconcile.Result{}, err, now)
		}
	}

	// Always manage feature dependencies (even for profile DDAIs, as features may differ between profiles)
	if err = r.manageFeatureDependencies(logger, enabledFeatures, resourceManagers); err != nil {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, reconcile.Result{}, err, now)
	}

	// Only manage override dependencies for default DDAIs
	if !isDDAILabeledWithProfile(instance) {
		if err = r.overrideDependencies(logger, resourceManagers, instance); err != nil {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, reconcile.Result{}, err, now)
		}
	}
	// 2. Reconcile each component.
	// 2.a. Cluster Agent

	result, err = r.reconcileV2ClusterAgent(logger, requiredComponents, append(configuredFeatures, enabledFeatures...), instance, resourceManagers, newStatus)
	if utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}
	// Update the status to make it the ClusterAgentReconcileConditionType successful
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.ClusterAgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)

	// 2.b. Node Agent
	result, err = r.reconcileV2Agent(logger, requiredComponents, append(configuredFeatures, enabledFeatures...), instance, resourceManagers, newStatus)
	if utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.AgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)

	// 2.c. Cluster Checks Runner
	result, err = r.reconcileV2ClusterChecksRunner(logger, requiredComponents, append(configuredFeatures, enabledFeatures...), instance, resourceManagers, newStatus)
	if utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}
	// Update the status to set ClusterChecksRunnerReconcileConditionType to successful
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.ClusterChecksRunnerReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)

	// 3. Cleanup extraneous resources.
	if err = r.cleanupExtraneousResources(ctx, logger, instance, newStatus, resourceManagers); err != nil {
		logger.Error(err, "Error cleaning up extraneous resources")
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}

	// 4. Apply all dependencies.
	// Always apply dependencies, even for profile DDAIs, as they may have feature-specific dependencies
	if err = r.applyAndCleanupDependencies(ctx, logger, depsStore); err != nil {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}

	// Always requeue
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}
	return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
}

func (r *Reconciler) updateStatusIfNeededV2(logger logr.Logger, agentdeployment *v1alpha1.DatadogAgentInternal, newStatus *v1alpha1.DatadogAgentInternalStatus, result reconcile.Result, currentError error, now metav1.Time) (reconcile.Result, error) {
	if currentError == nil {
		condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionFalse, "DatadogAgent_reconcile_ok", "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgent_reconcile_error", "DatadogAgent reconcile error", false)
	}

	r.setMetricsForwarderStatusV2(logger, agentdeployment, newStatus)

	if !IsEqualStatus(&agentdeployment.Status, newStatus) {
		updateAgentDeployment := agentdeployment.DeepCopy()
		updateAgentDeployment.Status = *newStatus
		if err := r.client.Status().Update(context.TODO(), updateAgentDeployment); err != nil {
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
func (r *Reconciler) setMetricsForwarderStatusV2(logger logr.Logger, agentdeployment *v1alpha1.DatadogAgentInternal, newStatus *v1alpha1.DatadogAgentInternalStatus) {
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
			logger.V(1).Info("metrics conditions status could not be set")
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
