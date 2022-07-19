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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/version"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1/patch"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"

	// Use to register features
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/clusterchecks"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/cspm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/dogstatsd"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/dummy"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/enabledefault"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/eventcollection"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/kubernetesstatecore"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/logcollection"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/npm"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/oomkill"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/prometheusscrape"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/tcpqueuelength"
	_ "github.com/DataDog/datadog-operator/controllers/datadogagent/feature/usm"
)

const (
	defaultRequeuePeriod = 15 * time.Second
)

// ReconcilerOptions provides options read from command line
type ReconcilerOptions struct {
	SupportExtendedDaemonset bool
	SupportCilium            bool
	OperatorMetricsEnabled   bool
	V2Enabled                bool
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
	scheme *runtime.Scheme, log logr.Logger, recorder record.EventRecorder, metricForwarder datadog.MetricForwardersManager,
) (*Reconciler, error) {
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
	var resp reconcile.Result
	var err error

	if r.options.V2Enabled {
		resp, err = r.internalReconcileV2(ctx, request)
	} else {
		resp, err = r.internalReconcile(ctx, request)
	}

	r.metricsForwarderProcessError(request, err)
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

	if result, err = r.handleFinalizer(reqLogger, instance, r.finalizeDadV1); utils.ShouldReturn(result, err) {
		return result, err
	}

	var patched bool
	if instance, patched = patch.CopyAndPatchDatadogAgent(instance); patched {
		reqLogger.Info("Patching DatadogAgent")
		err = r.client.Update(ctx, instance)
		if err != nil {
			reqLogger.Error(err, "failed to update DatadogAgent")
			return reconcile.Result{}, err
		}
	}
	if err = datadoghqv1alpha1.IsValidDatadogAgent(&instance.Spec); err != nil {
		reqLogger.V(1).Info("Invalid spec", "error", err)
		return r.updateStatusIfNeeded(reqLogger, instance, &instance.Status, result, err)
	}

	instOverrideStatus := datadoghqv1alpha1.DefaultDatadogAgent(instance)
	instance, result, err = r.updateOverrideIfNeeded(reqLogger, instance, instOverrideStatus, result)
	if err != nil {
		return result, err
	}

	return r.reconcileInstance(ctx, reqLogger, instance)
}

func reconcilerOptionsToFeatureOptions(opts *ReconcilerOptions, logger logr.Logger) *feature.Options {
	return &feature.Options{
		SupportExtendedDaemonset: opts.SupportExtendedDaemonset,
		Logger:                   logger,
	}
}

func (r *Reconciler) reconcileInstance(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogAgent) (reconcile.Result, error) {
	var result reconcile.Result

	features, requiredComponents, err := feature.BuildFeaturesV1(instance, reconcilerOptionsToFeatureOptions(&r.options, logger))
	if err != nil {
		return result, fmt.Errorf("unable to build features, err: %w", err)
	}
	logger.Info("requiredComponents status:", "agent", requiredComponents.Agent, "cluster-agent", requiredComponents.ClusterAgent, "cluster-checks-runner", requiredComponents.ClusterChecksRunner)

	// -----------------------
	// Manage dependencies
	// -----------------------
	storeOptions := &dependencies.StoreOptions{
		SupportCilium: r.options.SupportCilium,
		Logger:        logger,
		Scheme:        r.scheme,
	}
	depsStore := dependencies.NewStore(instance, storeOptions)
	resourcesManager := feature.NewResourceManagers(depsStore)
	var errs []error
	for _, feat := range features {
		if featErr := feat.ManageDependencies(resourcesManager, requiredComponents); err != nil {
			errs = append(errs, featErr)
		}
	}
	// Now create/update dependencies
	errs = append(errs, depsStore.Apply(ctx, r.client)...)
	if len(errs) > 0 {
		logger.V(2).Info("Dependencies apply error", "errs", errs)
		return result, errors.NewAggregate(errs)
	}
	// -----------------------

	newStatus := instance.Status.DeepCopy()
	reconcileFuncs := []reconcileFuncInterface{
		r.reconcileClusterAgent,
		r.reconcileClusterChecksRunner,
		r.reconcileAgent,
	}
	for _, reconcileFunc := range reconcileFuncs {
		result, err = reconcileFunc(logger, features, instance, newStatus)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeeded(logger, instance, newStatus, result, err)
		}
	}

	// Cleanup unused dependencies
	// Run it after the deployments reconcile
	if errs = depsStore.Cleanup(ctx, r.client, instance.Namespace, instance.Name); len(errs) > 0 {
		return result, errors.NewAggregate(errs)
	}

	// Always requeue
	if !result.Requeue && result.RequeueAfter == 0 {
		result.RequeueAfter = defaultRequeuePeriod
	}
	return r.updateStatusIfNeeded(logger, instance, newStatus, result, err)
}

