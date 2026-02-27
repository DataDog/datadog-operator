package datadogagentinternal

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv1alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	componentotel "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/otelagentgateway"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
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
	if err := global.ApplyGlobalDependencies(logger, ddai.GetObjectMeta(), &ddai.Spec, resourceManagers, true); len(err) > 0 {
		errs = append(errs, err...)
	}

	// Component specific dependencies
	if err := global.ApplyGlobalComponentDependencies(logger, ddai.GetObjectMeta(), &ddai.Spec, nil, resourceManagers, datadoghqv2alpha1.ClusterAgentComponentName, requiredComponents.ClusterAgent, true); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, ddai.GetObjectMeta(), &ddai.Spec, nil, resourceManagers, datadoghqv2alpha1.NodeAgentComponentName, requiredComponents.Agent, true); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, ddai.GetObjectMeta(), &ddai.Spec, nil, resourceManagers, datadoghqv2alpha1.ClusterChecksRunnerComponentName, requiredComponents.ClusterChecksRunner, true); len(err) > 0 {
		errs = append(errs, err...)
	}
	if err := global.ApplyGlobalComponentDependencies(logger, ddai.GetObjectMeta(), &ddai.Spec, nil, resourceManagers, datadoghqv2alpha1.OtelAgentGatewayComponentName, requiredComponents.OtelAgentGateway, true); len(err) > 0 {
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
		if err := feat.ManageDependencies(resourceManagers, ""); err != nil {
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
	errs := override.Dependencies(logger, resourceManagers, instance.GetObjectMeta(), &instance.Spec)
	if len(errs) > 0 {
		return errors.NewAggregate(errs)
	}
	return nil
}

// *************************************
// STEP 3 of the reconcile loop: cleanup
// *************************************

// cleanupExtraneousResources groups the cleanup calls for old components.
func (r *Reconciler) cleanupExtraneousResources(ctx context.Context, instance *datadoghqv1alpha1.DatadogAgentInternal, newStatus *datadoghqv1alpha1.DatadogAgentInternalStatus, resourceManagers feature.ResourceManagers) error {
	logger := ctrl.LoggerFrom(ctx)
	var errs []error
	// Cleanup old DaemonSets, DCA and CCR deployments.

	// TODO: re-enable once labels are updated to use DDAI name
	// if err := r.cleanupExtraneousDaemonSets(ctx, instance, newStatus); err != nil {
	// 	errs = append(errs, err)
	// 	logger.Error(err, "Error cleaning up old DaemonSets")
	// }

	// Only cleanup DCA and CCR deployments for the default (non-profile) DDAI
	// Profile DDAIs do not manage any component except the Agent DaemonSet
	if !isDDAILabeledWithProfile(instance) {
		if err := r.cleanupOldDCADeployments(ctx, instance); err != nil {
			errs = append(errs, err)
			logger.Error(err, "Error cleaning up old DCA Deployments")
		}
		if err := r.cleanupOldCCRDeployments(ctx, instance); err != nil {
			errs = append(errs, err)
			logger.Error(err, "Error cleaning up old CCR Deployments")
		}
		if err := r.cleanupOldOtelAgentGatewayDeployments(ctx, instance); err != nil {
			errs = append(errs, err)
			logger.Error(err, "Error cleaning up old OTel Agent Gateway Deployments")
		}
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
// It excludes DDA-managed resources from cleanup to avoid competition between the DDA
// and DDAI controllers when DatadogAgentInternalEnabled is true.
func (r *Reconciler) applyAndCleanupDependencies(ctx context.Context, depsStore *store.Store) error {
	logger := ctrl.LoggerFrom(ctx)
	logger.V(1).Info("Applying pending dependencies and cleaning up unused dependencies")
	var errs []error
	errs = append(errs, depsStore.Apply(ctx, r.client)...)
	if len(errs) > 0 {
		logger.V(2).Info("Dependencies apply error", "errs", errs)
		return errors.NewAggregate(errs)
	}

	// Cleanup unused dependencies, excluding resources managed by the DDA controller.
	// When DatadogAgentInternalEnabled is true, the DDA controller manages certain
	// dependencies (like credentials, DCA token, DCA service) and labels them with
	// ManagedByDDAControllerLabelKey. We exclude these from cleanup to prevent
	// the DDAI controller from deleting them.
	if errs = depsStore.Cleanup(ctx, r.client, true); len(errs) > 0 {
		logger.V(2).Info("Dependencies cleanup error", "errs", errs)
		return errors.NewAggregate(errs)
	}
	return nil
}

func (r *Reconciler) deleteDeploymentWithEvent(ctx context.Context, logger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, deployment *appsv1.Deployment) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	existingDeployment := &appsv1.Deployment{}
	if err := r.client.Get(ctx, nsName, existingDeployment); err != nil {
		if apierrors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Deployment", "deployment.Namespace", existingDeployment.Namespace, "deployment.Name", existingDeployment.Name)
	if err := r.client.Delete(ctx, existingDeployment); err != nil {
		return reconcile.Result{}, err
	}
	// Record event only if deletion was successful
	event := buildEventInfo(existingDeployment.Name, existingDeployment.Namespace, kubernetes.DeploymentKind, datadog.DeletionEvent)
	r.recordEvent(ddai, event)

	return reconcile.Result{}, nil
}

// cleanupOldDCADeployments deletes DCA deployments when deployment name is changed using clusterAgent name override
func (r *Reconciler) cleanupOldDCADeployments(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) error {
	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
	}
	deploymentName := component.GetDeploymentNameFromDatadogAgent(ddai.GetObjectMeta(), &ddai.Spec)
	deploymentList := appsv1.DeploymentList{}
	if err := r.client.List(ctx, &deploymentList, matchLabels); err != nil {
		return err
	}
	for _, deployment := range deploymentList.Items {
		if deploymentName != deployment.Name {
			objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
			if _, err := r.deleteDeploymentWithEvent(ctx, objLogger, ddai, &deployment); err != nil {
				return err
			}
		}
	}
	return nil
}

// cleanupOldCCRDeployments deletes CCR deployments when deployment name is changed using clusterChecksRunner name override
func (r *Reconciler) cleanupOldCCRDeployments(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) error {
	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterChecksRunnerResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
	}
	deploymentName := getDeploymentNameFromCCR(ddai)
	deploymentList := appsv1.DeploymentList{}
	if err := r.client.List(ctx, &deploymentList, matchLabels); err != nil {
		return err
	}
	for _, deployment := range deploymentList.Items {
		if deploymentName != deployment.Name {
			objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
			if _, err := r.deleteDeploymentWithEvent(ctx, objLogger, ddai, &deployment); err != nil {
				return err
			}
		}
	}
	return nil
}

// getDeploymentNameFromCCR returns the expected CCR deployment name based on
// the DDAI name and clusterChecksRunner name override
func getDeploymentNameFromCCR(ddai *v1alpha1.DatadogAgentInternal) string {
	deploymentName := componentccr.GetClusterChecksRunnerName(ddai)
	if componentOverride, ok := ddai.Spec.Override[datadoghqv2alpha1.ClusterChecksRunnerComponentName]; ok {
		if componentOverride.Name != nil && *componentOverride.Name != "" {
			deploymentName = *componentOverride.Name
		}
	}
	return deploymentName
}

// cleanupOldOtelAgentGatewayDeployments deletes OTel Agent Gateway deployments when
// the deployment name is changed using otelAgentGateway name override
func (r *Reconciler) cleanupOldOtelAgentGatewayDeployments(ctx context.Context, ddai *v1alpha1.DatadogAgentInternal) error {
	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultOtelAgentGatewayResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(ddai).String(),
	}
	deploymentName := getDeploymentNameFromOtelAgentGateway(ddai)
	deploymentList := appsv1.DeploymentList{}
	if err := r.client.List(ctx, &deploymentList, matchLabels); err != nil {
		return err
	}
	for _, deployment := range deploymentList.Items {
		if deploymentName != deployment.Name {
			objLogger := ctrl.LoggerFrom(ctx).WithValues("object.kind", "Deployment", "object.namespace", deployment.Namespace, "object.name", deployment.Name)
			if _, err := r.deleteDeploymentWithEvent(ctx, objLogger, ddai, &deployment); err != nil {
				return err
			}
		}
	}
	return nil
}

// getDeploymentNameFromOtelAgentGateway returns the expected OTel Agent Gateway deployment name based on
// the DDAI name and otelAgentGateway name override
func getDeploymentNameFromOtelAgentGateway(ddai *v1alpha1.DatadogAgentInternal) string {
	deploymentName := componentotel.GetOtelAgentGatewayName(ddai)
	if componentOverride, ok := ddai.Spec.Override[datadoghqv2alpha1.OtelAgentGatewayComponentName]; ok {
		if componentOverride.Name != nil && *componentOverride.Name != "" {
			deploymentName = *componentOverride.Name
		}
	}
	return deploymentName
}
