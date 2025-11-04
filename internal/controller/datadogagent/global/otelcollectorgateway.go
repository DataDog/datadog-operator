// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

// ApplyGlobalSettingsOtelAgentGateway applies global settings to the OtelAgentGateway deployment
func ApplyGlobalSettingsOtelAgentGateway(
	logger logr.Logger,
	manager feature.PodTemplateManagers,
	ddaMeta metav1.Object,
	ddaSpec *v2alpha1.DatadogAgentSpec,
	resourcesManager feature.ResourceManagers,
	requiredComponents feature.RequiredComponents,
) {
	applyGlobalSettings(logger, manager, ddaMeta, ddaSpec, resourcesManager, requiredComponents)
	applyOtelAgentGatewayResources(manager, ddaSpec)
}

func applyOtelAgentGatewayResources(manager feature.PodTemplateManagers, ddaSpec *v2alpha1.DatadogAgentSpec) {
	// Add any OtelAgentGateway-specific resource configuration here
}
