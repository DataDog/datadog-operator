// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"path/filepath"

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
	configMapName         string
	syscallMonitorEnabled bool
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *cwsFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features != nil && dda.Spec.Features.CWS != nil && apiutils.BoolValue(dda.Spec.Features.CWS.Enabled) {
		cws := dda.Spec.Features.CWS

		if apiutils.BoolValue(cws.SyscallMonitorEnabled) {
			f.syscallMonitorEnabled = true
		}

		if cws.CustomPolicies != nil && cws.CustomPolicies.Name != "" {
			f.configMapName = cws.CustomPolicies.Name
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
			f.configMapName = runtime.PoliciesDir.ConfigMapName
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

	authTokenPathEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDAuthTokenFilePath,
		Value: filepath.Join(apicommon.AuthVolumePath, "token"),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, authTokenPathEnvVar)

	volMountMgr := managers.VolumeMount()
	VolMgr := managers.Volume()

	// custom runtime policies
	if f.configMapConfig != nil && f.configMapName != "" {
		cmVol, cmVolMount := volume.GetConfigMapVolumes(
			f.configMapConfig,
			f.configMapName,
			apicommon.SecurityAgentRuntimeCustomPoliciesVolumeName,
			apicommon.SecurityAgentRuntimeCustomPoliciesVolumePath,
		)
		volMountMgr.AddVolumeMountToContainers(&cmVolMount, []apicommonv1.AgentContainerName{apicommonv1.SecurityAgentContainerName, apicommonv1.SystemProbeContainerName})
		VolMgr.AddVolume(&cmVol)

		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, policiesDirEnvVar)

		policiesVol, policiesVolMount := volume.GetVolumesEmptyDir(apicommon.SecurityAgentRuntimePoliciesDirVolumeName, apicommon.SecurityAgentRuntimePoliciesDirVolumePath)
		volMountMgr.AddVolumeMountToContainers(&policiesVolMount, []apicommonv1.AgentContainerName{apicommonv1.SecurityAgentContainerName, apicommonv1.SystemProbeContainerName})
		VolMgr.AddVolume(&policiesVol)

		socketDirVol, socketDirMount := volume.GetVolumesEmptyDir(apicommon.SystemProbeSocketVolumeName, apicommon.SystemProbeSocketVolumePath)
		volMountMgr.AddVolumeMountToContainer(&socketDirMount, apicommonv1.SecurityAgentContainerName)
		managers.Volume().AddVolume(&socketDirVol)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
