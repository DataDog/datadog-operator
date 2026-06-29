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
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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

	// Resolve the cluster provider before touching anything: while detection is
	// still warming up, hold (requeue) rather than apply a provider we're about to
	// change.
	provider, providerSource, hold := r.resolveClusterProvider(instance)
	if hold {
		logger.V(1).Info("Cluster provider not yet detected; requeuing before touching resources")
		return reconcile.Result{RequeueAfter: clusterProviderGateRequeue}, nil
	}
	setClusterProviderStatus(newDDAStatus, provider, providerSource, now)

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
	ddai, err := r.generateDDAIFromDDA(instance, provider)
	if err != nil {
		return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, err, now)
	}
	ddais = append(ddais, ddai)

	// Profiles
	sendProfileEnabledMetric(r.options.DatadogAgentProfileEnabled)
	if r.options.DatadogAgentProfileEnabled {
		dsName := component.GetDaemonSetNameFromDatadogAgent(instance, &instance.Spec)
		dsNSName := types.NamespacedName{
			Namespace: instance.Namespace,
			Name:      dsName,
		}
		maxUnavailable := agentprofile.GetMaxUnavailableFromSpecAndEDS(&instance.Spec, &r.options.ExtendedDaemonsetOptions, nil)

		// Profiles normally render their own DDAIs from the base DDAI. Shared
		// component config contributed by profiles is accumulated on the default
		// DDAI, because there is only one Cluster Agent/CCR for the cluster.
		defaultDDAI := ddai.DeepCopy()
		appliedProfiles, e := r.reconcileProfiles(ctx, dsNSName, maxUnavailable, defaultDDAI)
		if e != nil {
			return r.updateStatusIfNeededV2(logger, instance, ddaStatusCopy, result, e, now)
		}
		profileDDAIs, e := r.applyProfilesToDDAISpec(ddai, defaultDDAI, appliedProfiles)
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
		// Status write committed. Emit one Kubernetes event per detected
		// experiment phase transition. Gated by DD_FLEET_MANAGEMENT_EVENTS_ENABLED
		// — see experiment_events.go.
		r.emitExperimentTransitionEvent(agentdeployment, agentdeployment.Status.Experiment, newStatus.Experiment)
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

// Cluster provider status source labels.
const (
	clusterProviderSourceUser     = "UserSpecified"
	clusterProviderSourceDetected = "Detected"
	clusterProviderSourceNone     = "NoProviderDetected"
	clusterProviderSourceDisabled = "Disabled"

	// clusterProviderReasonDetected is the ClusterProviderDetected condition reason
	// used when a specific provider was auto-detected. It also tells
	// resolveClusterProvider that the persisted value came from detection (not a
	// user override), which scopes the no-downgrade guard.
	clusterProviderReasonDetected = "ProviderDetected"
)

// resolveClusterProvider resolves the cluster provider and reports whether the
// reconcile should hold (requeue without touching resources) until detection is
// ready. Precedence: user override > live detection > persisted status.
func (r *Reconciler) resolveClusterProvider(instance *datadoghqv2alpha1.DatadogAgent) (provider, source string, hold bool) {
	// 0. User override. The operator only writes the annotation on the DDAI, so its
	// presence on the DDA is definitionally user-set.
	if v, ok := instance.Annotations[kubernetes.ProviderAnnotationKey]; ok {
		return v, clusterProviderSourceUser, false
	}

	detector := r.options.ClusterProviderDetector
	if detector == nil { // detection disabled: empty provider, no status
		return "", clusterProviderSourceDisabled, false
	}

	statusProvider := instance.Status.ClusterProvider

	// 1. Live detection. Anti-flap: keep a previously *detected* specific provider
	// over a live default (a transient blip shouldn't tear down config). This guard
	// applies only to detected values — a user override that was set and then
	// removed is not pinned, so removing the annotation cleanly returns to detection.
	if live, ok := detector.Provider(); ok {
		if wasProviderDetected(instance) && kubernetes.IsSpecificProvider(statusProvider) && !kubernetes.IsSpecificProvider(live) {
			return statusProvider, clusterProviderSourceDetected, false
		}
		return live, clusterProviderSourceDetected, false
	}

	// 2. Persisted fallback: retain a previously *detected* provider across restarts
	// / leader changes / detection outages so we don't churn during warm-up. A
	// user-set value is not retained here — if the override was removed, warm-up
	// falls through to the gate rather than re-serving the stale value (which would
	// also poison the condition reason and re-pin it).
	if statusProvider != "" && wasProviderDetected(instance) {
		return statusProvider, clusterProviderSourceDetected, false
	}

	// 3. Hold while detection is still warming up; once the gate elapses, proceed
	// with no provider.
	if detector.InGracePeriod(clusterProviderGateTimeout) {
		return "", "", true
	}
	return "", clusterProviderSourceNone, false
}

// wasProviderDetected reports whether the persisted status.ClusterProvider was set
// by auto-detection rather than a user override, read from the
// ClusterProviderDetected condition reason. Only detected values are protected by
// the no-downgrade guard.
func wasProviderDetected(instance *datadoghqv2alpha1.DatadogAgent) bool {
	cond := meta.FindStatusCondition(instance.Status.Conditions, common.ClusterProviderDetectedConditionType)
	return cond != nil && cond.Reason == clusterProviderReasonDetected
}

// setClusterProviderStatus records the resolved cluster provider on the DDA
// status (the durable value read back by resolveClusterProvider) and updates the
// ClusterProviderDetected condition for visibility.
func setClusterProviderStatus(status *datadoghqv2alpha1.DatadogAgentStatus, provider, source string, now metav1.Time) {
	if source == clusterProviderSourceDisabled {
		return
	}
	status.ClusterProvider = provider

	var reason, message string
	switch {
	case source == clusterProviderSourceUser:
		reason = clusterProviderSourceUser
		message = fmt.Sprintf("Cluster provider set to %q by user annotation.", provider)
	case source == clusterProviderSourceDetected && kubernetes.IsSpecificProvider(provider):
		reason = clusterProviderReasonDetected
		message = fmt.Sprintf("Cluster provider detected as %q.", provider)
	default:
		// Detected-but-default, or no provider resolved (gate elapsed).
		reason = clusterProviderSourceNone
		message = "No cloud provider detected; provider-specific configuration not applied."
	}
	condition.UpdateDatadogAgentStatusConditions(status, now, common.ClusterProviderDetectedConditionType, metav1.ConditionTrue, reason, message, false)
}
