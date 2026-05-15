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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/defaults"
	"github.com/DataDog/datadog-operator/internal/controller/finalizer"
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
	final := finalizer.NewFinalizer(reqLogger, r.client, r.deleteResource(reqLogger), defaultRequeuePeriod, defaultErrRequeuePeriod)
	if result, err := final.HandleFinalizer(ctx, instance, "", datadogAgentFinalizer); utils.ShouldReturn(result, err) {
		return result, err
	}

	// 3. Set default values for GlobalConfig and Features
	instanceCopy := instance.DeepCopy()
	defaults.DefaultDatadogAgentSpec(&instanceCopy.Spec)

	// 4. Delegate to the main reconcile function.
	return r.reconcileInstanceV3(ctx, reqLogger, instanceCopy)
}

func (r *Reconciler) reconcileInstanceV3(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent) (reconcile.Result, error) {
	// Set up field manager for crd apply
	if r.fieldManager == nil {
		f, err := newFieldManager(r.client, r.scheme, getDDAIGVK())
		if err != nil {
			return reconcile.Result{}, err
		}
		r.fieldManager = f
	}

	var result reconcile.Result
	now := metav1.NewTime(time.Now())
	ddais := []*datadoghqv1alpha1.DatadogAgentInternal{}
	ddaStatusCopy := instance.Status.DeepCopy()
	newDDAStatus := generateNewStatusFromDDA(ddaStatusCopy)

	if r.options.CreateControllerRevisions {
		revList, err := r.listRevisions(ctx, instance)
		if err != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, err, now)
		}
		if err := r.manageExperiment(ctx, instance, newDDAStatus, now, revList); err != nil {
			return r.updateStatusIfNeededV2(logger, instance, newDDAStatus, result, err, now)
		}
		if err := r.manageRevision(ctx, instance, revList, newDDAStatus); err != nil {
			return r.updateStatusIfNeededV2(logger, instance, newDDAStatus, result, err, now)
		}
	}

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
		dsName := component.GetDaemonSetNameFromDatadogAgent(instance, &instance.Spec)
		dsNSName := types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      dsName,
		}
		maxUnavailable := agentprofile.GetMaxUnavailableFromSpecAndEDS(&instance.Spec, &r.options.ExtendedDaemonsetOptions, nil)
		appliedProfiles, e := r.reconcileProfiles(ctx, dsNSName, maxUnavailable)
		if e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}
		profileDDAIs, e := r.applyProfilesToDDAISpec(ddai, appliedProfiles)
		if e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}
		ddais = profileDDAIs
	}

	// Create or update the DDAI object in k8s
	for _, ddai := range ddais {
		if e := r.createOrUpdateDDAI(ddai); e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}

		// Add DDAI status to DDA status
		if e := r.addDDAIStatusToDDAStatus(newDDAStatus, ddai.ObjectMeta); e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}

		// Add DDA remote config status to DDAI status
		if res, e := r.addRemoteConfigStatusToDDAIStatus(ctx, newDDAStatus, ddai.ObjectMeta); utils.ShouldReturn(res, e) {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}
	}

	// Clean up unused DDAI objects
	if e := r.cleanUpUnusedDDAIs(ctx, ddais); e != nil {
		return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
	}

	// Prevent the reconcile loop from stopping by requeueing the DDAI object after a period of time
	result.RequeueAfter = defaultRequeuePeriod
	return r.updateStatusIfNeededV2(logger, instance, newDDAStatus, result, err, now)
}

func (r *Reconciler) updateStatusIfNeededV2(logger logr.Logger, agentdeployment *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus, result reconcile.Result, currentError error, now metav1.Time) (reconcile.Result, error) {
	if currentError == nil {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionFalse, "DatadogAgent_reconcile_ok", "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.DatadogAgentReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgent_reconcile_error", "DatadogAgent reconcile error", false)
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

func (r *Reconciler) getNodeList(ctx context.Context) ([]corev1.Node, error) {
	nodeList := corev1.NodeList{}
	err := r.client.List(ctx, &nodeList)
	if err != nil {
		return nodeList.Items, err
	}

	return nodeList.Items, nil
}