type reconcileFuncInterface func(logger logr.Logger, features []feature.Feature, dda *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) (reconcile.Result, error)

func (r *Reconciler) updateOverrideIfNeeded(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newOverride *datadoghqv1alpha1.DatadogAgentStatus, result reconcile.Result) (*datadoghqv1alpha1.DatadogAgent, reconcile.Result, error) {
	// We returned the most up to date instance to avoid conflict during the updateStatusIfNeeded after all the reconcile cycles.
	updateAgentDeployment := agentdeployment.DeepCopy()
	if !apiequality.Semantic.DeepEqual(agentdeployment.Status.DefaultOverride, newOverride.DefaultOverride) {
		updateAgentDeployment.Status.DefaultOverride = newOverride.DefaultOverride
		if err := r.client.Status().Update(context.TODO(), updateAgentDeployment); err != nil {
			if apierrors.IsConflict(err) {
				logger.V(1).Info("unable to update DatadogAgent status override due to update conflict", "error", err)
				return agentdeployment, reconcile.Result{RequeueAfter: time.Second}, nil
			}
			logger.Error(err, "unable to update DatadogAgent status override", "error", err)
			return agentdeployment, reconcile.Result{}, err
		}
		// Restore the Spec as it can be changed by Status().Update()
		updateAgentDeployment.Spec = agentdeployment.Spec
	}
	return updateAgentDeployment, result, nil
}

func (r *Reconciler) updateStatusIfNeeded(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus, result reconcile.Result, currentError error) (reconcile.Result, error) {
	now := metav1.NewTime(time.Now())
	condition.UpdateDatadogAgentStatusConditionsFailure(newStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeReconcileError, currentError)
	if currentError == nil {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeActive, corev1.ConditionTrue, "DatadogAgent reconcile ok", false)
	} else {
		condition.UpdateDatadogAgentStatusConditions(newStatus, now, datadoghqv1alpha1.DatadogAgentConditionTypeActive, corev1.ConditionFalse, "DatadogAgent reconcile error", false)
	}

	r.setMetricsForwarderStatus(logger, agentdeployment, newStatus)
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
func (r *Reconciler) setMetricsForwarderStatus(logger logr.Logger, agentdeployment *datadoghqv1alpha1.DatadogAgent, newStatus *datadoghqv1alpha1.DatadogAgentStatus) {
	if r.options.OperatorMetricsEnabled {
		if metricsCondition := r.forwarders.MetricsForwarderStatusForObj(agentdeployment); metricsCondition != nil {
			logger.V(1).Info("metrics conditions status not available")
			condition.SetDatadogAgentStatusCondition(newStatus, metricsCondition)
		}
	}
}

// metricsForwarderProcessError convert the reconciler errors into metrics if metrics forwarder is enabled
func (r *Reconciler) metricsForwarderProcessError(req reconcile.Request, err error) {
	if r.options.OperatorMetricsEnabled {
		r.forwarders.ProcessError(getMonitoredObj(req), err)
	}
}
