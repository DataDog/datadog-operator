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
	componentotelagentgateway "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/otelagentgateway"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
	"github.com/DataDog/datadog-operator/pkg/condition"
)

// OtelAgentGatewayComponent implements ComponentReconciler for the OTel Agent Gateway deployment
type OtelAgentGatewayComponent struct {
	reconciler *Reconciler
}

// NewOtelAgentGatewayComponent creates a new OtelAgentGateway component
func NewOtelAgentGatewayComponent(reconciler *Reconciler) *OtelAgentGatewayComponent {
	return &OtelAgentGatewayComponent{
		reconciler: reconciler,
	}
}

// Name returns the component name
func (c *OtelAgentGatewayComponent) Name() datadoghqv2alpha1.ComponentName {
	return datadoghqv2alpha1.OtelAgentGatewayComponentName
}

// IsEnabled checks if the OtelAgentGateway component should be reconciled
func (c *OtelAgentGatewayComponent) IsEnabled(requiredComponents feature.RequiredComponents, overrides map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride) (enabled bool, conflict bool) {
	return checkComponentEnabledWithOverride(c.Name(), requiredComponents.OtelAgentGateway.IsEnabled(), overrides)
}

// GetConditionType returns the condition type for status updates
func (c *OtelAgentGatewayComponent) GetConditionType() string {
	return common.OtelAgentGatewayReconcileConditionType
}

func (c *OtelAgentGatewayComponent) GetGlobalSettingsFunc() func(logger logr.Logger, podManagers feature.PodTemplateManagers, dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec, resourceManagers feature.ResourceManagers, requiredComponents feature.RequiredComponents) {
	return global.ApplyGlobalSettingsOtelAgentGateway
}

func (c *OtelAgentGatewayComponent) GetNewDeploymentFunc() func(dda metav1.Object, spec *datadoghqv2alpha1.DatadogAgentSpec) *appsv1.Deployment {
	return componentotelagentgateway.NewDefaultOtelAgentGatewayDeployment
}

func (c *OtelAgentGatewayComponent) GetManageFeatureFunc() func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
	return func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
		return feat.ManageOtelAgentGateway(managers, provider)
	}
}

func (c *OtelAgentGatewayComponent) UpdateStatus(deployment *appsv1.Deployment, newStatus *datadoghqv2alpha1.DatadogAgentStatus, updateTime metav1.Time, status metav1.ConditionStatus, reason, message string) {
	newStatus.OtelAgentGateway = condition.UpdateDeploymentStatus(deployment, newStatus.OtelAgentGateway, &updateTime)
	condition.UpdateDatadogAgentStatusConditions(newStatus, updateTime, common.OtelAgentGatewayReconcileConditionType, status, reason, message, true)
}

func (c *OtelAgentGatewayComponent) DeleteStatus(newStatus *datadoghqv2alpha1.DatadogAgentStatus, conditionType string) {
	newStatus.OtelAgentGateway = nil
	condition.DeleteDatadogAgentStatusCondition(newStatus, conditionType)
}

func (c *OtelAgentGatewayComponent) ForceDeleteComponent(dda *datadoghqv2alpha1.DatadogAgent, componentName datadoghqv2alpha1.ComponentName, requiredComponents feature.RequiredComponents) bool {
	return false
}

func (c *OtelAgentGatewayComponent) CleanupDependencies(ctx context.Context, logger logr.Logger, dda *datadoghqv2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) (reconcile.Result, error) {
	return reconcile.Result{}, nil
}
