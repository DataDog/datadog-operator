// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package cspm

import (
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	err := feature.Register(feature.CSPMIDType, buildCSPMFeature)
	if err != nil {
		panic(err)
	}
}

func buildCSPMFeature(options *feature.Options) feature.Feature {
	cspmFeat := &cspmFeature{}

	if options != nil {
		cspmFeat.logger = options.Logger
	}

	return cspmFeat
}

type cspmFeature struct {
	enable                bool
	serviceAccountName    string
	checkInterval         string
	hostBenchmarksEnabled bool

	owner  metav1.Object
	logger logr.Logger

	customConfig                *v2alpha1.CustomConfig
	configMapName               string
	customConfigAnnotationKey   string
	customConfigAnnotationValue string
}

// ID returns the ID of the Feature
func (f *cspmFeature) ID() feature.IDType {
	return feature.CSPMIDType
}

// Configure is used to configure the feature from a v1alpha1.DatadogAgentInternal instance.
func (f *cspmFeature) Configure(ddai *v1alpha1.DatadogAgentInternal) (reqComp feature.RequiredComponents) {
	f.owner = ddai

	// Merge configuration from Status.RemoteConfigConfiguration into the Spec
	mergeConfigs(&ddai.Spec, &ddai.Status)

	cspmConfig := ddai.Spec.Features.CSPM

	if cspmConfig != nil && apiutils.BoolValue(cspmConfig.Enabled) {
		f.enable = true
		f.serviceAccountName = constants.GetClusterAgentServiceAccountDDAI(ddai)

		if cspmConfig.CheckInterval != nil {
			f.checkInterval = strconv.FormatInt(cspmConfig.CheckInterval.Nanoseconds(), 10)
		}

		if cspmConfig.CustomBenchmarks != nil {
			f.customConfig = cspmConfig.CustomBenchmarks
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for cspm custom benchmarks config")
			} else {
				f.logger.V(2).Info("built cspm custom benchmarks from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.CSPMIDType)
		}
		f.configMapName = constants.GetConfName(ddai, f.customConfig, defaultCSPMConf)

		if cspmConfig.HostBenchmarks != nil && apiutils.BoolValue(cspmConfig.HostBenchmarks.Enabled) {
			f.hostBenchmarksEnabled = true
		}

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
			},
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.SecurityAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

func mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaiStatus *v1alpha1.DatadogAgentInternalStatus) {
	if ddaiStatus.RemoteConfigConfiguration == nil || ddaiStatus.RemoteConfigConfiguration.Features == nil || ddaiStatus.RemoteConfigConfiguration.Features.CSPM == nil || ddaiStatus.RemoteConfigConfiguration.Features.CSPM.Enabled == nil {
		return
	}

	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}

	if ddaSpec.Features.CSPM == nil {
		ddaSpec.Features.CSPM = &v2alpha1.CSPMFeatureConfig{}
	}

	if ddaiStatus.RemoteConfigConfiguration.Features.CSPM.Enabled != nil {
		ddaSpec.Features.CSPM.Enabled = ddaiStatus.RemoteConfigConfiguration.Features.CSPM.Enabled
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *cspmFeature) ManageDependencies(managers feature.ResourceManagers) error {
	// Create configMap if one does not already exist and ConfigData is defined
	if f.customConfig != nil && f.customConfig.ConfigMap == nil && f.customConfig.ConfigData != nil {
		cm, err := configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configMapName, cspmConfFileName)
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

	// Manage RBAC
	rbacName := getRBACResourceName(f.owner)

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	if f.customConfig != nil {
		var vol corev1.Volume
		var volMount corev1.VolumeMount

		if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
			managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
		}

		if f.customConfig.ConfigMap != nil {
			// Custom config is referenced via ConfigMap
			// Cannot use standard GetVolumesFromConfigMap because security features are not under /conf.d
			vol = volume.GetVolumeFromConfigMap(f.customConfig.ConfigMap, f.configMapName, cspmConfigVolumeName)

			// Need to use subpaths so that existing configurations are not overwritten
			for _, item := range f.customConfig.ConfigMap.Items {
				volMount = corev1.VolumeMount{
					Name:      cspmConfigVolumeName,
					MountPath: securityAgentComplianceConfigDirVolumePath + "/" + item.Path,
					SubPath:   item.Path,
					ReadOnly:  true,
				}

				managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.ClusterAgentContainerName)
			}
		} else {
			// Custom config is referenced via ConfigData (and configMap is created in ManageDependencies)
			vol = volume.GetBasicVolume(f.configMapName, cspmConfigVolumeName)

			// Need to use subpaths so that existing configurations are not overwritten
			volMount = volume.GetVolumeMountWithSubPath(
				cspmConfigVolumeName,
				securityAgentComplianceConfigDirVolumePath+"/"+cspmConfFileName,
				cspmConfFileName,
			)
			managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.ClusterAgentContainerName)
		}
		// Mount custom policies to cluster agent container.
		managers.Volume().AddVolume(&vol)
	}

	enabledEnvVar := &corev1.EnvVar{
		Name:  DDComplianceConfigEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, enabledEnvVar)

	if f.checkInterval != "" {
		intervalEnvVar := &corev1.EnvVar{
			Name:  DDComplianceConfigCheckInterval,
			Value: f.checkInterval,
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, intervalEnvVar)
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// security context capabilities
	capabilities := []corev1.Capability{
		"AUDIT_CONTROL",
		"AUDIT_READ",
	}
	managers.SecurityContext().AddCapabilitiesToContainer(capabilities, apicommon.SecurityAgentContainerName)

	volMountMgr := managers.VolumeMount()
	VolMgr := managers.Volume()

	// Custom policies are copied and merged with default policies via a workaround in the init-volume container.
	if f.customConfig != nil {
		var vol corev1.Volume
		var volMount corev1.VolumeMount

		if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
			managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
		}

		if f.customConfig.ConfigMap != nil {
			// Custom config is referenced via ConfigMap
			// Cannot use typical GetVolumesFromConfigMap because security features are not under /conf.d
			vol = volume.GetVolumeFromConfigMap(f.customConfig.ConfigMap, f.configMapName, cspmConfigVolumeName)
			volMount = corev1.VolumeMount{
				Name:      cspmConfigVolumeName,
				MountPath: "/etc/datadog-agent-compliance-benchmarks",
				ReadOnly:  true,
			}
		} else {
			// Custom config is referenced via ConfigData (and configMap is created in ManageDependencies)
			vol = volume.GetBasicVolume(f.configMapName, cspmConfigVolumeName)

			volMount = corev1.VolumeMount{
				Name:      cspmConfigVolumeName,
				MountPath: "/etc/datadog-agent-compliance-benchmarks",
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
					managers.PodTemplateSpec().Spec.InitContainers[id].Args[0] + ";cp -v /etc/datadog-agent-compliance-benchmarks/* /opt/datadog-agent/compliance.d/",
				}
			}
		}

		// Add empty volume to Security Agent
		benchmarksVol, benchmarksVolMount := volume.GetVolumesEmptyDir(securityAgentComplianceConfigDirVolumeName, securityAgentComplianceConfigDirVolumePath, true)
		managers.Volume().AddVolume(&benchmarksVol)
		managers.VolumeMount().AddVolumeMountToContainer(&benchmarksVolMount, apicommon.SecurityAgentContainerName)

		// Add compliance.d volume mount to init-volume container at different path
		benchmarkVolMountInitVol := corev1.VolumeMount{
			Name:      securityAgentComplianceConfigDirVolumeName,
			MountPath: "/opt/datadog-agent/compliance.d",
			ReadOnly:  false,
		}
		volMountMgr.AddVolumeMountToInitContainer(&benchmarkVolMountInitVol, apicommon.InitVolumeContainerName)
	}

	// cgroups volume mount
	cgroupsVol, cgroupsVolMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&cgroupsVolMount, apicommon.SecurityAgentContainerName)
	VolMgr.AddVolume(&cgroupsVol)

	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(common.PasswdVolumeName, common.PasswdHostPath, common.PasswdMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&passwdVolMount, apicommon.SecurityAgentContainerName)
	VolMgr.AddVolume(&passwdVol)

	// procdir volume mount
	procdirVol, procdirVolMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&procdirVolMount, apicommon.SecurityAgentContainerName)
	VolMgr.AddVolume(&procdirVol)

	// host root volume mount
	hostRootVol, hostRootVolMount := volume.GetVolumes(common.HostRootVolumeName, common.HostRootHostPath, common.HostRootMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&hostRootVolMount, apicommon.SecurityAgentContainerName)
	VolMgr.AddVolume(&hostRootVol)

	// group volume mount
	groupVol, groupVolMount := volume.GetVolumes(common.GroupVolumeName, common.GroupHostPath, common.GroupMountPath, true)
	volMountMgr.AddVolumeMountToContainer(&groupVolMount, apicommon.SecurityAgentContainerName)
	VolMgr.AddVolume(&groupVol)

	// env vars
	enabledEnvVar := &corev1.EnvVar{
		Name:  DDComplianceConfigEnabled,
		Value: "true",
	}
	managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SecurityAgentContainerName}, enabledEnvVar)

	hostRootEnvVar := &corev1.EnvVar{
		Name:  common.DDHostRootEnvVar,
		Value: common.HostRootMountPath,
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.SecurityAgentContainerName, hostRootEnvVar)

	if f.checkInterval != "" {
		intervalEnvVar := &corev1.EnvVar{
			Name:  DDComplianceConfigCheckInterval,
			Value: f.checkInterval,
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.SecurityAgentContainerName, intervalEnvVar)
	}

	hostBenchmarksEnabledEnvVar := &corev1.EnvVar{
		Name:  DDComplianceHostBenchmarksEnabled,
		Value: apiutils.BoolToString(&f.hostBenchmarksEnabled),
	}
	managers.EnvVar().AddEnvVarToContainer(apicommon.SecurityAgentContainerName, hostBenchmarksEnabledEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *cspmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
