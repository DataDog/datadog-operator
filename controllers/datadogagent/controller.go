// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package datadogagent

import (
	"context"
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
)

const (
	defaultRequeuePeriod = 15 * time.Second
)

// ReconcilerOptions provides options read from command line
type ReconcilerOptions struct {
	SupportExtendedDaemonset bool
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
