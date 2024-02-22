// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cws

import (
	"path/filepath"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"

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

	customConfig                *apicommonv1.CustomConfig
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

	if dda.Spec.Features != nil && dda.Spec.Features.CWS != nil && apiutils.BoolValue(dda.Spec.Features.CWS.Enabled) {
		cws := dda.Spec.Features.CWS

		f.syscallMonitorEnabled = apiutils.BoolValue(cws.SyscallMonitorEnabled)

		if cws.CustomPolicies != nil {
			f.customConfig = v2alpha1.ConvertCustomConfig(cws.CustomPolicies)
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for cws custom policies config")
			} else {
				f.logger.V(2).Info("built cws custom policies from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.CWSIDType)
		}
		f.configMapName = apicommonv1.GetConfName(dda, f.customConfig, apicommon.DefaultCWSConf)

		if cws.Network != nil {
			f.networkEnabled = apiutils.BoolValue(cws.Network.Enabled)
		}
		if cws.SecurityProfiles != nil {
			f.activityDumpEnabled = apiutils.BoolValue(cws.SecurityProfiles.Enabled)
		}

		if dda.Spec.Features.RemoteConfiguration != nil {
			f.remoteConfigurationEnabled = apiutils.BoolValue(dda.Spec.Features.RemoteConfiguration.Enabled)
			if cws.RemoteConfiguration != nil {
				f.remoteConfigurationEnabled = f.remoteConfigurationEnabled && apiutils.BoolValue(cws.RemoteConfiguration.Enabled)
			}
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
			f.customConfig = v1alpha1.ConvertConfigDirSpecToCustomConfig(runtime.PoliciesDir)
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
	managers.Annotation().AddAnnotation(apicommon.SystemProbeAppArmorAnnotationKey, apicommon.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommonv1.SystemProbeContainerName)

	// envvars
	enabledEnvVar := &corev1.EnvVar{
		Name:  apicommon.DDRuntimeSecurityConfigEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, enabledEnvVar)
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

	if f.networkEnabled {
		networkEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDRuntimeSecurityConfigNetworkEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, networkEnvVar)
	}

	if f.activityDumpEnabled {
		adEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDRuntimeSecurityConfigActivityDumpEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, adEnvVar)
	}

	if f.remoteConfigurationEnabled {
		rcEnvVar := &corev1.EnvVar{
			Name:  apicommon.DDRuntimeSecurityConfigRemoteConfigurationEnabled,
			Value: "true",
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SystemProbeContainerName, rcEnvVar)
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

	// tracefs volume mount
	tracefsVol, tracefsVolMount := volume.GetVolumes(apicommon.TracefsVolumeName, apicommon.TracefsPath, apicommon.TracefsPath, false)
	volMountMgr.AddVolumeMountToContainer(&tracefsVolMount, apicommonv1.SystemProbeContainerName)
	volMgr.AddVolume(&tracefsVol)

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
				MountPath: apicommon.SecurityAgentRuntimeCustomPoliciesVolumePath,
				ReadOnly:  true,
			}
		} else {
			// Custom config is referenced via ConfigData (and configMap is created in ManageDependencies)
			vol = volume.GetBasicVolume(f.configMapName, cwsConfigVolumeName)

			volMount = corev1.VolumeMount{
				Name:      cwsConfigVolumeName,
				MountPath: apicommon.SecurityAgentRuntimeCustomPoliciesVolumePath,
				ReadOnly:  true,
			}
		}
		// Mount custom policies to init-volume container.
		managers.VolumeMount().AddVolumeMountToInitContainer(&volMount, apicommonv1.InitVolumeContainerName)
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
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.SecurityAgentContainerName, policiesDirEnvVar)

		policiesVol, policiesVolMount := volume.GetVolumesEmptyDir(apicommon.SecurityAgentRuntimePoliciesDirVolumeName, apicommon.SecurityAgentRuntimePoliciesDirVolumePath, true)
		volMgr.AddVolume(&policiesVol)
		volMountMgr.AddVolumeMountToContainers(&policiesVolMount, []apicommonv1.AgentContainerName{apicommonv1.SecurityAgentContainerName, apicommonv1.SystemProbeContainerName})

		// Add runtime-security.d volume mount to init-volume container at different path
		policiesVolMountInitVol := corev1.VolumeMount{
			Name:      apicommon.SecurityAgentRuntimePoliciesDirVolumeName,
			MountPath: "/opt/datadog-agent/runtime-security.d",
			ReadOnly:  false,
		}
		volMountMgr.AddVolumeMountToInitContainer(&policiesVolMountInitVol, apicommonv1.InitVolumeContainerName)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cwsFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
