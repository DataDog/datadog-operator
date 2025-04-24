// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	pkgutils "github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func (r *Reconciler) internalReconcileV2(ctx context.Context, instance *datadoghqv2alpha1.DatadogAgent) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("datadogagent", pkgutils.GetNamespacedName(instance))
	reqLogger.Info("Reconciling DatadogAgent")
	var result reconcile.Result

	// 1. Validate the resource.
	if err := datadoghqv2alpha1.ValidateDatadogAgent(instance); err != nil {
		return result, err
	}

	// 2. Handle finalizer logic.
	if result, err := r.handleFinalizer(reqLogger, instance, r.finalizeDadV2); utils.ShouldReturn(result, err) {
		return result, err
	}

	// 3. Set default values for GlobalConfig and Features
	instanceCopy := instance.DeepCopy()
	defaults.DefaultDatadogAgent(instanceCopy)

	// 4. Delegate to the main reconcile function.
	return r.reconcileInstanceV2(ctx, reqLogger, instanceCopy)
}

func (r *Reconciler) reconcileInstanceV2(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent) (reconcile.Result, error) {
	var result reconcile.Result
	newStatus := instance.Status.DeepCopy()
	now := metav1.NewTime(time.Now())

	configuredFeatures, enabledFeatures, requiredComponents := feature.BuildFeatures(instance, reconcilerOptionsToFeatureOptions(&r.options, r.log))
	// update list of enabled features for metrics forwarder
	r.updateMetricsForwardersFeatures(instance, enabledFeatures)

	// 1. Manage dependencies.
	depsStore, resourceManagers := r.setupDependencies(instance, logger)

	var err error
	if err = r.manageGlobalDependencies(logger, instance, resourceManagers, requiredComponents); err != nil {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, reconcile.Result{}, err, now)
	}
	if err = r.manageFeatureDependencies(logger, enabledFeatures, resourceManagers); err != nil {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, reconcile.Result{}, err, now)
	}
	if err = r.overrideDependencies(logger, resourceManagers, instance); err != nil {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, reconcile.Result{}, err, now)
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

	// TODO: this feels like it should be moved somewhere else
	userSpecifiedClusterAgentToken := instance.Spec.Global.ClusterAgentToken != nil || instance.Spec.Global.ClusterAgentTokenSecret != nil
	if !userSpecifiedClusterAgentToken {
		ensureAutoGeneratedTokenInStatus(instance, newStatus, resourceManagers, logger)
	}

	// 3. Cleanup extraneous resources.
	if err = r.cleanupExtraneousResources(ctx, logger, instance, newStatus, resourceManagers); err != nil {
		logger.Error(err, "Error cleaning up extraneous resources")
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}

	// 4. Apply and cleanup dependencies.
	if err = r.applyAndCleanupDependencies(ctx, logger, depsStore); err != nil {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}

	// Always requeue
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}
	return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
}

func (r *Reconciler) updateStatusIfNeededV2(logger logr.Logger, agentdeployment *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus, result reconcile.Result, currentError error, now metav1.Time) (reconcile.Result, error) {
	if currentError == nil {
		condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionFalse, "DatadogAgent_reconcile_ok", "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgent_reconcile_error", "DatadogAgent reconcile error", false)
	}

	r.setMetricsForwarderStatusV2(logger, agentdeployment, newStatus)

	if !apiequality.Semantic.DeepEqual(&agentdeployment.Status, newStatus) {
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

func (r *Reconciler) updateDAPStatus(logger logr.Logger, profile *datadoghqv1alpha1.DatadogAgentProfile) {
	// update dap status for non-default profiles only
	if !agentprofile.IsDefaultProfile(profile.Namespace, profile.Name) {
		if err := r.client.Status().Update(context.TODO(), profile); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("unable to update DatadogAgentProfile status due to update conflict")
			}
			logger.Error(err, "unable to update DatadogAgentProfile status")
		}
	}
}

// setMetricsForwarderStatus sets the metrics forwarder status condition if enabled
func (r *Reconciler) setMetricsForwarderStatusV2(logger logr.Logger, agentdeployment *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus) {
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

func ensureAutoGeneratedTokenInStatus(instance *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus, resourceManagers feature.ResourceManagers, logger logr.Logger) {
	if instance.Status.ClusterAgent != nil && instance.Status.ClusterAgent.GeneratedToken != "" {
		// Already there; nothing to do.
		return
	}

	// The secret should have been created in the "enabledefault" feature.
	tokenSecret, exists := resourceManagers.Store().Get(
		kubernetes.SecretsKind, instance.Namespace, secrets.GetDefaultDCATokenSecretName(instance),
	)
	if !exists {
		logger.V(1).Info("expected autogenerated token was not created by the \"enabledefault\" feature")
		return
	}

	generatedToken := tokenSecret.(*corev1.Secret).Data[common.DefaultTokenKey]
	if newStatus == nil {
		newStatus = &datadoghqv2alpha1.DatadogAgentStatus{}
	}
	if newStatus.ClusterAgent == nil {
		newStatus.ClusterAgent = &datadoghqv2alpha1.DeploymentStatus{}
	}
	// Persist generated token for subsequent reconcile loops
	newStatus.ClusterAgent.GeneratedToken = string(generatedToken)
}

func (r *Reconciler) updateMetricsForwardersFeatures(dda *datadoghqv2alpha1.DatadogAgent, features []feature.Feature) {
	if r.forwarders != nil {
		featureIDs := make([]string, len(features))
		for i, f := range features {
			featureIDs[i] = string(f.ID())
		}

		r.forwarders.SetEnabledFeatures(dda, featureIDs)
	}
}
