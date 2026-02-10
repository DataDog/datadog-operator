// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
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
	// Enable the OTel collector
	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_OTELCOLLECTOR_ENABLED",
		Value: "true",
	})

	// Enable gateway mode
	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_OTELCOLLECTOR_GATEWAY_MODE",
		Value: "true",
	})

	// Set hostname from spec.nodeName
	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name: "DD_HOSTNAME",
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{
				FieldPath: "spec.nodeName",
			},
		},
	})

	// Set converter features (exclude infra attribute, prometheus and DD flare)
	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_OTELCOLLECTOR_CONVERTER_FEATURES",
		Value: "health_check,zpages,pprof,datadog",
	})

	// Disable features that are not needed / won't work in standalone ddot-collector
	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_ENABLE_METADATA_COLLECTION",
		Value: "false",
	})

	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_PROCESS_AGENT_ENABLED",
		Value: "false",
	})

	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_PROCESS_CONFIG_RUN_IN_CORE_AGENT_ENABLED",
		Value: "false",
	})

	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_REMOTE_CONFIGURATION_ENABLED",
		Value: "false",
	})

	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_INVENTORIES_ENABLED",
		Value: "false",
	})

	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_CMD_PORT",
		Value: "0",
	})

	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_AGENT_IPC_PORT",
		Value: "0",
	})

	manager.EnvVar().AddEnvVarToContainer(apicommon.OtelAgent, &corev1.EnvVar{
		Name:  "DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL",
		Value: "0",
	})
}
