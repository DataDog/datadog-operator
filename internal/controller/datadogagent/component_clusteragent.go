// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	"context"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
)

// ClusterAgentComponent implements ComponentReconciler for the Cluster Agent deployment
type ClusterAgentComponent struct {
	BaseComponent
	reconciler *Reconciler
}

// NewClusterAgentComponent creates a new ClusterAgent component
func NewClusterAgentComponent(reconciler *Reconciler) *ClusterAgentComponent {
	base := NewComponentBuilder(datadoghqv2alpha1.ClusterAgentComponentName).
		WithConditionType(common.ClusterAgentReconcileConditionType).
		WithGlobalSettings(global.ApplyGlobalSettingsClusterAgent).
		WithNewDeployment(componentdca.NewDefaultClusterAgentDeployment).
		WithManageFeature(func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
			return feat.ManageClusterAgent(managers, provider)
		}).
		WithStatusField(func(status *datadoghqv2alpha1.DatadogAgentStatus) **datadoghqv2alpha1.DeploymentStatus {
			return &status.ClusterAgent
		}).
		Build()

	return &ClusterAgentComponent{
		BaseComponent: base,
		reconciler:    reconciler,
	}
}

// IsEnabled checks if the Cluster Agent component should be reconciled
func (c *ClusterAgentComponent) IsEnabled(requiredComponents feature.RequiredComponents, overrides map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride) (enabled bool, conflict bool) {
	return checkComponentEnabledWithOverride(c.Name(), requiredComponents.ClusterAgent.IsEnabled(), overrides)
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
