package datadogagent

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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/internal/controller/metrics"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

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

// manageGlobalDependencies manages the global dependencies for a component.
func (r *Reconciler) manageGlobalDependencies(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourceManagers feature.ResourceManagers, requiredComponents feature.RequiredComponents) error {
	var errs []error
	// Non component specific dependencies
	if err := global.ApplyGlobalDependencies(logger, dda.GetObjectMeta(), &dda.Spec, resourceManagers, false); len(err) > 0 {
		errs = append(errs, err...)
	}

	// Component specific dependencies
	if err := global.ApplyGlobalComponentDependencies(logger, dda.GetObjectMeta(), &dda.Spec, &dda.Status, resourceManagers, datadoghqv2alpha1.ClusterAgentComponentName, requiredComponents.ClusterAgent, false); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, dda.GetObjectMeta(), &dda.Spec, &dda.Status, resourceManagers, datadoghqv2alpha1.NodeAgentComponentName, requiredComponents.Agent, false); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, dda.GetObjectMeta(), &dda.Spec, &dda.Status, resourceManagers, datadoghqv2alpha1.ClusterChecksRunnerComponentName, requiredComponents.ClusterChecksRunner, false); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, dda.GetObjectMeta(), &dda.Spec, &dda.Status, resourceManagers, datadoghqv2alpha1.OtelAgentGatewayComponentName, requiredComponents.OtelAgentGateway, false); len(err) > 0 {
		errs = append(errs, err...)
	}

	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// manageFeatureDependencies iterates over features to set up dependencies.
func (r *Reconciler) manageFeatureDependencies(logger logr.Logger, features []feature.Feature, resourceManagers feature.ResourceManagers, provider string) error {
	var errs []error

	for _, feat := range features {
		logger.V(1).Info("Managing dependencies", "featureID", feat.ID())
		if err := feat.ManageDependencies(resourceManagers, provider); err != nil {
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
	errs := override.Dependencies(logger, resourceManagers, instance.GetObjectMeta(), &instance.Spec)
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
			profiles, profilesByNode, err = r.profilesToApply(ctx, logger, nodeList, now, &instance.Spec)
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
		if r.options.IntrospectionEnabled && r.useDefaultDaemonset(providerList) {
			// Use legacy provider if EKS or OpenShift providers are present to prevent daemonset overrides
			res, err := r.reconcileV2Agent(logger, requiredComponents, features, instance, resourceManagers, newStatus, kubernetes.DefaultProvider, providerList, &profile)
			if utils.ShouldReturn(res, err) {
				errs = append(errs, err)
			}
		} else {
			// Create one DaemonSet per provider (for GKE, etc.)
			for provider := range providerList {
				res, err := r.reconcileV2Agent(logger, requiredComponents, features, instance, resourceManagers, newStatus, provider, providerList, &profile)
				if utils.ShouldReturn(res, err) {
					errs = append(errs, err)
				}
			}
		}
	}
	if utils.ShouldReturn(result, errors.NewAggregate(errs)) {
		return result, errors.NewAggregate(errs)
	}
	condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.AgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)
	return reconcile.Result{}, nil
}

// useDefaultDaemonset determines if we should use a legacy provider specific Daemonset for EKS and Openshift providers
func (r *Reconciler) useDefaultDaemonset(providerList map[string]struct{}) bool {
	if len(providerList) == 0 {
		return false
	}

	for provider := range providerList {
		providerLabel, _ := kubernetes.GetProviderLabelKeyValue(provider)
		if providerLabel == kubernetes.OpenShiftProviderLabel || providerLabel == kubernetes.EKSProviderLabel {
			return true
		}
	}
	return false
}

// *************************************
// STEP 3 of the reconcile loop: cleanup
// *************************************

// cleanupExtraneousResources groups the cleanup calls for old components.
func (r *Reconciler) cleanupExtraneousResources(ctx context.Context, logger logr.Logger, instance *datadoghqv2alpha1.DatadogAgent, newStatus *datadoghqv2alpha1.DatadogAgentStatus, resourceManagers feature.ResourceManagers) error {
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
			profiles, profilesByNode, e = r.profilesToApply(ctx, logger, nodeList, now, &instance.Spec)
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
	logger.V(1).Info("Applying pending dependencies and cleaning up unused dependencies")
	var errs []error
	errs = append(errs, depsStore.Apply(ctx, r.client)...)
	if len(errs) > 0 {
		logger.Error(errors.NewAggregate(errs), "Dependencies apply error")
		return errors.NewAggregate(errs)
	}
	if errs = depsStore.Cleanup(ctx, r.client, false); len(errs) > 0 {
		logger.Error(errors.NewAggregate(errs), "Dependencies cleanup error")
		return errors.NewAggregate(errs)
	}
	return nil
}

// generateNewStatusFromDDA generates a new status from a DDA status.
// If an existing DCA token is present, it is copied to the new status.
func generateNewStatusFromDDA(ddaStatus *datadoghqv2alpha1.DatadogAgentStatus) *datadoghqv2alpha1.DatadogAgentStatus {
	status := &datadoghqv2alpha1.DatadogAgentStatus{}
	if ddaStatus != nil {
		if ddaStatus.ClusterAgent != nil && ddaStatus.ClusterAgent.GeneratedToken != "" {
			status.ClusterAgent = &datadoghqv2alpha1.DeploymentStatus{
				GeneratedToken: ddaStatus.ClusterAgent.GeneratedToken,
			}
		}
		if ddaStatus.RemoteConfigConfiguration != nil {
			status.RemoteConfigConfiguration = ddaStatus.RemoteConfigConfiguration
		}
	}
	return status
}
