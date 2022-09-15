// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"path/filepath"

	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.CWSIDType, buildCWSFeature)
	if err != nil {
		panic(err)
	}
}

func buildCWSFeature(options *feature.Options) feature.Feature {
	cwsFeat := &cwsFeature{}

	return cwsFeat
}

type cwsFeature struct {
	configMapConfig       *apicommonv1.ConfigMapConfig
	syscallMonitorEnabled bool
}

// ID returns the ID of the Feature
func (f *cwsFeature) ID() feature.IDType {
	return feature.CWSIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *cwsFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features != nil && dda.Spec.Features.CWS != nil && apiutils.BoolValue(dda.Spec.Features.CWS.Enabled) {
		cws := dda.Spec.Features.CWS

		f.syscallMonitorEnabled = apiutils.BoolValue(cws.SyscallMonitorEnabled)

		if cws.CustomPolicies != nil && cws.CustomPolicies.Name != "" {
			f.configMapConfig = cws.CustomPolicies
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.SecurityAgentContainerName,
					apicommonv1.SystemProbeContainerName,
				},
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *cwsFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Agent.Security != nil && *dda.Spec.Agent.Security.Runtime.Enabled {
		runtime := dda.Spec.Agent.Security.Runtime

		if runtime.SyscallMonitor != nil && apiutils.BoolValue(runtime.SyscallMonitor.Enabled) {
			f.syscallMonitorEnabled = true
		}

		if runtime.PoliciesDir != nil && runtime.PoliciesDir.ConfigMapName != "" {
			f.configMapConfig = v1alpha1.ConvertConfigDirSpec(runtime.PoliciesDir).ConfigMap
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.SecurityAgentContainerName,
					apicommonv1.SystemProbeContainerName,
				},
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *cwsFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// annotations
	managers.Annotation().AddAnnotation(apicommon.SystemProbeAppArmorAnnotationKey, apicommon.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommonv1.SystemProbeContainerName)

	// envvars
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDRuntimeSecurityConfigEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, enabledEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, enabledEnvVar)

	runtimeSocketEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDRuntimeSecurityConfigSocket,
		Value: filepath.Join(apicommon.SystemProbeSocketVolumePath, "runtime-security.sock"),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, runtimeSocketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, runtimeSocketEnvVar)

	if f.syscallMonitorEnabled {
		monitorEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDRuntimeSecurityConfigSyscallMonitorEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, monitorEnvVar)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, monitorEnvVar)
	}

	policiesDirEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDRuntimeSecurityConfigPoliciesDir,
		Value: apicommon.SecurityAgentRuntimePoliciesDirVolumePath,
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, policiesDirEnvVar)

	hostRootEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDHostRootEnvVar,
		Value: apicommon.HostRootMountPath,
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, hostRootEnvVar)

	volMountMgr := managers.VolumeMount()
	volMgr := managers.Volume()

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(apicommon.DebugfsVolumeName, apicommon.DebugfsPath, apicommon.DebugfsPath, false)
	volMountMgr.AddVolumeMountToContainer(&debugfsVolMount, apicommonv1.SystemProbeContainerName)
	volMgr.AddVolume(&debugfsVol)

	// securityfs volume mount
	securityfsVol, securityfsVolMount := volume.GetVolumes(apicommon.SecurityfsVolumeName, apicommon.SecurityfsVolumePath, apicommon.SecurityfsMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&securityfsVolMount, apicommonv1.SystemProbeContainerName)
	volMgr.AddVolume(&securityfsVol)

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, false)
	volMountMgr.AddVolumeMountToContainer(&socketVolMount, apicommonv1.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommonv1.SecurityAgentContainerName)
	volMgr.AddVolume(&socketVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(apicommon.ProcdirVolumeName, apicommon.ProcdirHostPath, apicommon.ProcdirMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&procdirVolMount, apicommonv1.SystemProbeContainerName)
	volMgr.AddVolume(&procdirVol)

	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(apicommon.PasswdVolumeName, apicommon.PasswdHostPath, apicommon.PasswdMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&passwdVolMount, apicommonv1.SystemProbeContainerName)
	volMgr.AddVolume(&passwdVol)

	// group volume mount
	groupVol, groupVolMount := volume.GetVolumes(apicommon.GroupVolumeName, apicommon.GroupHostPath, apicommon.GroupMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&groupVolMount, apicommonv1.SystemProbeContainerName)
	volMgr.AddVolume(&groupVol)

	// osRelease volume mount
	osReleaseVol, osReleaseVolMount := volume.GetVolumes(apicommon.SystemProbeOSReleaseDirVolumeName, apicommon.SystemProbeOSReleaseDirVolumePath, apicommon.SystemProbeOSReleaseDirMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&osReleaseVolMount, apicommonv1.SystemProbeContainerName)
	volMgr.AddVolume(&osReleaseVol)

	// hostroot volume mount
	hostrootVol, hostrootVolMount := volume.GetVolumes(apicommon.HostRootVolumeName, apicommon.HostRootHostPath, apicommon.HostRootMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&hostrootVolMount, apicommonv1.SecurityAgentContainerName)
	volMgr.AddVolume(&hostrootVol)

	// custom runtime policies
	if f.configMapConfig != nil {
		cmVol, cmVolMount := volume.GetConfigMapVolumes(
			f.configMapConfig,
			f.configMapConfig.Name,
			apicommon.SecurityAgentRuntimeCustomPoliciesVolumeName,
			apicommon.SecurityAgentRuntimeCustomPoliciesVolumePath,
		)
		volMountMgr.AddVolumeMountToContainers(&cmVolMount, []apicommonv1.AgentContainerName{apicommonv1.SecurityAgentContainerName, apicommonv1.SystemProbeContainerName})
		volMgr.AddVolume(&cmVol)

		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, policiesDirEnvVar)

		policiesVol, policiesVolMount := volume.GetVolumesEmptyDir(apicommon.SecurityAgentRuntimePoliciesDirVolumeName, apicommon.SecurityAgentRuntimePoliciesDirVolumePath, true)
		volMountMgr.AddVolumeMountToContainers(&policiesVolMount, []apicommonv1.AgentContainerName{apicommonv1.SecurityAgentContainerName, apicommonv1.SystemProbeContainerName})
		volMgr.AddVolume(&policiesVol)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
