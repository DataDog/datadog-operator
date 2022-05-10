// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
)

func convertClusterAgentSpec(src *DatadogAgentSpecClusterAgentSpec, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	// src.Enabled dropped as not present in v2

	if src.Image != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Image = src.Image
	}

	if src.DeploymentName != "" {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Name = &src.DeploymentName
	}

	if src.Config != nil {
		if src.Config.SecurityContext != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).SecurityContext = src.Config.SecurityContext
		}

		convertClusterAgentExternalMetrics(src.Config.ExternalMetrics, dst)
		convertClusterAgentAdmissionController(src.Config.AdmissionController, dst)

		if src.Config.ClusterChecksEnabled != nil {
			features := getV2Features(dst)
			if features.ClusterChecks == nil {
				features.ClusterChecks = &v2alpha1.ClusterChecksFeatureConfig{}
			}

			features.ClusterChecks.Enabled = src.Config.ClusterChecksEnabled
		}

		if src.Config.CollectEvents != nil {
			features := getV2Features(dst)
			if features.EventCollection == nil {
				features.EventCollection = &v2alpha1.EventCollectionFeatureConfig{}
			}

			setBooleanPtrOR(src.Config.CollectEvents, &features.EventCollection.CollectKubernetesEvents)
		}

		if src.Config.LogLevel != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName), commonv1.ClusterAgentContainerName).LogLevel = src.Config.LogLevel
		}

		if src.Config.Resources != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName), commonv1.ClusterAgentContainerName).Resources = src.Config.Resources
		}

		if src.Config.Command != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName), commonv1.ClusterAgentContainerName).Command = src.Config.Command
		}

		if src.Config.Args != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName), commonv1.ClusterAgentContainerName).Args = src.Config.Args
		}

		if src.Config.Confd != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).ExtraConfd = convertConfigDirSpec(src.Config.Confd)
		}

		if src.Config.Env != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName), commonv1.ClusterAgentContainerName).Env = src.Config.Env
		}

		if src.Config.VolumeMounts != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName), commonv1.ClusterAgentContainerName).VolumeMounts = src.Config.VolumeMounts
		}

		if src.Config.Volumes != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Volumes = src.Config.Volumes
		}

		if src.Config.HealthPort != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName), commonv1.ClusterAgentContainerName).HealthPort = src.Config.HealthPort
		}
	}

	if src.CustomConfig != nil {
		tmpl := getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName)
		if tmpl.CustomConfigurations == nil {
			tmpl.CustomConfigurations = make(map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig)
		}

		tmpl.CustomConfigurations[v2alpha1.AgentGeneralConfigFile] = *convertConfigMapConfig(src.CustomConfig)
	}

	if src.Rbac != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).CreateRbac = src.Rbac.Create
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).ServiceAccountName = src.Rbac.ServiceAccountName
	}

	if src.Replicas != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Replicas = src.Replicas
	}

	if src.AdditionalAnnotations != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Annotations = src.AdditionalAnnotations
	}

	if src.AdditionalLabels != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Labels = src.AdditionalLabels
	}

	if src.PriorityClassName != "" {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).PriorityClassName = &src.PriorityClassName
	}

	if src.Affinity != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Affinity = src.Affinity
	}

	if src.Tolerations != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).Tolerations = src.Tolerations
	}

	if src.NodeSelector != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterAgentComponentName).NodeSelector = src.NodeSelector
	}

	// TODO: NetworkPolicy field for DCA? In v2 we only have a single global NetworkPolicy configuration
}

func convertClusterAgentExternalMetrics(src *ExternalMetricsConfig, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	features := getV2Features(dst)
	if features.ExternalMetricsServer == nil {
		features.ExternalMetricsServer = &v2alpha1.ExternalMetricsServerFeatureConfig{}
	}

	features.ExternalMetricsServer.Enabled = src.Enabled
	features.ExternalMetricsServer.Port = src.Port

	// Only copy if not default
	if src.WpaController {
		features.ExternalMetricsServer.WPAController = &src.WpaController
	}
	if !src.UseDatadogMetrics {
		features.ExternalMetricsServer.UseDatadogMetrics = &src.UseDatadogMetrics
	}

	if src.Endpoint != nil {
		features.ExternalMetricsServer.Endpoint = &v2alpha1.Endpoint{
			URL: src.Endpoint,
		}
	}

	if src.Credentials != nil {
		if features.ExternalMetricsServer.Endpoint == nil {
			features.ExternalMetricsServer.Endpoint = &v2alpha1.Endpoint{}
		}

		features.ExternalMetricsServer.Endpoint.Credentials = convertCredentials(src.Credentials)
	}
}

func convertClusterAgentAdmissionController(src *AdmissionControllerConfig, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	features := getV2Features(dst)
	if features.AdmissionController == nil {
		features.AdmissionController = &v2alpha1.AdmissionControllerFeatureConfig{}
	}

	features.AdmissionController.Enabled = src.Enabled
	features.AdmissionController.MutateUnlabelled = src.MutateUnlabelled
	features.AdmissionController.ServiceName = src.ServiceName
	features.AdmissionController.AgentCommunicationMode = src.AgentCommunicationMode
}
