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
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilerrors "k8s.io/apimachinery/pkg/util/errors"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/override"
	"github.com/DataDog/datadog-operator/pkg/condition"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/datadog"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (r *Reconciler) reconcileV2ClusterAgent(ctx context.Context, logger logr.Logger, requiredComponents feature.RequiredComponents, features []feature.Feature, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus, provider string) (reconcile.Result, error) {
	var result reconcile.Result
	now := metav1.NewTime(time.Now())
	componentName := datadoghqv2alpha1.ClusterAgentComponentName
	deploymentLogger := logger.WithValues("component", componentName)

	// Start by creating the Default Cluster-Agent deployment
	deployment := componentdca.NewDefaultClusterAgentDeployment(dda.GetObjectMeta(), &dda.Spec)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	global.ApplyGlobalSettingsClusterAgent(logger, podManagers, dda.GetObjectMeta(), &dda.Spec, resourcesManager, requiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	var featErrors []error
	for _, feat := range features {
		if errFeat := feat.ManageClusterAgent(podManagers, provider); errFeat != nil {
			featErrors = append(featErrors, errFeat)
		}
	}
	if len(featErrors) > 0 {
		err := utilerrors.NewAggregate(featErrors)
		updateStatusV2WithClusterAgent(deployment, newStatus, now, metav1.ConditionFalse, fmt.Sprintf("%s feature error", componentName), err.Error())
		return result, err
	}

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
					fmt.Sprintf("%s component is set to disabled", componentName),
					true,
				)
			}
			deleteStatusV2WithClusterAgent(newStatus)
			return r.cleanupV2ClusterAgent(ctx, deploymentLogger, dda, deployment, resourcesManager, newStatus)
		}
		override.PodTemplateSpec(logger, podManagers, componentOverride, componentName, dda.Name)
		override.Deployment(deployment, componentOverride)
	} else if !dcaEnabled {
		// If the override is not defined, then disable based on dcaEnabled value
		deleteStatusV2WithClusterAgent(newStatus)
		return r.cleanupV2ClusterAgent(ctx, deploymentLogger, dda, deployment, resourcesManager, newStatus)
	}

	if r.options.IntrospectionEnabled {
		// Add provider label to deployment
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels[constants.MD5AgentDeploymentProviderLabelKey] = provider
	}

	return r.createOrUpdateDeployment(deploymentLogger, dda, deployment, newStatus, updateStatusV2WithClusterAgent)
}

func updateStatusV2WithClusterAgent(dca *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterAgent = condition.UpdateDeploymentStatus(dca, newStatus.ClusterAgent, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.ClusterAgentReconcileConditionType, status, reason, message, true)
}

func deleteStatusV2WithClusterAgent(newStatus *datadoghqv2alpha1.DatadogAgentStatus) {
	newStatus.ClusterAgent = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, common.ClusterAgentReconcileConditionType)
}

func (r *Reconciler) cleanupV2ClusterAgent(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, deployment *appsv1.Deployment, resourcesManager feature.ResourceManagers, newStatus *datadoghqv2alpha1.DatadogAgentStatus) (reconcile.Result, error) {
	nsName := types.NamespacedName{
		Name:      deployment.GetName(),
		Namespace: deployment.GetNamespace(),
	}

	// Existing deployment attached to this instance
	existingDeployment := &appsv1.Deployment{}
	if err := r.client.Get(ctx, nsName, existingDeployment); err != nil {
		if errors.IsNotFound(err) {
			return reconcile.Result{}, nil
		}
		return reconcile.Result{}, err
	}
	logger.Info("Deleting Deployment", "deployment.Namespace", existingDeployment.Namespace, "deployment.Name", existingDeployment.Name)
	event := buildEventInfo(existingDeployment.Name, existingDeployment.Namespace, kubernetes.DeploymentKind, datadog.DeletionEvent)
	r.recordEvent(dda, event)
	if err := r.client.Delete(ctx, existingDeployment); err != nil {
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

func (r *Reconciler) setClusterAgentStatus(status *datadoghqv2alpha1.DatadogAgentStatus, deploymentStatus *datadoghqv2alpha1.DeploymentStatus) {
	status.ClusterAgent = deploymentStatus
}
