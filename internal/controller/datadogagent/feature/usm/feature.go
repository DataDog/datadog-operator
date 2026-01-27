// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package usm

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.USMIDType, buildUSMFeature)
	if err != nil {
		panic(err)
	}
}

func buildUSMFeature(options *feature.Options) feature.Feature {
	usmFeat := &usmFeature{}

	return usmFeat
}

type usmFeature struct {
	directSend bool
}

// ID returns the ID of the Feature
func (f *usmFeature) ID() feature.IDType {
	return feature.USMIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *usmFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaRCStatus *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	// Merge configuration from Status.RemoteConfigConfiguration into the Spec
	mergeConfigs(ddaSpec, ddaRCStatus)

	usmConfig := ddaSpec.Features.USM

	if usmConfig != nil && apiutils.BoolValue(usmConfig.Enabled) {
		containers := []apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
			apicommon.SystemProbeContainerName,
		}
		if !apiutils.BoolValue(ddaSpec.Features.NPM.DirectSend) {
			containers = append(containers, apicommon.ProcessAgentContainerName)
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: containers,
			},
		}

		f.directSend = apiutils.BoolValue(ddaSpec.Features.NPM.DirectSend)
	}

	return reqComp
}

func mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaRCStatus *v2alpha1.RemoteConfigConfiguration) {
	if ddaRCStatus == nil || ddaRCStatus.Features == nil || ddaRCStatus.Features.USM == nil || ddaRCStatus.Features.USM.Enabled == nil {
		return
	}

	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}

	if ddaSpec.Features.USM == nil {
		ddaSpec.Features.USM = &v2alpha1.USMFeatureConfig{}
	}

	if ddaRCStatus.Features.USM.Enabled != nil {
		ddaSpec.Features.USM.Enabled = ddaRCStatus.Features.USM.Enabled
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *usmFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// enable HostPID for system-probe
	managers.PodTemplateSpec().Spec.HostPID = true

	// annotations
	managers.Annotation().AddAnnotation(common.SystemProbeAppArmorAnnotationKey, common.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	managers.Volume().AddVolume(&procdirVol)
	managers.VolumeMount().AddVolumeMountToContainers(&procdirVolMount, []apicommon.AgentContainerName{apicommon.ProcessAgentContainerName, apicommon.SystemProbeContainerName})

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	managers.Volume().AddVolume(&cgroupsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&cgroupsVolMount, []apicommon.AgentContainerName{apicommon.ProcessAgentContainerName, apicommon.SystemProbeContainerName})

	debugfsVol, debugfsVolMount := volume.GetVolumes(common.DebugfsVolumeName, common.DebugfsPath, common.DebugfsPath, false)
	managers.Volume().AddVolume(&debugfsVol)
	managers.VolumeMount().AddVolumeMountToContainers(&debugfsVolMount, []apicommon.AgentContainerName{apicommon.ProcessAgentContainerName, apicommon.SystemProbeContainerName})

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommon.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketVolMountReadOnly,
		[]apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
			apicommon.ProcessAgentContainerName,
		},
	)

	// env vars for Core Agent, Process Agent and System Probe
	containersForEnvVars := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.ProcessAgentContainerName,
		apicommon.SystemProbeContainerName,
	}

	enabledEnvVar := &corev1.EnvVar{
		Name:  DDSystemProbeServiceMonitoringEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, enabledEnvVar)

	sysProbeEnableEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, sysProbeEnableEnvVar)

	socketEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeSocket,
		Value: common.DefaultSystemProbeSocketPath,
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, socketEnvVar)

	// env vars for Process Agent only
	sysProbeExternalEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeExternal,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, sysProbeExternalEnvVar)

	// env vars for System Probe only
	cnmDirectSendEnvVar := &corev1.EnvVar{
		Name:  DDSystemProbeCNMDirectSend,
		Value: apiutils.BoolToString(&f.directSend),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, cnmDirectSendEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *usmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *usmFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
