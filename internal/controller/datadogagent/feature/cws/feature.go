// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"path/filepath"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	err := feature.Register(feature.CWSIDType, buildCWSFeature)
	if err != nil {
		panic(err)
	}
}

func buildCWSFeature(options *feature.Options) feature.Feature {
	cwsFeat := &cwsFeature{}

	if options != nil {
		cwsFeat.logger = options.Logger
	}

	return cwsFeat
}

type cwsFeature struct {
	syscallMonitorEnabled      bool
	networkEnabled             bool
	activityDumpEnabled        bool
	remoteConfigurationEnabled bool

	owner  metav1.Object
	logger logr.Logger

	customConfig                *v2alpha1.CustomConfig
	configMapName               string
	customConfigAnnotationKey   string
	customConfigAnnotationValue string
}

// ID returns the ID of the Feature
func (f *cwsFeature) ID() feature.IDType {
	return feature.CWSIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *cwsFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// Merge configuration from Status.RemoteConfigConfiguration into the Spec
	mergeConfigs(&dda.Spec, &dda.Status)

	cwsConfig := dda.Spec.Features.CWS

	if cwsConfig != nil && apiutils.BoolValue(cwsConfig.Enabled) {
		f.syscallMonitorEnabled = apiutils.BoolValue(cwsConfig.SyscallMonitorEnabled)

		if cwsConfig.CustomPolicies != nil {
			f.customConfig = cwsConfig.CustomPolicies
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for cws custom policies config")
			} else {
				f.logger.V(2).Info("built cws custom policies from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.CWSIDType)
		}
		f.configMapName = constants.GetConfName(dda, f.customConfig, defaultCWSConf)

		if cwsConfig.Network != nil {
			f.networkEnabled = apiutils.BoolValue(cwsConfig.Network.Enabled)
		}
		if cwsConfig.SecurityProfiles != nil {
			f.activityDumpEnabled = apiutils.BoolValue(cwsConfig.SecurityProfiles.Enabled)
		}

		if dda.Spec.Features != nil && dda.Spec.Features.RemoteConfiguration != nil {
			f.remoteConfigurationEnabled = apiutils.BoolValue(dda.Spec.Features.RemoteConfiguration.Enabled)
			if cwsConfig.RemoteConfiguration != nil {
				f.remoteConfigurationEnabled = f.remoteConfigurationEnabled && apiutils.BoolValue(cwsConfig.RemoteConfiguration.Enabled)
			}
		}

		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.SecurityAgentContainerName,
					apicommon.SystemProbeContainerName,
				},
			},
		}
	}

	return reqComp
}

func mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaStatus *v2alpha1.DatadogAgentStatus) {
	if ddaStatus.RemoteConfigConfiguration == nil || ddaStatus.RemoteConfigConfiguration.Features == nil || ddaStatus.RemoteConfigConfiguration.Features.CWS == nil || ddaStatus.RemoteConfigConfiguration.Features.CWS.Enabled == nil {
		return
	}

	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}

	if ddaSpec.Features.CWS == nil {
		ddaSpec.Features.CWS = &v2alpha1.CWSFeatureConfig{}
	}

	if ddaStatus.RemoteConfigConfiguration.Features.CWS.Enabled != nil {
		ddaSpec.Features.CWS.Enabled = ddaStatus.RemoteConfigConfiguration.Features.CWS.Enabled
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *cwsFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// Create configMap if one does not already exist and ConfigData is defined
	if f.customConfig != nil && f.customConfig.ConfigMap == nil && f.customConfig.ConfigData != nil {
		cm, err := configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configMapName, cwsConfFileName)
		if err != nil {
			return err
		}

		if cm != nil {
			// Add md5 hash annotation for custom config
			if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
				annotations := object.MergeAnnotationsLabels(f.logger, cm.GetAnnotations(), map[string]string{f.customConfigAnnotationKey: f.customConfigAnnotationValue}, "*")
				cm.SetAnnotations(annotations)
			}

			if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
				return err
			}
		}
	}
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// annotations
	managers.Annotation().AddAnnotation(common.SystemProbeAppArmorAnnotationKey, common.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// envvars

	// env vars for Core Agent, Security Agent and System Probe
	containersForEnvVars := []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.SecurityAgentContainerName,
		apicommon.SystemProbeContainerName,
	}

	enabledEnvVar := &corev1.EnvVar{
		Name:  DDRuntimeSecurityConfigEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, enabledEnvVar)

	runtimeSocketEnvVar := &corev1.EnvVar{
		Name:  DDRuntimeSecurityConfigSocket,
		Value: filepath.Join(common.SystemProbeSocketVolumePath, "runtime-security.sock"),
	}
	managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, runtimeSocketEnvVar)

	if f.syscallMonitorEnabled {
		monitorEnvVar := &corev1.EnvVar{
			Name:  DDRuntimeSecurityConfigSyscallMonitorEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainers(containersForEnvVars, monitorEnvVar)
	}

	if f.networkEnabled {
		networkEnvVar := &corev1.EnvVar{
			Name:  DDRuntimeSecurityConfigNetworkEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, networkEnvVar)
	}

	if f.activityDumpEnabled {
		adEnvVar := &corev1.EnvVar{
			Name:  DDRuntimeSecurityConfigActivityDumpEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, adEnvVar)
	}

	if f.remoteConfigurationEnabled {
		rcEnvVar := &corev1.EnvVar{
			Name:  DDRuntimeSecurityConfigRemoteConfigurationEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, rcEnvVar)
	}

	policiesDirEnvVar := &corev1.EnvVar{
		Name:  DDRuntimeSecurityConfigPoliciesDir,
		Value: securityAgentRuntimePoliciesDirVolumePath,
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, policiesDirEnvVar)

	hostRootEnvVar := &corev1.EnvVar{
		Name:  common.DDHostRootEnvVar,
		Value: common.HostRootMountPath,
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.SecurityAgentContainerName, hostRootEnvVar)

	volMountMgr := managers.VolumeMount()
	volMgr := managers.Volume()

	// debugfs volume mount
	debugfsVol, debugfsVolMount := volume.GetVolumes(common.DebugfsVolumeName, common.DebugfsPath, common.DebugfsPath, false)
	volMountMgr.AddVolumeMountToContainer(&debugfsVolMount, apicommon.SystemProbeContainerName)
	volMgr.AddVolume(&debugfsVol)

	// tracefs volume mount
	tracefsVol, tracefsVolMount := volume.GetVolumes(tracefsVolumeName, tracefsPath, tracefsPath, false)
	volMountMgr.AddVolumeMountToContainer(&tracefsVolMount, apicommon.SystemProbeContainerName)
	volMgr.AddVolume(&tracefsVol)

	// securityfs volume mount
	securityfsVol, securityfsVolMount := volume.GetVolumes(securityfsVolumeName, securityfsVolumePath, securityfsMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&securityfsVolMount, apicommon.SystemProbeContainerName)
	volMgr.AddVolume(&securityfsVol)

	// socket volume mount (needs write perms for the system probe container but not the others)
	socketVol, socketVolMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	volMountMgr.AddVolumeMountToContainer(&socketVolMount, apicommon.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketVolMountReadOnly,
		[]apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
			apicommon.SecurityAgentContainerName,
		},
	)
	volMgr.AddVolume(&socketVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&procdirVolMount, apicommon.SystemProbeContainerName)
	volMgr.AddVolume(&procdirVol)

	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(common.PasswdVolumeName, common.PasswdHostPath, common.PasswdMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&passwdVolMount, apicommon.SystemProbeContainerName)
	volMgr.AddVolume(&passwdVol)

	// group volume mount
	groupVol, groupVolMount := volume.GetVolumes(common.GroupVolumeName, common.GroupHostPath, common.GroupMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&groupVolMount, apicommon.SystemProbeContainerName)
	volMgr.AddVolume(&groupVol)

	// osRelease volume mount
	osReleaseVol, osReleaseVolMount := volume.GetVolumes(common.SystemProbeOSReleaseDirVolumeName, common.SystemProbeOSReleaseDirVolumePath, common.SystemProbeOSReleaseDirMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&osReleaseVolMount, apicommon.SystemProbeContainerName)
	volMgr.AddVolume(&osReleaseVol)

	// hostroot volume mount
	hostrootVol, hostrootVolMount := volume.GetVolumes(common.HostRootVolumeName, common.HostRootHostPath, common.HostRootMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&hostrootVolMount, apicommon.SecurityAgentContainerName)
	volMgr.AddVolume(&hostrootVol)

	// Custom policies are copied and merged with default policies via a workaround in the init-volume container.
	if f.customConfig != nil {
		var vol corev1.Volume
		var volMount corev1.VolumeMount

		if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
			managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
		}

		if f.customConfig.ConfigMap != nil {
			// Custom config is referenced via ConfigMap
			// Cannot use standard GetVolumesFromConfigMap because security features are not under /conf.d
			vol = volume.GetVolumeFromConfigMap(f.customConfig.ConfigMap, f.configMapName, cwsConfigVolumeName)
			volMount = corev1.VolumeMount{
				Name:      cwsConfigVolumeName,
				MountPath: securityAgentRuntimeCustomPoliciesVolumePath,
				ReadOnly:  true,
			}
		} else {
			// Custom config is referenced via ConfigData (and configMap is created in ManageDependencies)
			vol = volume.GetBasicVolume(f.configMapName, cwsConfigVolumeName)

			volMount = corev1.VolumeMount{
				Name:      cwsConfigVolumeName,
				MountPath: securityAgentRuntimeCustomPoliciesVolumePath,
				ReadOnly:  true,
			}
		}
		// Mount custom policies to init-volume container.
		managers.VolumeMount().AddVolumeMountToInitContainer(&volMount, apicommon.InitVolumeContainerName)
		managers.Volume().AddVolume(&vol)

		// Add workaround command to init-volume container
		for id, container := range managers.PodTemplateSpec().Spec.InitContainers {
			if container.Name == "init-volume" {
				managers.PodTemplateSpec().Spec.InitContainers[id].Args = []string{
					managers.PodTemplateSpec().Spec.InitContainers[id].Args[0] + ";cp -v /etc/datadog-agent-runtime-policies/* /opt/datadog-agent/runtime-security.d/",
				}
			}
		}

		// Add policies directory envvar to Security Agent, and empty volume to System Probe and Security Agent.
		managers.EnvVar().AddEnvVarToContainer(apicommon.SecurityAgentContainerName, policiesDirEnvVar)

		policiesVol, policiesVolMount := volume.GetVolumesEmptyDir(securityAgentRuntimePoliciesDirVolumeName, securityAgentRuntimePoliciesDirVolumePath, true)
		volMgr.AddVolume(&policiesVol)
		volMountMgr.AddVolumeMountToContainers(&policiesVolMount, []apicommon.AgentContainerName{apicommon.SecurityAgentContainerName, apicommon.SystemProbeContainerName})

		// Add runtime-security.d volume mount to init-volume container at different path
		policiesVolMountInitVol := corev1.VolumeMount{
			Name:      securityAgentRuntimePoliciesDirVolumeName,
			MountPath: "/opt/datadog-agent/runtime-security.d",
			ReadOnly:  false,
		}
		volMountMgr.AddVolumeMountToInitContainer(&policiesVolMountInitVol, apicommon.InitVolumeContainerName)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
