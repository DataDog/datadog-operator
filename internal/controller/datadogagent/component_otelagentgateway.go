// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogagent

import (
	datadoghqv2alpha1 "github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentotelagentgateway "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/otelagentgateway"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/global"
)

// OtelAgentGatewayComponent implements ComponentReconciler for the OTel Agent Gateway deployment
type OtelAgentGatewayComponent struct {
	BaseComponent
	reconciler *Reconciler
}

// NewOtelAgentGatewayComponent creates a new OtelAgentGateway component
func NewOtelAgentGatewayComponent(reconciler *Reconciler) *OtelAgentGatewayComponent {
	base := NewComponentBuilder(datadoghqv2alpha1.OtelAgentGatewayComponentName).
		WithConditionType(common.OtelAgentGatewayReconcileConditionType).
		WithGlobalSettings(global.ApplyGlobalSettingsOtelAgentGateway).
		WithNewDeployment(componentotelagentgateway.NewDefaultOtelAgentGatewayDeployment).
		WithManageFeature(func(feat feature.Feature, managers feature.PodTemplateManagers, provider string) error {
			return feat.ManageOtelAgentGateway(managers, provider)
		}).
		WithStatusField(func(status *datadoghqv2alpha1.DatadogAgentStatus) **datadoghqv2alpha1.DeploymentStatus {
			return &status.OtelAgentGateway
		}).
		Build()

	return &OtelAgentGatewayComponent{
		BaseComponent: base,
		reconciler:    reconciler,
	}
}

// IsEnabled checks if the OtelAgentGateway component should be reconciled
func (c *OtelAgentGatewayComponent) IsEnabled(requiredComponents feature.RequiredComponents, overrides map[datadoghqv2alpha1.ComponentName]*datadoghqv2alpha1.DatadogAgentComponentOverride) (enabled bool, conflict bool) {
	return checkComponentEnabledWithOverride(c.Name(), requiredComponents.OtelAgentGateway.IsEnabled(), overrides)
}
