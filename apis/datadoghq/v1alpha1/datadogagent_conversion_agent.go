// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package v1alpha1

import (
	"strings"

	corev1 "k8s.io/api/core/v1"

	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/apis/utils"
)

func convertDatadogAgentSpec(src *DatadogAgentSpecAgentSpec, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	// TODO: Enable/Disable Agent DaemonSet
	// TODO: ExtendedDaemonSet support

	if src.Image != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Image = src.Image
	}

	if src.DaemonsetName != "" {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Name = &src.DaemonsetName
	}

	if src.Config != nil {
		if src.Config.SecurityContext != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).SecurityContext = src.Config.SecurityContext
		}

		if src.Config.DDUrl != nil {
			getV2GlobalConfig(dst).Endpoint = &v2alpha1.Endpoint{
				URL: src.Config.DDUrl,
			}
		}

		// src.Config.LogLevel not forwarded as setting is not at the same level (NodeAgent POD in v1, Container in v2)

		if src.Config.Confd != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).ExtraConfd = ConvertConfigDirSpec(src.Config.Confd)
		}

		if src.Config.Checksd != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).ExtraChecksd = ConvertConfigDirSpec(src.Config.Checksd)
		}

		if src.Config.HostPort != nil {
			features := getV2Features(dst)
			if features.Dogstatsd == nil {
				features.Dogstatsd = &v2alpha1.DogstatsdFeatureConfig{}
			}
			if features.Dogstatsd.HostPortConfig == nil {
				features.Dogstatsd.HostPortConfig = &v2alpha1.HostPortConfig{}
			}
			features.Dogstatsd.HostPortConfig.Enabled = utils.NewBoolPointer(true)
			features.Dogstatsd.HostPortConfig.Port = src.Config.HostPort
		}

		if src.Config.PodLabelsAsTags != nil {
			getV2GlobalConfig(dst).PodLabelsAsTags = src.Config.PodLabelsAsTags
		}

		if src.Config.PodAnnotationsAsTags != nil {
			getV2GlobalConfig(dst).PodAnnotationsAsTags = src.Config.PodAnnotationsAsTags
		}

		if src.Config.NodeLabelsAsTags != nil {
			getV2GlobalConfig(dst).NodeLabelsAsTags = src.Config.NodeLabelsAsTags
		}

		if src.Config.NamespaceLabelsAsTags != nil {
			getV2GlobalConfig(dst).NamespaceLabelsAsTags = src.Config.NamespaceLabelsAsTags
		}

		if src.Config.NamespaceAnnotationsAsTags != nil {
			getV2GlobalConfig(dst).NamespaceAnnotationsAsTags = src.Config.NamespaceAnnotationsAsTags
		}

		if src.Config.Tags != nil {
			getV2GlobalConfig(dst).Tags = append(getV2GlobalConfig(dst).Tags, src.Config.Tags...)
		}

		if src.Config.CollectEvents != nil {
			features := getV2Features(dst)
			if features.EventCollection == nil {
				features.EventCollection = &v2alpha1.EventCollectionFeatureConfig{}
			}

			features.EventCollection.CollectKubernetesEvents = src.Config.CollectEvents
		}

		if src.Config.Env != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).Env = src.Config.Env
		}

		if src.Config.Volumes != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Volumes = src.Config.Volumes
		}

		if src.Config.VolumeMounts != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).VolumeMounts = src.Config.VolumeMounts
		}

		if src.Config.Resources != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).Resources = src.Config.Resources
		}

		if src.Config.Command != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).Command = src.Config.Command
		}

		if src.Config.Args != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).Args = src.Config.Args
		}

		if src.Config.LivenessProbe != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).LivenessProbe = src.Config.LivenessProbe
		}

		if src.Config.ReadinessProbe != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).ReadinessProbe = src.Config.ReadinessProbe
		}

		if src.Config.HealthPort != nil {
			getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.CoreAgentContainerName).HealthPort = src.Config.HealthPort
		}

		if src.Config.CriSocket != nil {
			getV2GlobalConfig(dst).CriSocketPath = src.Config.CriSocket.CriSocketPath
			getV2GlobalConfig(dst).DockerSocketPath = src.Config.CriSocket.DockerSocketPath
		}

		if src.Config.Dogstatsd != nil {
			features := getV2Features(dst)
			if features.Dogstatsd == nil {
				features.Dogstatsd = &v2alpha1.DogstatsdFeatureConfig{}
			}

			features.Dogstatsd.OriginDetectionEnabled = src.Config.Dogstatsd.DogstatsdOriginDetection
			features.Dogstatsd.MapperProfiles = convertConfigMapConfig(src.Config.Dogstatsd.MapperProfiles)

			if src.Config.Dogstatsd.UnixDomainSocket != nil {
				features.Dogstatsd.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{
					Enabled: src.Config.Dogstatsd.UnixDomainSocket.Enabled,
					Path:    src.Config.Dogstatsd.UnixDomainSocket.HostFilepath,
				}
			}
		}

		if src.Config.Tolerations != nil {
			getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Tolerations = src.Config.Tolerations
		}

		if src.Config.Kubelet != nil {
			getV2GlobalConfig(dst).Kubelet = src.Config.Kubelet
		}
	}

	if src.Rbac != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).CreateRbac = src.Rbac.Create
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).ServiceAccountName = src.Rbac.ServiceAccountName
	}

	if src.AdditionalAnnotations != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Annotations = src.AdditionalAnnotations
	}

	if src.AdditionalLabels != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Labels = src.AdditionalLabels
	}

	// DNSPolicy + DNSConfig missing, is it required?

	if src.Env != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Env = src.Env
	}

	if src.HostNetwork {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).HostNetwork = &src.HostNetwork
	}

	if src.HostPID {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).HostPID = &src.HostPID
	}

	if src.CustomConfig != nil {
		tmpl := getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName)
		if tmpl.CustomConfigurations == nil {
			tmpl.CustomConfigurations = make(map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig)
		}

		tmpl.CustomConfigurations[v2alpha1.AgentGeneralConfigFile] = *convertConfigMapConfig(src.CustomConfig)
	}

	if src.NetworkPolicy != nil {
		getV2GlobalConfig(dst).NetworkPolicy = &v2alpha1.NetworkPolicyConfig{
			Create:               src.NetworkPolicy.Create,
			Flavor:               v2alpha1.NetworkPolicyFlavor(src.NetworkPolicy.Flavor),
			DNSSelectorEndpoints: src.NetworkPolicy.DNSSelectorEndpoints,
		}
	}

	if src.Affinity != nil {
		getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName).Affinity = src.Affinity
	}

	if src.LocalService != nil {
		localService := &v2alpha1.LocalService{
			ForceEnableLocalService: src.LocalService.ForceLocalServiceEnable,
		}

		if src.LocalService.OverrideName != "" {
			localService.NameOverride = &src.LocalService.OverrideName
		}

		getV2GlobalConfig(dst).LocalService = localService
	}

	convertAPMSpec(src.Apm, dst)
	convertLogCollection(src.Log, dst)
	convertProcessSpec(src.Process, dst)
	convertSystemProbeSpec(src.SystemProbe, dst)
	convertSecurityAgentSpec(src.Security, dst)
}

