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
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
)

// ClusterChecksRunnerComponent implements ComponentReconciler for the Cluster Checks Runner deployment
type ClusterChecksRunnerComponent struct {
	BaseComponent
	reconciler *Reconciler
}

// NewClusterChecksRunnerComponent creates a new ClusterChecksRunner component
func NewClusterChecksRunnerComponent(reconciler *Reconciler) *ClusterChecksRunnerComponent {
	base := NewComponentBuilder(datadoghqv2alpha1.ClusterChecksRunnerComponentName).
		WithConditionType(common.ClusterChecksRunnerReconcileConditionType).
		WithGlobalSettings(global.ApplyGlobalSettingsClusterChecksRunner).
		WithNewDeployment(componentccr.NewDefaultClusterChecksRunnerDeployment).
		WithManageFeature(func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
			return feat.ManageClusterChecksRunner(managers, provider)
		}).
		WithStatusField(func(status *datadoghqv2alpha1.DatadogAgentStatus) **datadoghqv2alpha1.DeploymentStatus {
			return &status.ClusterChecksRunner
		}).
		Build()

	return &ClusterChecksRunnerComponent{
		BaseComponent: base,
		reconciler:    reconciler,
	}
}

// IsEnabled checks if the ClusterChecksRunner component should be reconciled
// CCR requires the Cluster Agent to be enabled as well
func (c *ClusterChecksRunnerComponent) IsEnabled(requiredComponents feature.RequiredComponents, overrides map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride) (enabled bool, conflict bool) {
	return checkComponentEnabledWithOverride(c.Name(), requiredComponents.ClusterChecksRunner.IsEnabled(), overrides)
}

func (c *ClusterChecksRunnerComponent) ForceDeleteComponent(dda *datadoghqv2alpha1.DatadogAgent, componentName datadoghqv2alpha1.ComponentName, requiredComponents feature.RequiredComponents) bool {
	dcaEnabled := requiredComponents.ClusterAgent.IsEnabled()
	// If the Cluster Agent is disabled, then CCR should be disabled too
	if dcaOverride, ok := dda.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
		if apiutils.BoolValue(dcaOverride.Disabled) {
			return true
		}
	} else if !dcaEnabled {
		return true
	}
	return false
}

func (c *ClusterChecksRunnerComponent) CleanupDependencies(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
