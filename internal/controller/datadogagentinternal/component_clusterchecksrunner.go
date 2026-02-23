// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagentinternal

import (
	"context"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentccr "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/pkg/condition"
)

// ClusterChecksRunnerComponent implements ComponentReconciler for the Cluster Checks Runner deployment
type ClusterChecksRunnerComponent struct {
	reconciler *Reconciler
}

// NewClusterChecksRunnerComponent creates a new ClusterChecksRunner component
func NewClusterChecksRunnerComponent(reconciler *Reconciler) *ClusterChecksRunnerComponent {
	return &ClusterChecksRunnerComponent{
		reconciler: reconciler,
	}
}

// Name returns the component name
func (c *ClusterChecksRunnerComponent) Name() datadoghqv2alpha1.ComponentName {
	return datadoghqv2alpha1.ClusterChecksRunnerComponentName
}

// IsEnabled checks if the ClusterChecksRunner component should be reconciled
// CCR requires the Cluster Agent to be enabled as well
func (c *ClusterChecksRunnerComponent) IsEnabled(requiredComponents feature.RequiredComponents, overrides map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride) (enabled bool, conflict bool) {
	return checkComponentEnabledWithOverride(c.Name(), requiredComponents.ClusterChecksRunner.IsEnabled(), overrides)
}

// GetConditionType returns the condition type for status updates
func (c *ClusterChecksRunnerComponent) GetConditionType() string {
	return common.ClusterChecksRunnerReconcileConditionType
}

func (c *ClusterChecksRunnerComponent) GetGlobalSettingsFunc() func(logger logr.Logger, podManagers feature.PodTemplateManagers, ddai metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec, resourceManagers feature.ResourceManagers, requiredComponents feature.RequiredComponents) {
	return global.ApplyGlobalSettingsClusterChecksRunner
}

func (c *ClusterChecksRunnerComponent) GetNewDeploymentFunc() func(ddai metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment {
	return componentccr.NewDefaultClusterChecksRunnerDeployment
}

func (c *ClusterChecksRunnerComponent) GetManageFeatureFunc() func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
	return func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
		return feat.ManageClusterChecksRunner(managers, provider)
	}
}

func (c *ClusterChecksRunnerComponent) UpdateStatus(deployment *appsv1.Deployment, newStatus *v1alpha1.DatadogAgentInternalStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.ClusterChecksRunner = condition.UpdateDeploymentStatus(deployment, newStatus.ClusterChecksRunner, &updateTime)
	condition.UpdateDatadogAgentInternalStatusConditions(newStatus, updateTime, common.ClusterChecksRunnerReconcileConditionType, status, reason, message, true)
}

func (c *ClusterChecksRunnerComponent) DeleteStatus(newStatus *v1alpha1.DatadogAgentInternalStatus, conditionType string) {
	newStatus.ClusterChecksRunner = nil
	condition.DeleteDatadogAgentInternalStatusCondition(newStatus, conditionType)
}

func (c *ClusterChecksRunnerComponent) ForceDeleteComponent(ddai *v1alpha1.DatadogAgentInternal, requiredComponents feature.RequiredComponents) bool {
	// CCR requires the Cluster Agent to be enabled
	dcaEnabled := requiredComponents.ClusterAgent.IsEnabled()

	// If the Cluster Agent is disabled via override, then CCR should be disabled too
	if dcaOverride, ok := ddai.Spec.Override[datadoghqv2alpha1.ClusterAgentComponentName]; ok {
		if apiutils.BoolValue(dcaOverride.Disabled) {
			return true
		}
	} else if !dcaEnabled {
		return true
	}
	return false
}

func (c *ClusterChecksRunnerComponent) CleanupDependencies(ctx context.Context, logger logr.Logger, ddai *v1alpha1.DatadogAgentInternal, resourcesManager feature.ResourceManagers) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
