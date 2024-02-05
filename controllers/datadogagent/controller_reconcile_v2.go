// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/agentprofile"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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

	if instance.Spec.Global == nil || instance.Spec.Global.Credentials == nil {
		return result, fmt.Errorf("credentials not configured in the DatadogAgent, can't reconcile")
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
	newStatus := instance.Status.DeepCopy()

	features, requiredComponents := feature.BuildFeatures(instance, reconcilerOptionsToFeatureOptions(&r.options, logger))
	// update list of enabled features for metrics forwarder
	r.updateMetricsForwardersFeatures(instance, features)

	// -----------------------
	// Manage dependencies
	// -----------------------
	storeOptions := &dependencies.StoreOptions{
		SupportCilium: r.options.SupportCilium,
		VersionInfo:   r.versionInfo,
		PlatformInfo:  r.platformInfo,
		Logger:        logger,
		Scheme:        r.scheme,
	}
	depsStore := dependencies.NewStore(instance, storeOptions)
	resourceManagers := feature.NewResourceManagers(depsStore)

	var errs []error

	// Set up dependencies required by enabled features
	for _, feat := range features {
		logger.V(1).Info("Dependency ManageDependencies", "featureID", feat.ID())
		if featErr := feat.ManageDependencies(resourceManagers, requiredComponents); featErr != nil {
			errs = append(errs, featErr)
		}
	}

	// Examine user configuration to override any external dependencies (e.g. RBACs)
	errs = append(errs, override.Dependencies(logger, resourceManagers, instance)...)

	userSpecifiedClusterAgentToken := instance.Spec.Global.ClusterAgentToken != nil || instance.Spec.Global.ClusterAgentTokenSecret != nil
	if !userSpecifiedClusterAgentToken {
		ensureAutoGeneratedTokenInStatus(instance, newStatus, resourceManagers, logger)
	}

	// -----------------------------
	// Start reconcile Components
	// -----------------------------

	var err error

	result, err = r.reconcileV2ClusterAgent(logger, requiredComponents, features, instance, resourceManagers, newStatus)
	if utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
	}

	requiredContainers := requiredComponents.Agent.Containers

	profiles, profilesByNode, err := r.profilesToApply(ctx, logger)
	if err != nil {
		return reconcile.Result{}, err
	}

	for _, profile := range profiles {
		result, err = r.reconcileV2Agent(logger, requiredComponents, features, instance, resourceManagers, newStatus, requiredContainers, &profile)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
		}
	}

	if err = r.handleProfiles(ctx, profiles, profilesByNode, instance.Namespace); err != nil {
		return reconcile.Result{}, err
	}

	result, err = r.reconcileV2ClusterChecksRunner(logger, requiredComponents, features, instance, resourceManagers, newStatus)
	if utils.ShouldReturn(result, err) {
		return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
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
	if errs = depsStore.Cleanup(ctx, r.client); len(errs) > 0 {
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

// setMetricsForwarderStatus sets the metrics forwarder status condition if enabled
func (r *Reconciler) setMetricsForwarderStatusV2(logger logr.Logger, agentdeployment *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus) {
	if r.options.OperatorMetricsEnabled {
		if forwarderCondition := r.forwarders.MetricsForwarderStatusForObj(agentdeployment); forwarderCondition != nil {
			datadoghqv2alpha1.UpdateDatadogAgentStatusConditions(
				newStatus,
				forwarderCondition.LastUpdateTime,
				forwarderCondition.ConditionType,
				datadoghqv2alpha1.GetMetav1ConditionStatus(forwarderCondition.Status),
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
		kubernetes.SecretsKind, instance.Namespace, datadoghqv2alpha1.GetDefaultDCATokenSecretName(instance),
	)
	if !exists {
		logger.V(1).Info("expected autogenerated token was not created by the \"enabledefault\" feature")
		return
	}

	generatedToken := tokenSecret.(*corev1.Secret).Data[apicommon.DefaultTokenKey]
	if newStatus == nil {
		newStatus = &datadoghqv2alpha1.DatadogAgentStatus{}
	}
	if newStatus.ClusterAgent == nil {
		newStatus.ClusterAgent = &commonv1.DeploymentStatus{}
	}
	// Persist generated token for subsequent reconcile loops
	newStatus.ClusterAgent.GeneratedToken = string(generatedToken)
}

func (r *Reconciler) updateMetricsForwardersFeatures(dda *datadoghqv2alpha1.DatadogAgent, features []feature.Feature) {
	// todo: fix nil pointer metrics forwarder
	// if r.forwarders != nil {
	// 	r.forwarders.SetEnabledFeatures(dda, features)
	// }
}

func (r *Reconciler) profilesToApply(ctx context.Context, logger logr.Logger) ([]datadoghqv1alpha1.DatadogAgentProfile, map[string]types.NamespacedName, error) {
	profilesList := datadoghqv1alpha1.DatadogAgentProfileList{}
	err := r.client.List(ctx, &profilesList)
	if err != nil {
		return nil, nil, err
	}

	nodes := r.nodeStore.GetNodes()

	return agentprofile.ProfilesToApply(profilesList.Items, nodes, logger)
}
