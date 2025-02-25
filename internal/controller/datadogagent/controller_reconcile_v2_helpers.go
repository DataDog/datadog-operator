package datadogagent

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// STEP 1 of the reconcile loop: validate

// fetchAndValidateDatadogAgent retrieves the DatadogAgent and performs basic validation.
func (r *Reconciler) fetchAndValidateDatadogAgent(ctx context.Context, req reconcile.Request) (*datadoghqv2alpha1.DatadogAgent, reconcile.Result, error) {
	instance := &datadoghqv2alpha1.DatadogAgent{}
	var result reconcile.Result
	if err := r.client.Get(ctx, req.NamespacedName, instance); err != nil {
		if apierrors.IsNotFound(err) {
			// Object not found; nothing to do.
			return nil, result, nil
		}
		return nil, result, err
	}
	// Ensure required credentials are configured.
	if instance.Spec.Global == nil || instance.Spec.Global.Credentials == nil {
		return nil, result, fmt.Errorf("credentials not configured in the DatadogAgent, can't reconcile")
	}
	return instance, result, nil
}

// STEP 2 of the reconcile loop: reconcile 3 components

// setupDependencies initializes the store and resource managers.
func (r *Reconciler) setupDependencies(instance *datadoghqv2alpha1.DatadogAgent, logger logr.Logger) (*store.Store, feature.ResourceManagers) {
	storeOptions := &store.StoreOptions{
		SupportCilium: r.options.SupportCilium,
		PlatformInfo:  r.platformInfo,
		Logger:        logger,
		Scheme:        r.scheme,
	}
	depsStore := store.NewStore(instance, storeOptions)
	resourceManagers := feature.NewResourceManagers(depsStore)
	return depsStore, resourceManagers
}

// manageFeatureDependencies iterates over features to set up dependencies.
func (r *Reconciler) manageFeatureDependencies(logger logr.Logger, features []feature.Feature, requiredComponents feature.RequiredComponents, resourceManagers feature.ResourceManagers) error {
	var errs []error
	for _, feat := range features {
		logger.V(1).Info("Managing dependencies", "featureID", feat.ID())
		if err := feat.ManageDependencies(resourceManagers, requiredComponents); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// overrideDependencies wraps the dependency override logic.
func (r *Reconciler) overrideDependencies(logger logr.Logger, resourceManagers feature.ResourceManagers, instance *datadoghqv2alpha1.DatadogAgent) error {
	errs := override.Dependencies(logger, resourceManagers, instance)
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// reconcileAgentProfiles handles profiles and agent reconciliation.
func (r *Reconciler) reconcileAgentProfiles(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent, requiredComponents feature.RequiredComponents, features []feature.Feature, resourceManagers feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus, now metav1.Time) (reconcile.Result, error) {
	// Start with a default profile and provider.
	providerList := map[string]struct{}{kubernetes.LegacyProvider: {}}
	profiles := []datadoghqv1alpha1.DatadogAgentProfile{{}}
	metrics.IntrospectionEnabled.Set(metrics.FalseValue)
	metrics.DAPEnabled.Set(metrics.FalseValue)

	// If profiles or introspection is enabled, get the node list and update providers.
	if r.options.DatadogAgentProfileEnabled || r.options.IntrospectionEnabled {
		nodeList, err := r.getNodeList(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		if r.options.IntrospectionEnabled {
			providerList = kubernetes.GetProviderListFromNodeList(nodeList, logger)
			metrics.IntrospectionEnabled.Set(metrics.TrueValue)
		}
		if r.options.DatadogAgentProfileEnabled {
			metrics.DAPEnabled.Set(metrics.TrueValue)
			var profilesByNode map[string]types.NamespacedName
			profiles, profilesByNode, err = r.profilesToApply(ctx, logger, nodeList, now, instance)
			// TODO: in main, we error on err instead of e here
			// https://github.com/DataDog/datadog-operator/blob/8ba3647fbad340015d835b6fc1cb48639502c33d/internal/controller/datadogagent/controller_reconcile_v2.go#L179-L182
			if err != nil {
				return reconcile.Result{}, err
			}
			if err = r.handleProfiles(ctx, profilesByNode, instance.Namespace); err != nil {
				return reconcile.Result{}, err
			}
		}
	}

	// Reconcile the agent for every profile and provider.
	var errs []error
	var result reconcile.Result
	for _, profile := range profiles {
		for provider := range providerList {
			res, err := r.reconcileV2Agent(logger, requiredComponents, features, instance, resourceManagers, newStatus, provider, providerList, &profile)
			if utils.ShouldReturn(res, err) {
				errs = append(errs, err)
			}
		}
	}
	if utils.ShouldReturn(result, errors.NewAggregate(errs)) {
		return result, errors.NewAggregate(errs)
	}
	condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.AgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)
	return reconcile.Result{}, nil
}

// *************************************
// STEP 3 of the reconcile loop: cleanup
// *************************************

// cleanupExtraneousResources groups the cleanup calls for old components.
func (r *Reconciler) cleanupExtraneousResources(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus, resourceManagers feature.ResourceManagers) error {
	var errs []error
	// Cleanup old DaemonSets, DCA and CCR deployments.
	// TODO: but not for DDAI, this doesn't clean up profiles/instrospection DS, we'd need to refetch / recreate them like in reconcileAgentProfiles
	now := metav1.NewTime(time.Now())
	providerList := map[string]struct{}{kubernetes.LegacyProvider: {}}
	profiles := []datadoghqv1alpha1.DatadogAgentProfile{{}}

	if r.options.DatadogAgentProfileEnabled || r.options.IntrospectionEnabled {
		// Get a node list for profiles and introspection
		nodeList, e := r.getNodeList(ctx)
		if e != nil {
			return e
		}

		if r.options.IntrospectionEnabled {
			providerList = kubernetes.GetProviderListFromNodeList(nodeList, logger)
		}

		if r.options.DatadogAgentProfileEnabled {
			var profilesByNode map[string]types.NamespacedName
			profiles, profilesByNode, e = r.profilesToApply(ctx, logger, nodeList, now, instance)
			if e != nil {
				return e
			}

			if err := r.handleProfiles(ctx, profilesByNode, instance.Namespace); err != nil {
				return err
			}
		}
	}

	if err := r.cleanupExtraneousDaemonSets(ctx, logger, instance, newStatus, providerList, profiles); err != nil {
		errs = append(errs, err)
		logger.Error(err, "Error cleaning up old DaemonSets")
	}
	if err := r.cleanupOldDCADeployments(ctx, logger, instance, resourceManagers, newStatus); err != nil {
		errs = append(errs, err)
		logger.Error(err, "Error cleaning up old DCA Deployments")
	}
	if err := r.cleanupOldCCRDeployments(ctx, logger, instance, newStatus); err != nil {
		errs = append(errs, err)
		logger.Error(err, "Error cleaning up old CCR Deployments")
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// *************************************
// STEP 4 of the reconcile loop: cleanup dependencies
// *************************************

// applyAndCleanupDependencies applies pending changes and cleans up unused dependencies.
func (r *Reconciler) applyAndCleanupDependencies(ctx context.Context, logger logr.Logger, depsStore *store.Store) error {
	var errs []error
	errs = append(errs, depsStore.Apply(ctx, r.client)...)
	if len(errs) > 0 {
		logger.V(2).Info("Dependencies apply error", "errs", errs)
		return errors.NewAggregate(errs)
	}
	if errs = depsStore.Cleanup(ctx, r.client); len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}
