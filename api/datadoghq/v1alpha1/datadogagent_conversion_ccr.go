// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	commonv1 "github.com/DataDog/datadog-operator/api/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
)

func convertCCRSpec(src *DatadogAgentSpecClusterChecksRunnerSpec, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	if src.Enabled != nil {
		features := getV2Features(dst)
		if features.ClusterChecks == nil {
			features.ClusterChecks = &v2alpha1.ClusterChecksFeatureConfig{}
		}

		features.ClusterChecks.UseClusterChecksRunners = src.Enabled
	}

	if src.Image != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Image = src.Image
	}

	if src.DeploymentName != "" {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Name = &src.DeploymentName
	}

	if src.Config != nil {
		if src.Config.LogLevel != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName), commonv1.ClusterChecksRunnersContainerName).LogLevel = src.Config.LogLevel
		}

		if src.Config.SecurityContext != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).SecurityContext = src.Config.SecurityContext
		}

		if src.Config.Resources != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName), commonv1.ClusterChecksRunnersContainerName).Resources = src.Config.Resources
		}

		if src.Config.Command != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName), commonv1.ClusterChecksRunnersContainerName).Command = src.Config.Command
		}

		if src.Config.Args != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName), commonv1.ClusterChecksRunnersContainerName).Args = src.Config.Args
		}

		if src.Config.Env != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName), commonv1.ClusterChecksRunnersContainerName).Env = src.Config.Env
		}

		if src.Config.VolumeMounts != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName), commonv1.ClusterChecksRunnersContainerName).VolumeMounts = src.Config.VolumeMounts
		}

		if src.Config.Volumes != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Volumes = src.Config.Volumes
		}

		if src.Config.HealthPort != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName), commonv1.ClusterChecksRunnersContainerName).HealthPort = src.Config.HealthPort
		}
	}

	if src.CustomConfig != nil {
		tmpl := getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName)
		if tmpl.CustomConfigurations == nil {
			tmpl.CustomConfigurations = make(map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig)
		}

		tmpl.CustomConfigurations[v2alpha1.AgentGeneralConfigFile] = *convertConfigMapConfig(src.CustomConfig)
	}

	if src.Rbac != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).CreateRbac = src.Rbac.Create
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).ServiceAccountName = src.Rbac.ServiceAccountName
	}

	if src.Replicas != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Replicas = src.Replicas
	}

	if src.AdditionalAnnotations != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Annotations = src.AdditionalAnnotations
	}

	if src.AdditionalLabels != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Labels = src.AdditionalLabels
	}

	if src.PriorityClassName != "" {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).PriorityClassName = &src.PriorityClassName
	}

	if src.Affinity != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Affinity = src.Affinity
	}

	if src.Tolerations != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).Tolerations = src.Tolerations
	}

	if src.NodeSelector != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.ClusterChecksRunnerComponentName).NodeSelector = src.NodeSelector
	}

	// TODO: NetworkPolicy field for CLC? In v2 we only have a single global NetworkPolicy configuration
}