func convertAPMSpec(src *APMSpec, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	features := getV2Features(dst)
	if features.APM == nil {
		features.APM = &v2alpha1.APMFeatureConfig{}
	}

	if src.Enabled != nil {
		features.APM.Enabled = src.Enabled
	}

	if src.UnixDomainSocket != nil {
		features.APM.UnixDomainSocketConfig = &v2alpha1.UnixDomainSocketConfig{
			Enabled: src.UnixDomainSocket.Enabled,
			Path:    src.UnixDomainSocket.HostFilepath,
		}
	}

	if src.HostPort != nil {
		features.APM.HostPortConfig = &v2alpha1.HostPortConfig{
			Enabled: utils.NewBoolPointer(true),
			Port:    src.HostPort,
		}
	}

	if src.Env != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.TraceAgentContainerName).Env = src.Env
	}

	if src.VolumeMounts != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.TraceAgentContainerName).VolumeMounts = src.VolumeMounts
	}

	if src.Resources != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.TraceAgentContainerName).Resources = src.Resources
	}

	if src.Command != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.TraceAgentContainerName).Command = src.Command
	}

	if src.Args != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.TraceAgentContainerName).Args = src.Args
	}

	if src.LivenessProbe != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.TraceAgentContainerName).LivenessProbe = src.LivenessProbe
	}
}

