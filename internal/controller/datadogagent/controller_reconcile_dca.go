// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) reconcileV2ClusterAgent(ctx context.Context, logger logr.Logger, requiredComponents feature.RequiredComponents, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	var result reconcile.Result
	now := metav1.NewTime(time.Now())

	// Get provider list for introspection
	providerList := map[string]struct{}{kubernetes.LegacyProvider: {}}
	if r.options.IntrospectionEnabled {
		nodeList, err := r.getNodeList(ctx)
		if err != nil {
			return reconcile.Result{}, err
		}
		providerList = kubernetes.GetProviderListFromNodeList(nodeList, logger)
		logger.Info("providerList for cluster agent", "providerList", providerList)
	}

	// One one provider (one deployment) for cluster agent, keep only the non-default provider
	if len(providerList) > 1 {
		for provider := range providerList {
			if provider == kubernetes.DefaultProvider {
				delete(providerList, provider)
				break
			}
		}
	}

	// Reconcile cluster agent for each provider
	var errs []error
	for provider := range providerList {
		logger.Info("DCA providerList", "provider", provider)
		// Start by creating the Default Cluster-Agent deployment
		deployment := componentdca.NewDefaultClusterAgentDeployment(dda)
		podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

		// Set Global setting on the default deployment
		global.ApplyGlobalSettingsClusterAgent(logger, podManagers, dda, resourcesManager, requiredComponents)

		// Apply features changes on the Deployment.Spec.Template
		var featErrors []error
		for _, feat := range features {
			logger.Info("DCA feature", "feature", feat.ID())
			if errFeat := feat.ManageClusterAgent(podManagers, provider); errFeat != nil {
				featErrors = append(featErrors, errFeat)
			}
		}
		if len(featErrors) > 0 {
			err := utilerrors.NewAggregate(featErrors)
			updateStatusV2WithClusterAgent(deployment, newStatus, now, metav1.ConditionFalse, "ClusterAgent feature error", err.Error())
			return result, err
		}

		deploymentLogger := logger.WithValues("component", datadoghqv2alpha1.ClusterAgentComponentName, "provider", provider)

		// The requiredComponents can change depending on if updates to features result in disabled components
		dcaEnabled := requiredComponents.ClusterAgent.IsEnabled()

		// If Override is defined for the clusterAgent component, apply the override on the PodTemplateSpec, it will cascade to container.
		if componentOverride, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
			if apiutils.BoolValue(componentOverride.Disabled) {
				if dcaEnabled {
					// The override supersedes what's set in requiredComponents; update status to reflect the conflict
					condition.UpdateDatadogAgentStatusConditions(
						newStatus,
						metav1.NewTime(time.Now()),
						common.OverrideReconcileConflictConditionType,
						metav1.ConditionTrue,
						"OverrideConflict",
						"ClusterAgent component is set to disabled",
						true,
					)
				}
				deleteStatusV2WithClusterAgent(newStatus)
				return r.cleanupV2ClusterAgent(deploymentLogger, dda, deployment, resourcesManager, newStatus)
			}
			override.PodTemplateSpec(logger, podManagers, componentOverride, datadoghqv2alpha1.ClusterAgentComponentName, dda.Name)
			override.Deployment(deployment, componentOverride)
		} else if !dcaEnabled {
			// If the override is not defined, then disable based on dcaEnabled value
			deleteStatusV2WithClusterAgent(newStatus)
			return r.cleanupV2ClusterAgent(deploymentLogger, dda, deployment, resourcesManager, newStatus)
		}

		// Add provider label to deployment
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels[constants.MD5AgentDeploymentProviderLabelKey] = provider

		res, err := r.createOrUpdateDeployment(deploymentLogger, dda, deployment, newStatus, updateStatusV2WithClusterAgent)
		if err != nil {
			errs = append(errs, err)
		}
		if utils.ShouldReturn(res, err) {
			return res, err
		}
	}

	if len(errs) > 0 {
		return result, utilerrors.NewAggregate(errs)
	}

	condition.UpdateDatadogAgentStatusConditions(newStatus, now, common.ClusterAgentReconcileConditionType, metav1.ConditionTrue, "reconcile_succeed", "reconcile succeed", false)
	return reconcile.Result{}, nil
}

func updateStatusV2WithClusterAgent(dca *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterAgent = condition.UpdateDeploymentStatus(dca, newStatus.ClusterAgent, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.ClusterAgentReconcileConditionType, status, reason, message, true)
}

func deleteStatusV2WithClusterAgent(newStatus *datadoghqv2alpha1.DatadogAgentStatus) {
	newStatus.ClusterAgent = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, common.ClusterAgentReconcileConditionType)
}

func (r *Reconciler) cleanupV2ClusterAgent(logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, deployment *appsv1.Deployment, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	// ClusterAgentDeployment attached to this instance
	clusterAgentDeployment := &appsv1.Deployment{}
	if err := r.client.Get(context.TODO(), nsName, clusterAgentDeployment); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Cluster Agent Deployment", "deployment.Namespace", clusterAgentDeployment.Namespace, "deployment.Name", clusterAgentDeployment.Name)
	event := buildEventInfo(clusterAgentDeployment.Name, clusterAgentDeployment.Namespace, kubernetes.ClusterRoleBindingKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	if err := r.client.Delete(context.TODO(), clusterAgentDeployment); err != nil {
		return reconcile.Result{}, err
	}

	// Delete associated RBACs as well
	rbacManager := resourcesManager.RBACManager()
	logger.Info("Deleting Cluster Agent RBACs")
	if err := rbacManager.DeleteServiceAccountByComponent(string(datadoghqv2alpha1.ClusterAgentComponentName), dda.Namespace); err != nil {
		return reconcile.Result{}, err
	}
	if err := rbacManager.DeleteRoleByComponent(string(datadoghqv2alpha1.ClusterAgentComponentName), dda.Namespace); err != nil {
		return reconcile.Result{}, err
	}
	if err := rbacManager.DeleteClusterRoleByComponent(string(datadoghqv2alpha1.ClusterAgentComponentName)); err != nil {
		return reconcile.Result{}, err
	}

	newStatus.ClusterAgent = nil

	return reconcile.Result{}, nil
}

// cleanupOldDCADeployments deletes DCA deployments when a DCA Deployment's name is changed using clusterAgent name override
func (r *Reconciler) cleanupOldDCADeployments(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus) error {
	matchLabels := client.MatchingLabels{
		apicommon.AgentDeploymentComponentLabelKey: constants.DefaultClusterAgentResourceSuffix,
		kubernetes.AppKubernetesManageByLabelKey:   "datadog-operator",
		kubernetes.AppKubernetesPartOfLabelKey:     object.NewPartOfLabelValue(dda).String(),
	}
	deploymentName := component.GetDeploymentNameFromDatadogAgent(dda, &dda.Spec)
	deploymentList := appsv1.DeploymentList{}
	if err := r.client.List(ctx, &deploymentList, matchLabels); err != nil {
		return err
	}
	for _, deployment := range deploymentList.Items {
		if deploymentName != deployment.Name {
			if _, err := r.cleanupV2ClusterAgent(logger, dda, &deployment, resourcesManager, newStatus); err != nil {
				return err
			}
		}
	}

	return nil
}
