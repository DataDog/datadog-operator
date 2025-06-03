// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	pkgutils "github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
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
	if r.options.DatadogAgentInternalEnabled {
		if result, err := r.handleFinalizer(reqLogger, instance, r.finalizeDDAWithDDAI); utils.ShouldReturn(result, err) {
			return result, err
		}
	}
	if result, err := r.handleFinalizer(reqLogger, instance, r.finalizeDadV2); utils.ShouldReturn(result, err) {
		return result, err
	}

	// 3. Set default values for GlobalConfig and Features
	instanceCopy := instance.DeepCopy()
	defaults.DefaultDatadogAgentSpec(&instanceCopy.Spec)

	// 4. Delegate to the main reconcile function.
	if r.options.DatadogAgentInternalEnabled {
		return r.reconcileInstanceV3(ctx, reqLogger, instanceCopy)
	}
	return r.reconcileInstanceV2(ctx, reqLogger, instanceCopy)
}

func (r *Reconciler) reconcileInstanceV3(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent) (reconcile.Result, error) {
	var result reconcile.Result
	now := metav1.NewTime(time.Now())
	ddais := []*datadoghqv1alpha1.DatadogAgentInternal{}
	ddaStatusCopy := instance.Status.DeepCopy()
	newDDAStatus := generateNewStatusFromDDA(ddaStatusCopy)

	// Manage dependencies
	if err := r.manageDDADependenciesWithDDAI(ctx, logger, instance, newDDAStatus); err != nil {
		return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, err, now)
	}

	// Generate default DDAI object from DDA
	ddai, err := r.generateDDAIFromDDA(instance)
	if err != nil {
		return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, err, now)
	}
	ddais = append(ddais, ddai)

	// Profiles
	// TODO: introspection
	sendProfileEnabledMetric(r.options.DatadogAgentProfileEnabled)
	if r.options.DatadogAgentProfileEnabled {
		profileDDAIs, e := r.applyProfilesToDDAISpec(ctx, logger, ddai, now)
		if e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}
		ddais = profileDDAIs
	}

	// Create or update the DDAI object in k8s
	for _, ddai := range ddais {
		if e := r.createOrUpdateDDAI(logger, ddai); e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}

		// Add DDAI status to DDA status
		if e := r.addDDAIStatusToDDAStatus(logger, newDDAStatus, ddai.ObjectMeta); e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}

		// TODO: copy remote config status from DDA to DDAI
	}

	// Prevent the reconcile loop from stopping by requeueing the DDAI object after a period of time
	result.RequeueAfter = defaultRequeuePeriod
	return r.updateStatusIfNeededV2(logger, instance, newDDAStatus, result, err, now)
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
	condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.ClusterAgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)

	// 2.b. Node Agent and profiles
	// TODO: ignore profiles and introspection for DDAI

	if result, err = r.reconcileAgentProfiles(ctx, logger, instance, requiredComponents, append(configuredFeatures, enabledFeatures...), resourceManagers, newStatus, now); utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}

	// 2.c. Cluster Checks Runner
	result, err = r.reconcileV2ClusterChecksRunner(logger, requiredComponents, append(configuredFeatures, enabledFeatures...), instance, resourceManagers, newStatus)
	if utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err, now)
	}
	// Update the status to set ClusterChecksRunnerReconcileConditionType to successful
	condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.ClusterChecksRunnerReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)

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
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionFalse, "DatadogAgent_reconcile_ok", "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgent_reconcile_error", "DatadogAgent reconcile error", false)
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
			condition.UpdateDatadogAgentStatusConditions(
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

func (r *Reconciler) updateMetricsForwardersFeatures(dda *datadoghqv2alpha1.DatadogAgent, features []feature.Feature) {
	if r.forwarders != nil {
		featureIDs := make([]string, len(features))
		for i, f := range features {
			featureIDs[i] = string(f.ID())
		}

		r.forwarders.SetEnabledFeatures(dda, featureIDs)
	}
}

// profilesToApply gets a list of profiles and returns the ones that should be
// applied in the cluster.
// - If there are no profiles, it returns the default profile.
// - If there are no conflicting profiles, it returns all the profiles plus the default one.
// - If there are conflicting profiles, it returns a subset that does not
// conflict plus the default one. When there are conflicting profiles, the
// oldest one is the one that takes precedence. When two profiles share an
// identical creation timestamp, the profile whose name is alphabetically first
// is considered to have priority.
// This function also returns a map that maps each node name to the profile that
// should be applied to it.
func (r *Reconciler) profilesToApply(ctx context.Context, logger logr.Logger, nodeList []corev1.Node, now metav1.Time, ddaSpec *datadoghqv2alpha1.DatadogAgentSpec) ([]datadoghqv1alpha1.DatadogAgentProfile, map[string]types.NamespacedName, error) {
	profilesList := datadoghqv1alpha1.DatadogAgentProfileList{}
	err := r.client.List(ctx, &profilesList)
	if err != nil {
		return nil, nil, err
	}

	var profileListToApply []datadoghqv1alpha1.DatadogAgentProfile
	profileAppliedByNode := make(map[string]types.NamespacedName, len(nodeList))

	sortedProfiles := agentprofile.SortProfiles(profilesList.Items)
	for _, profile := range sortedProfiles {
		maxUnavailable := agentprofile.GetMaxUnavailable(logger, ddaSpec, &profile, len(nodeList), &r.options.ExtendedDaemonsetOptions)
		profileAppliedByNode, err = agentprofile.ApplyProfile(logger, &profile, nodeList, profileAppliedByNode, now, maxUnavailable)
		r.updateDAPStatus(logger, &profile)
		if err != nil {
			// profile is invalid or conflicts
			logger.Error(err, "profile cannot be applied", "datadogagentprofile", profile.Name, "datadogagentprofile_namespace", profile.Namespace)
			continue
		}
		profileListToApply = append(profileListToApply, profile)
	}

	// add default profile
	profileListToApply = agentprofile.ApplyDefaultProfile(profileListToApply, profileAppliedByNode, nodeList)

	return profileListToApply, profileAppliedByNode, nil
}

func (r *Reconciler) getNodeList(ctx context.Context) ([]corev1.Node, error) {
	nodeList := corev1.NodeList{}
	err := r.client.List(ctx, &nodeList)
	if err != nil {
		return nodeList.Items, err
	}

	return nodeList.Items, nil
}
