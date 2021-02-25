// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/v1alpha1"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"

	utilserrors "k8s.io/apimachinery/pkg/util/errors"
)

// AgentFeature describes a feature in Datadog agent that can be subject to restriction
type AgentFeature string

// IsValid returns true if the feature is known
func (f AgentFeature) IsValid() bool {
	switch f {
	case
		AgentFeatureSystemProbe,
		AgentFeatureAPM,
		AgentFeatureLogs,
		AgentFeatureRuntimeSecurity,
		AgentFeatureCompliance:
		return true
	default:
		return false
	}
}

const (
	defaultRequeuePeriod = 15 * time.Second

	// AgentFeatureSystemProbe represents the system probe container
	AgentFeatureSystemProbe AgentFeature = "system-probe"
	// AgentFeatureAPM represents the APM/trace-agent container
	AgentFeatureAPM = "apm"
	// AgentFeatureLogs represents the logs-agent (core agent container)
	AgentFeatureLogs = "logs"
	// AgentFeatureRuntimeSecurity represents the runtime-security features
	AgentFeatureRuntimeSecurity = "runtime-security"
	// AgentFeatureCompliance represents the compliance features
	AgentFeatureCompliance = "compliance"
)

// ReconcilerOptions provides options read from command line
type ReconcilerOptions struct {
	SupportExtendedDaemonset bool
	// AllowedContainerRegistries is a list of allowed container registries
	AllowedContainerRegistries []string
	// DisallowedAgentFeatures is a list of allowed agent features
	DisallowedAgentFeatures []AgentFeature
	// AgentHostStoragePath is a path allowed for use on the host
	AgentHostStoragePath string
}

// Reconciler is the internal reconciler for Datadog Agent
type Reconciler struct {
	options     ReconcilerOptions
	client      client.Client
	versionInfo *version.Info
	scheme      *runtime.Scheme
	log         logr.Logger
	recorder    record.EventRecorder
	forwarders  datadog.MetricForwardersManager
}

// NewReconciler returns a reconciler for DatadogAgent
func NewReconciler(options ReconcilerOptions, client client.Client, versionInfo *version.Info,
	scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, metricForwarder datadog.MetricForwardersManager) (*Reconciler, error) {
	return &Reconciler{
		options:     options,
		client:      client,
		versionInfo: versionInfo,
		scheme:      scheme,
		log:         log,
		recorder:    recorder,
		forwarders:  metricForwarder,
	}, nil
}

// Reconcile is similar to reconciler.Reconcile interface, but taking a context
func (r *Reconciler) Reconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	resp, err := r.internalReconcile(ctx, request)
	r.forwarders.ProcessError(getMonitoredObj(request), err)
	return resp, err
}

func (r *Reconciler) internalReconcile(ctx context.Context, request reconcile.Request) (reconcile.Result, error) {
	reqLogger := r.log.WithValues("datadogagent", request.NamespacedName)
	reqLogger.Info("Reconciling DatadogAgent")

	// Fetch the DatadogAgent instance
	instance := &datadoghqv1alpha1.DatadogAgent{}
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

	if result, err = r.handleFinalizer(reqLogger, instance); shouldReturn(result, err) {
		return result, err
	}

	if !datadoghqv1alpha1.IsDefaultedDatadogAgent(instance) {
		reqLogger.Info("Defaulting values")
		defaultedInstance := datadoghqv1alpha1.DefaultDatadogAgent(instance)
		err = r.client.Update(ctx, defaultedInstance)
		if err != nil {
			reqLogger.Error(err, "failed to update DatadogAgent")
			return reconcile.Result{}, err
		}
		// DatadogAgent is now defaulted return and requeue
		return reconcile.Result{Requeue: true}, nil
	}

	newStatus := instance.Status.DeepCopy()

	if err = datadoghqv1alpha1.IsValidDatadogAgent(&instance.Spec); err != nil {
		reqLogger.Info("Invalid spec")
		return r.updateStatusIfNeeded(reqLogger, instance, newStatus, result, err)
	}

	if err = r.validateRestrictions(&instance.Spec); err != nil {
		return r.updateStatusIfNeeded(reqLogger, instance, newStatus, result, err)
	}

	reconcileFuncs :=
		[]reconcileFuncInterface{
			r.reconcileClusterAgent,
			r.reconcileClusterChecksRunner,
			r.reconcileAgent,
		}
	for _, reconcileFunc := range reconcileFuncs {
		result, err = reconcileFunc(reqLogger, instance, newStatus)
		if shouldReturn(result, err) {
			return r.updateStatusIfNeeded(reqLogger, instance, newStatus, result, err)
		}
	}

	// Always requeue
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}
	return r.updateStatusIfNeeded(reqLogger, instance, newStatus, result, err)
}

type reconcileFuncInterface func(logger logr.Logger, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error)

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus, result reconcile.Result, currentError error) (reconcile.Result, error) {
	now := metav1.NewTime(time.Now())
	condition.UpdateDatadogAgentStatusConditionsFailure(newStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeReconcileError, currentError)
	if currentError == nil {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeActive, corev1.ConditionTrue, "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeActive, corev1.ConditionFalse, "DatadogAgent reconcile error", false)
	}

	// get metrics forwarder status
	if metricsCondition := r.forwarders.MetricsForwarderStatusForObj(agentdeployment); metricsCondition != nil {
		logger.V(1).Info("metrics conditions status not available")
		condition.SetDatadogAgentStatusCondition(newStatus, metricsCondition)
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

func (r *Reconciler) validateRestrictions(spec *datadoghqv1alpha1.DatadogAgentSpec) error {
	// Check registries used
	errs := []error{
		r.allowImage(spec.Agent.Image.Name),
	}

	if spec.ClusterAgent != nil {
		errs = append(errs, r.allowImage(spec.ClusterAgent.Image.Name))
	}

	if spec.ClusterChecksRunner != nil {
		errs = append(errs, r.allowImage(spec.ClusterChecksRunner.Image.Name))
	}

	// Check features used
	if isSystemProbeEnabled(spec) {
		errs = append(errs, r.allowFeature(AgentFeatureSystemProbe))
	}

	if datadoghqv1alpha1.BoolValue(spec.Agent.Log.Enabled) {
		errs = append(errs, r.allowFeature(AgentFeatureLogs))
	}

	if isAPMEnabled(spec) {
		errs = append(errs, r.allowFeature(AgentFeatureAPM))
	}

	if isRuntimeSecurityEnabled(spec) {
		errs = append(errs, r.allowFeature(AgentFeatureRuntimeSecurity))
	}

	if isComplianceEnabled(spec) {
		errs = append(errs, r.allowFeature(AgentFeatureCompliance))
	}

	return utilserrors.NewAggregate(errs)
}

func (r *Reconciler) allowImage(imageName string) error {
	if len(r.options.AllowedContainerRegistries) == 0 {
		return nil
	}
	for _, registry := range r.options.AllowedContainerRegistries {
		if strings.HasPrefix(imageName, registry+"/") {
			return nil
		}
	}
	return fmt.Errorf("image %s is disallowed in the current configuration", imageName)
}

func (r *Reconciler) allowFeature(f AgentFeature) error {
	if len(r.options.DisallowedAgentFeatures) == 0 {
		return nil
	}
	for _, disallowed := range r.options.DisallowedAgentFeatures {
		if f == disallowed {
			return fmt.Errorf("%s agent feature cannot be enabled in the current configuration", f)
		}
	}
	return nil
}
