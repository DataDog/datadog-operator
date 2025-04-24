package datadogagentinternal

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/store"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

// STEP 2 of the reconcile loop: reconcile 3 components

// setupDependencies initializes the store and resource managers.
func (r *Reconciler) setupDependencies(instance *datadoghqv1alpha1.DatadogAgentInternal, logger logr.Logger) (*store.Store, feature.ResourceManagers) {
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

// manageGlobalDependencies manages the global dependencies for a component.
func (r *Reconciler) manageGlobalDependencies(logger logr.Logger, ddai *datadoghqv1alpha1.DatadogAgentInternal, resourceManagers feature.ResourceManagers, requiredComponents feature.RequiredComponents) error {
	var errs []error
	// Non component specific dependencies
	if err := global.ApplyGlobalDependencies(logger, ddai, resourceManagers); len(err) > 0 {
		errs = append(errs, err...)
	}

	// Component specific dependencies
	if err := global.ApplyGlobalComponentDependencies(logger, ddai, resourceManagers, datadoghqv2alpha1.ClusterAgentComponentName, requiredComponents.ClusterAgent); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, ddai, resourceManagers, datadoghqv2alpha1.NodeAgentComponentName, requiredComponents.Agent); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, ddai, resourceManagers, datadoghqv2alpha1.ClusterChecksRunnerComponentName, requiredComponents.ClusterChecksRunner); len(err) > 0 {
		errs = append(errs, err...)
	}

	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// manageFeatureDependencies iterates over features to set up dependencies.
func (r *Reconciler) manageFeatureDependencies(logger logr.Logger, features []feature.Feature, resourceManagers feature.ResourceManagers) error {
	var errs []error
	for _, feat := range features {
		logger.V(1).Info("Managing dependencies", "featureID", feat.ID())
		if err := feat.ManageDependencies(resourceManagers); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// overrideDependencies wraps the dependency override logic.
func (r *Reconciler) overrideDependencies(logger logr.Logger, resourceManagers feature.ResourceManagers, instance *datadoghqv1alpha1.DatadogAgentInternal) error {
	errs := override.Dependencies(logger, resourceManagers, instance)
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// reconcileAgentProfiles handles profiles and agent reconciliation.
func (r *Reconciler) reconcileAgentProfiles(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogAgentInternal, requiredComponents feature.RequiredComponents, features []feature.Feature, resourceManagers feature.ResourceManagers, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, now metav1.Time) (reconcile.Result, error) {
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
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, now, common.AgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)
	return reconcile.Result{}, nil
}

// *************************************
// STEP 3 of the reconcile loop: cleanup
// *************************************

// cleanupExtraneousResources groups the cleanup calls for old components.
func (r *Reconciler) cleanupExtraneousResources(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogAgentInternal, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, resourceManagers feature.ResourceManagers) error {
	var errs []error
	// Cleanup old DaemonSets, DCA and CCR deployments.
	now := metav1.NewTime(time.Now())
	providerList := map[string]struct{}{kubernetes.LegacyProvider: {}}
	profiles := []datadoghqv1alpha1.DatadogAgentProfile{{}}

	// Repeat of the code from reconcileAgentProfiles, but this will be removed in DDAI controller since this logic will be from DDA to DDAI.
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
