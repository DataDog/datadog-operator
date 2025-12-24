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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
)

// ClusterAgentComponent implements ComponentReconciler for the Cluster Agent deployment
type ClusterAgentComponent struct {
	reconciler *Reconciler
}

// NewClusterAgentComponent creates a new ClusterAgent component
func NewClusterAgentComponent(reconciler *Reconciler) *ClusterAgentComponent {
	return &ClusterAgentComponent{
		reconciler: reconciler,
	}
}

// Name returns the component name
func (c *ClusterAgentComponent) Name() datadoghqv2alpha1.ComponentName {
	return datadoghqv2alpha1.ClusterAgentComponentName
}

// IsEnabled checks if the Cluster Agent component should be reconciled
func (c *ClusterAgentComponent) IsEnabled(requiredComponents feature.RequiredComponents) bool {
	return requiredComponents.ClusterAgent.IsEnabled()
}

// GetConditionType returns the condition type for status updates
func (c *ClusterAgentComponent) GetConditionType() string {
	return common.ClusterAgentReconcileConditionType
}

func (c *ClusterAgentComponent) GetGlobalSettingsFunc() func(logger logr.Logger, podManagers feature.PodTemplateManagers, dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec, resourceManagers feature.ResourceManagers, requiredComponents feature.RequiredComponents) {
	return global.ApplyGlobalSettingsClusterAgent
}

func (c *ClusterAgentComponent) GetNewDeploymentFunc() func(dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment {
	return componentdca.NewDefaultClusterAgentDeployment
}

func (c *ClusterAgentComponent) GetManageFeatureFunc() func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
	return func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
		return feat.ManageClusterAgent(managers, provider)
	}
}

// Reconcile reconciles component
func (c *ClusterAgentComponent) Reconcile(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	var result reconcile.Result
	now := metav1.NewTime(time.Now())
	deploymentLogger := params.Logger.WithValues("component", c.Name())

	// Start by creating the Default Cluster-Agent deployment
	deployment := c.GetNewDeploymentFunc()(params.DDA.GetObjectMeta(), &params.DDA.Spec)
	podManagers := feature.NewPodTemplateManagers(&deployment.Spec.Template)

	// Set Global setting on the default deployment
	c.GetGlobalSettingsFunc()(params.Logger, podManagers, params.DDA.GetObjectMeta(), &params.DDA.Spec, params.ResourceManagers, params.RequiredComponents)

	// Apply features changes on the Deployment.Spec.Template
	var featErrors []error
	for _, feat := range params.Features {
		if errFeat := c.GetManageFeatureFunc()(feat, podManagers, params.Provider); errFeat != nil {
			featErrors = append(featErrors, errFeat)
		}
	}
	if len(featErrors) > 0 {
		err := utilerrors.NewAggregate(featErrors)
		c.UpdateStatus(deployment, params.Status, now, metav1.ConditionFalse, fmt.Sprintf("%s feature error", c.Name()), err.Error())
		return result, err
	}

	// The requiredComponents can change depending on if updates to features result in disabled components
	componentEnabled := c.IsEnabled(params.RequiredComponents)

	if c.ForceDeleteComponent(params.DDA, c.Name(), params.RequiredComponents) {
		return c.Cleanup(ctx, params)
	}

	// If Override is defined for the component, apply the override on the PodTemplateSpec, it will cascade to container.
	if componentOverride, ok := params.DDA.Spec.Override[c.Name()]; ok {
		if apiutils.BoolValue(componentOverride.Disabled) {
			if componentEnabled {
				// The override supersedes what's set in requiredComponents; update status to reflect the conflict
				condition.UpdateDatadogAgentStatusConditions(
					params.Status,
					metav1.NewTime(time.Now()),
					common.OverrideReconcileConflictConditionType,
					metav1.ConditionTrue,
					"OverrideConflict",
					fmt.Sprintf("%s component is set to disabled", c.Name()),
					true,
				)
			}
			return c.Cleanup(ctx, params)
		}
		override.PodTemplateSpec(params.Logger, podManagers, componentOverride, c.Name(), params.DDA.Name)
		override.Deployment(deployment, componentOverride)
	} else if !componentEnabled {
		return c.Cleanup(ctx, params)
	}

	if c.reconciler.options.IntrospectionEnabled {
		// Add provider label to deployment
		if deployment.Labels == nil {
			deployment.Labels = make(map[string]string)
		}
		deployment.Labels[constants.MD5AgentDeploymentProviderLabelKey] = params.Provider
	}

	return c.reconciler.createOrUpdateDeployment(deploymentLogger, params.DDA, deployment, params.Status, c.UpdateStatus)
}

// Cleanup removes the component deployment, associated resources and updates status
func (c *ClusterAgentComponent) Cleanup(ctx context.Context, params *ReconcileComponentParams) (reconcile.Result, error) {
	deployment := c.GetNewDeploymentFunc()(params.DDA.GetObjectMeta(), &params.DDA.Spec)
	result, err := c.reconciler.deleteDeploymentWithEvent(ctx, params.Logger, params.DDA, deployment)

	if err != nil {
		return result, err
	}

	// Do status and other resource cleanup if the deployment was deleted successfully
	if result, err := c.CleanupDependencies(ctx, params.Logger, params.DDA, params.ResourceManagers); err != nil {
		return result, err
	}
	c.DeleteStatus(params.Status, c.GetConditionType())

	return result, nil
}

func (c *ClusterAgentComponent) UpdateStatus(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterAgent = condition.UpdateDeploymentStatus(deployment, newStatus.ClusterAgent, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.ClusterAgentReconcileConditionType, status, reason, message, true)
}

func (c *ClusterAgentComponent) DeleteStatus(newStatus *datadoghqv2alpha1.DatadogAgentStatus, conditionType string) {
	newStatus.ClusterAgent = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, conditionType)
}

func (c *ClusterAgentComponent) ForceDeleteComponent(dda *datadoghqv2alpha1.DatadogAgent, componentName datadoghqv2alpha1.ComponentName, requiredComponents feature.RequiredComponents) bool {
	return false
}
func (c *ClusterAgentComponent) CleanupDependencies(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) (reconcile.Result, error) {
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
	return reconcile.Result{}, nil
}
