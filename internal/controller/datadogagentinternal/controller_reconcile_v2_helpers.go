package datadogagentinternal

import (
	"context"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/util/errors"

	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/override"
)

// STEP 2 of the reconcile loop: reconcile 3 components

// setupDependencies initializes the store and resource managers.
func (r *Reconciler) setupDependencies(owner, namingInstance *datadoghqv1alpha1.DatadogAgentInternal, logger logr.Logger) (*store.Store, feature.ResourceManagers) {
	storeOptions := &store.StoreOptions{
		SupportCilium:  r.options.SupportCilium,
		PlatformInfo:   r.platformInfo,
		Logger:         logger,
		Scheme:         r.scheme,
		NamingInstance: namingInstance,
	}
	depsStore := store.NewStore(owner, storeOptions)
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

// *************************************
// STEP 3 of the reconcile loop: cleanup
// *************************************

// cleanupExtraneousResources groups the cleanup calls for old components.
func (r *Reconciler) cleanupExtraneousResources(ctx context.Context, logger logr.Logger, instance *datadoghqv1alpha1.DatadogAgentInternal, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, resourceManagers feature.ResourceManagers) error {
	var errs []error
	// Cleanup old DaemonSets, DCA and CCR deployments.

	// TODO: re-enable once labels are updated to use DDAI name
	// if err := r.cleanupExtraneousDaemonSets(ctx, logger, instance, newStatus); err != nil {
	// 	errs = append(errs, err)
	// 	logger.Error(err, "Error cleaning up old DaemonSets")
	// }
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
	// TODO: modify cleanup to prevent DDA dependency deletion
	// if errs = depsStore.Cleanup(ctx, r.client); len(errs) > 0 {
	// 	return errors.NewAggregate(errs)
	// }
	return nil
}