func convertLogCollection(src *LogCollectionConfig, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	features := getV2Features(dst)
	if features.LogCollection == nil {
		features.LogCollection = &v2alpha1.LogCollectionFeatureConfig{}
	}

	setBooleanPtrOR(src.Enabled, &features.LogCollection.Enabled)
	setBooleanPtrOR(src.LogsConfigContainerCollectAll, &features.LogCollection.ContainerCollectAll)
	setBooleanPtrOR(src.ContainerCollectUsingFiles, &features.LogCollection.ContainerCollectUsingFiles)

	// Cannot resolve if both set, consider features has priority
	if src.ContainerLogsPath != nil && features.LogCollection.ContainerLogsPath == nil {
		features.LogCollection.ContainerLogsPath = src.ContainerLogsPath
	}
	if src.PodLogsPath != nil && features.LogCollection.PodLogsPath == nil {
		features.LogCollection.PodLogsPath = src.PodLogsPath
	}
	if src.ContainerSymlinksPath != nil && features.LogCollection.ContainerSymlinksPath == nil {
		features.LogCollection.ContainerSymlinksPath = src.ContainerSymlinksPath
	}
	if src.TempStoragePath != nil && features.LogCollection.TempStoragePath == nil {
		features.LogCollection.TempStoragePath = src.TempStoragePath
	}
	if src.OpenFilesLimit != nil && features.LogCollection.OpenFilesLimit == nil {
		features.LogCollection.OpenFilesLimit = src.OpenFilesLimit
	}
}

func convertProcessSpec(src *ProcessSpec, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	features := getV2Features(dst)
	if features.LiveProcessCollection == nil {
		features.LiveProcessCollection = &v2alpha1.LiveProcessCollectionFeatureConfig{}
	}
	features.LiveProcessCollection.Enabled = src.ProcessCollectionEnabled

	if features.LiveContainerCollection == nil {
		features.LiveContainerCollection = &v2alpha1.LiveContainerCollectionFeatureConfig{}
	}
	features.LiveContainerCollection.Enabled = src.Enabled

	if src.Env != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.ProcessAgentContainerName).Env = src.Env
	}

	if src.VolumeMounts != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.ProcessAgentContainerName).VolumeMounts = src.VolumeMounts
	}

	if src.Resources != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.ProcessAgentContainerName).Resources = src.Resources
	}

	if src.Command != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.ProcessAgentContainerName).Command = src.Command
	}

	if src.Args != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.ProcessAgentContainerName).Args = src.Args
	}
}

