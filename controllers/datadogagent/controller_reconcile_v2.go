package datadogagent

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/dependencies"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
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
	if !apiequality.Semantic.DeepEqual(instance.Spec, datadoghqv2alpha1.DatadogAgentSpec{}) {
		instanceV1 := &datadoghqv1alpha1.DatadogAgent{}
		if err = r.client.Get(ctx, request.NamespacedName, instanceV1); err != nil {
			if apierrors.IsNotFound(err) {
				// Request object not found, could have been deleted after reconcile request.
				// Owned objects are automatically garbage collected. For additional cleanup logic use finalizers.
				// Return and don't requeue
				return result, nil
			}
			// Error reading the object - requeue the request.
			return result, err
		}
		if err = datadoghqv1alpha1.ConvertTo(instanceV1, instance); err != nil {
			reqLogger.Error(err, "unable to convert to v2alpha1")
			return result, err
		}
	}

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

	return r.reconcileInstanceV2(ctx, reqLogger, instance)
}

func (r *Reconciler) reconcileInstanceV2(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent) (reconcile.Result, error) {
	var result reconcile.Result

	features, requiredComponents, err := feature.BuildFeatures(instance, reconcilerOptionsToFeatureOptions(&r.options, logger))
	if err != nil {
		return result, fmt.Errorf("unable to build features, err: %w", err)
	}
	logger.Info("requiredComponents status:", "agent", requiredComponents.Agent, "cluster-agent", requiredComponents.ClusterAgent, "cluster-check-runner", requiredComponents.ClusterCheckRunner)

	// -----------------------
	// Manage dependencies
	// -----------------------
	storeOptions := &dependencies.StoreOptions{
		SupportCilium: r.options.SupportCilium,
		Logger:        logger,
	}
	depsStore := dependencies.NewStore(storeOptions)
	resourcesManager := feature.NewResourceManagers(depsStore)
	var errs []error
	for _, feat := range features {
		if featErr := feat.ManageDependencies(resourcesManager); err != nil {
			errs = append(errs, featErr)
		}
	}
	// Now create/update dependencies
	errs = append(errs, depsStore.Apply(ctx, r.client)...)
	if len(errs) > 0 {
		logger.V(2).Info("Dependencies apply error", "errs", errs)
		return result, errors.NewAggregate(errs)
	}
	// -----------------------------
	// Start reconcile Components
	// -----------------------------

	newStatus := instance.Status.DeepCopy()

	if requiredComponents.ClusterAgent.IsEnabled() {
		result, err = r.reconcileV2ClusterAgent(logger, features, instance, newStatus)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
		}
	}

	if requiredComponents.Agent.IsEnabled() {
		result, err = r.reconcileV2Agent(logger, features, instance, newStatus)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
		}
	}

	if requiredComponents.ClusterCheckRunner.IsEnabled() {
		result, err = r.reconcileV2ClusterCheckRunner(logger, features, instance, newStatus)
		if utils.ShouldReturn(result, err) {
			return r.updateStatusIfNeededV2(logger, instance, newStatus, result, err)
		}
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
	// TODO(operator-ga): implement updateStatusIfNeededV2

	// now := metav1.NewTime(time.Now())

	return result, currentError
}

func (r *Reconciler) finalizeDadV2(reqLogger logr.Logger, obj client.Object) {
	dda := obj.(*datadoghqv2alpha1.DatadogAgent)

	r.forwarders.Unregister(dda)
	reqLogger.Info("Successfully finalized DatadogAgent")
}
