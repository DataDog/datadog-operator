// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/pkg/condition"
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