func convertSystemProbeSpec(src *SystemProbeSpec, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	features := getV2Features(dst)
	if features.NPM == nil {
		features.NPM = &v2alpha1.NPMFeatureConfig{}
	}

	// TODO: BPFDebugEnabled, DebugPort skipped as not sure if should be exposed.

	setBooleanPtrOR(src.Enabled, &features.NPM.Enabled)
	features.NPM.EnableConntrack = src.ConntrackEnabled
	features.NPM.CollectDNSStats = src.CollectDNSStats

	if src.EnableTCPQueueLength != nil {
		features.TCPQueueLength = &v2alpha1.TCPQueueLengthFeatureConfig{
			Enabled: src.EnableTCPQueueLength,
		}
	}

	if src.EnableOOMKill != nil {
		features.OOMKill = &v2alpha1.OOMKillFeatureConfig{
			Enabled: src.EnableOOMKill,
		}
	}

	if src.Env != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName).Env = src.Env
	}

	if src.VolumeMounts != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName).VolumeMounts = src.VolumeMounts
	}

	if src.Resources != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName).Resources = src.Resources
	}

	if src.Command != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName).Command = src.Command
	}

	if src.Args != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName).Args = src.Args
	}

	if src.SecCompRootPath != "" {
		tmplOverride := getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName)
		ddaGeneric := getV2Container(tmplOverride, commonv1.SystemProbeContainerName)
		ddaGeneric.SeccompConfig = &v2alpha1.SeccompConfig{CustomRootPath: &src.SecCompRootPath}
	}

	if src.SecCompCustomProfileConfigMap != "" {
		tmplOverride := getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName)
		ddaGeneric := getV2Container(tmplOverride, commonv1.SystemProbeContainerName)
		customConfigMap := &v2alpha1.CustomConfig{
			ConfigMap: &commonv1.ConfigMapConfig{
				Name: src.SecCompCustomProfileConfigMap,
			},
		}
		if ddaGeneric.SeccompConfig == nil {
			ddaGeneric.SeccompConfig = &v2alpha1.SeccompConfig{
				CustomProfile: customConfigMap,
			}
		} else if ddaGeneric.SeccompConfig.CustomProfile == nil {
			ddaGeneric.SeccompConfig.CustomProfile = customConfigMap
		} else {
			ddaGeneric.SeccompConfig.CustomProfile.ConfigMap.Name = src.SecCompCustomProfileConfigMap
		}
	}

	if src.SecCompProfileName != "" {
		sysProbeContainer := getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName)
		profile := corev1.SeccompProfile{}
		switch {
		case src.SecCompProfileName == "unconfined":
			profile.Type = corev1.SeccompProfileTypeUnconfined
		case strings.HasPrefix(src.SecCompProfileName, "runtime"):
			profile.Type = corev1.SeccompProfileTypeRuntimeDefault
		case strings.HasPrefix(src.SecCompProfileName, "localhost"):
			profile.Type = corev1.SeccompProfileTypeLocalhost
			profileName := strings.TrimPrefix(src.SecCompProfileName, "localhost/")
			profile.LocalhostProfile = &profileName
		}
		if sysProbeContainer.SecurityContext == nil {
			sysProbeContainer.SecurityContext = &corev1.SecurityContext{
				SeccompProfile: &profile,
			}
		} else if sysProbeContainer.SecurityContext.SeccompProfile == nil {
			sysProbeContainer.SecurityContext.SeccompProfile = &profile
		} else {
			sysProbeContainer.SecurityContext.SeccompProfile.Type = profile.Type
			sysProbeContainer.SecurityContext.SeccompProfile.LocalhostProfile = profile.LocalhostProfile
		}
	}

	if src.AppArmorProfileName != "" {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName).AppArmorProfileName = &src.AppArmorProfileName
	}

	if src.CustomConfig != nil {
		tmpl := getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName)
		if tmpl.CustomConfigurations == nil {
			tmpl.CustomConfigurations = make(map[v2alpha1.AgentConfigFileName]v2alpha1.CustomConfig)
		}

		tmpl.CustomConfigurations[v2alpha1.SystemProbeConfigFile] = *convertConfigMapConfig(src.CustomConfig)
	}

	if src.SecurityContext != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SystemProbeContainerName).SecurityContext = src.SecurityContext
	}
}

func convertSecurityAgentSpec(src *SecuritySpec, dst *v2alpha1.DatadogAgent) {
	if src == nil {
		return
	}

	features := getV2Features(dst)
	if features.CSPM == nil {
		features.CSPM = &v2alpha1.CSPMFeatureConfig{}
	}
	if features.CWS == nil {
		features.CWS = &v2alpha1.CWSFeatureConfig{}
	}

	features.CSPM.Enabled = src.Compliance.Enabled
	features.CSPM.CheckInterval = src.Compliance.CheckInterval
	if features.CSPM.CustomBenchmarks != nil {
		features.CSPM.CustomBenchmarks = &v2alpha1.CustomConfig{
			ConfigMap: &commonv1.ConfigMapConfig{
				Name:  src.Compliance.ConfigDir.ConfigMapName,
				Items: src.Compliance.ConfigDir.Items,
			},
		}
	}

	features.CWS.Enabled = src.Runtime.Enabled
	if src.Runtime.SyscallMonitor != nil {
		features.CWS.SyscallMonitorEnabled = src.Runtime.SyscallMonitor.Enabled
	}
	if features.CWS.CustomPolicies != nil {
		features.CWS.CustomPolicies = &v2alpha1.CustomConfig{
			ConfigMap: &commonv1.ConfigMapConfig{
				Name:  src.Runtime.PoliciesDir.ConfigMapName,
				Items: src.Runtime.PoliciesDir.Items,
			},
		}
	}

	if src.Env != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SecurityAgentContainerName).Env = src.Env
	}

	if src.VolumeMounts != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SecurityAgentContainerName).VolumeMounts = src.VolumeMounts
	}

	if src.Resources != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SecurityAgentContainerName).Resources = src.Resources
	}

	if src.Command != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SecurityAgentContainerName).Command = src.Command
	}

	if src.Args != nil {
		getV2Container(getV2TemplateOverride(&dst.Spec, v2alpha1.NodeAgentComponentName), commonv1.SecurityAgentContainerName).Args = src.Args
	}
}
