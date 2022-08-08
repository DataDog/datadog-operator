// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	"github.com/go-logr/logr"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
)

func (r *Reconciler) internalReconcileV2(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("datadogagent", request.NamespacedName)
	reqLogger.Info("Reconciling DatadogAgent")

	// Fetch the DatadogAgent instance
	instance := &datadoghqv2alpha1.DatadogAgent{}
	var result reconcile.Result
	err := r.client.Get(ctx, request.NamespacedName, instance)
	if err != nil {
		if apierrors.IsNotFound(err) {
			// Request object not found, could have been deleted after reconcile request.
			// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
			// Return and don't requeue
			return result, nil
		}
		// Error reading the object - requeue the request.
		return result, err
	}

	// check it the resource was properly decoded in v2
	// if not it means it was a v1
	/*if apiequality.Semantic.DeepEqual(instance.Spec, datadoghqv2alpha1.DatadogAgentSpec{}) {
		instanceV1 := &datadoghqv1alpha1.DatadogAgent{}
		if err = r.client.Get(ctx, request.NamespacedName, instanceV1); err != nil {
			if apierrors.IsNotFound(err) {
				// Request object not found, could have been deleted after reconcile request.
				// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
				// Return and don't requeue
				return result, nil
			}
			// Error reading the object - requeue the request.starting metrics server
			return result, err
		}
		if err = datadoghqv1alpha1.ConvertTo(instanceV1, instance); err != nil {
			reqLogger.Error(err, "unable to convert to v2alpha1")
			return result, err
		}
	}*/

	if result, err = r.handleFinalizer(reqLogger, instance, r.finalizeDadV2); utils.ShouldReturn(result, err) {
		return result, err
	}

	// TODO check if IsValideDatadogAgent function is needed for v2
	/*
		if err = datadoghqv2alpha1.IsValidDatadogAgent(&instance.Spec); err != nil {
			reqLogger.V(1).Info("Invalid spec", "error", err)
			return r.updateStatusIfNeeded(reqLogger, instance, &instance.Status, result, err)
		}
	*/

	// Set default values for GlobalConfig and Features
	instanceCopy := instance.DeepCopy()
	datadoghqv2alpha1.DefaultDatadogAgent(instanceCopy)

	return r.reconcileInstanceV2(ctx, reqLogger, instanceCopy)
}

func (r *Reconciler) reconcileInstanceV2(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent) (reconcile.Result, error) {
	var result reconcile.Result

	features, requiredComponents := feature.BuildFeatures(instance, reconcilerOptionsToFeatureOptions(&r.options, logger))

	override.RequiredComponents(&requiredComponents, instance.Spec.Override)

	logger.Info("requiredComponents status:", "agent", requiredComponents.Agent.IsEnabled(), "cluster-agent", requiredComponents.ClusterAgent.IsEnabled(), "cluster-checks-runner", requiredComponents.ClusterChecksRunner.IsEnabled())

	// -----------------------
	// Manage dependencies
	// -----------------------
	storeOptions := &dependencies.StoreOptions{
		SupportCilium: r.options.SupportCilium,
		VersionInfo:   r.versionInfo,
		Logger:        logger,
		Scheme:        r.scheme,
	}
	depsStore := dependencies.NewStore(instance, storeOptions)
	resourceManagers := feature.NewResourceManagers(depsStore)

	var errs []error

	// Set up dependencies required by enabled features
	for _, feat := range features {
		logger.Info("Dependency ManageDependencies", "featureID", feat.ID())
		if featErr := feat.ManageDependencies(resourceManagers, requiredComponents); featErr != nil {
			errs = append(errs, featErr)
		}
	}

	// Examine user configuration to override any external dependencies (e.g. RBACs)
	errs = append(errs, override.Dependencies(resourceManagers, instance.Spec.Override, instance.Namespace)...)

	// -----------------------------
	// Start reconcile Components
	// -----------------------------

	var err error
	newStatus := instance.Status.DeepCopy()

	if requiredComponents.ClusterAgent.IsEnabled() {
		logger.Info("ClusterAgent enabled")
		result, err = r.reconcileV2ClusterAgent(logger, features, instance, resourceManagers, newStatus)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
		}
	}

	if requiredComponents.Agent.IsEnabled() {
		requiredContainers := requiredComponents.Agent.Containers
		result, err = r.reconcileV2Agent(logger, features, instance, resourceManagers, newStatus, requiredContainers)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
		}
	}

	if requiredComponents.ClusterChecksRunner.IsEnabled() {
		result, err = r.reconcileV2ClusterChecksRunner(logger, features, instance, resourceManagers, newStatus)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
		}
	}

	// ------------------------------
	// Create and update dependencies
	// ------------------------------
	errs = append(errs, depsStore.Apply(ctx, r.client)...)
	if len(errs) > 0 {
		logger.V(2).Info("Dependencies apply error", "errs", errs)
		return result, errors.NewAggregate(errs)
	}

	// -----------------------------
	// Cleanup unused dependencies
	// -----------------------------
	// Run it after the deployments reconcile
	if errs = depsStore.Cleanup(ctx, r.client, instance.Namespace, instance.Name); len(errs) > 0 {
		return result, errors.NewAggregate(errs)
	}

	// Always requeue
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}
	return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
}

func (r *Reconciler) updateStatusIfNeededV2(logger logr.Logger, agentdeployment *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus, result reconcile.Result, currentError error) (reconcile.Result, error) {
	now := metav1.NewTime(time.Now())
	if currentError == nil {
		datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv2alpha1.DatadogAgentReconcileErrorConditionType, metav1.ConditionFalse, "DatadogAgent_reconcile_ok", "DatadogAgent reconcile ok", false)
	} else {
		datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv2alpha1.DatadogAgentReconcileErrorConditionType, metav1.ConditionTrue, "DatadogAgent_reconcile_error", "DatadogAgent reconcile error", false)
	}

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

func (r *Reconciler) finalizeDadV2(reqLogger logr.Logger, obj client.Object) {
	dda := obj.(*datadoghqv2alpha1.DatadogAgent)

	if r.options.OperatorMetricsEnabled {
		r.forwarders.Unregister(dda)
	}

	reqLogger.Info("Successfully finalized DatadogAgent")
}
