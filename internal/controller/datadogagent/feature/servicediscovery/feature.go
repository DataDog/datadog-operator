// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/images"
	pkgutils "github.com/DataDog/datadog-operator/pkg/utils"
)

const serviceDiscoveryAutoEnableMinVersion = "7.78.0-0"

func init() {
	if err := feature.Register(feature.ServiceDiscoveryType, buildFeature); err != nil {
		panic(err)
	}
}

func buildFeature(*feature.Options) feature.Feature {
	return &serviceDiscoveryFeature{}
}

type serviceDiscoveryFeature struct {
	useSystemProbeLite bool
}

// ID returns the ID of the Feature
func (f *serviceDiscoveryFeature) ID() feature.IDType {
	return feature.ServiceDiscoveryType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *serviceDiscoveryFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.useSystemProbeLite = false
	if resolveEnabled(ddaSpec) {
		f.useSystemProbeLite = shouldEnableServiceDiscoveryByDefault(ddaSpec)
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: ptr.To(true),
			Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
		}
	}

	return reqComp
}

// resolveEnabled applies service discovery's version-aware defaulting to ddaSpec when enabled is omitted.
// It returns the resolved enabled value.
func resolveEnabled(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	if ddaSpec.Features == nil {
		return false
	}

	if ddaSpec.Features.ServiceDiscovery == nil {
		ddaSpec.Features.ServiceDiscovery = &v2alpha1.ServiceDiscoveryFeatureConfig{}
	}

	if ddaSpec.Features.ServiceDiscovery.Enabled == nil {
		ddaSpec.Features.ServiceDiscovery.Enabled = ptr.To(shouldEnableServiceDiscoveryByDefault(ddaSpec))
	}

	return apiutils.BoolValue(ddaSpec.Features.ServiceDiscovery.Enabled)
}

func shouldEnableServiceDiscoveryByDefault(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	// Agent version must be >= 7.78.0 to enable service discovery by default.
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent != nil && nodeAgent.Image != nil {
			return pkgutils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), serviceDiscoveryAutoEnableMinVersion, ptr.To(true))
		}
	}
	return pkgutils.IsAboveMinVersion(images.AgentLatestVersion, serviceDiscoveryAutoEnableMinVersion, nil)
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *serviceDiscoveryFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *serviceDiscoveryFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *serviceDiscoveryFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// annotations
	managers.Annotation().AddAnnotation(common.SystemProbeAppArmorAnnotationKey, common.SystemProbeAppArmorAnnotationValue)

	// security context capabilities
	managers.SecurityContext().AddCapabilitiesToContainer(agent.DefaultCapabilitiesForSystemProbe(), apicommon.SystemProbeContainerName)

	// socket volume mount (needs write perms for the system probe container but not the others)
	procdirVol, procdirMount := volume.GetVolumes(common.ProcdirVolumeName, common.ProcdirHostPath, common.ProcdirMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&procdirMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&procdirVol)

	// Needed to resolve container information
	cgroupsVol, cgroupsMount := volume.GetVolumes(common.CgroupsVolumeName, common.CgroupsHostPath, common.CgroupsMountPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&cgroupsMount, apicommon.SystemProbeContainerName)
	managers.Volume().AddVolume(&cgroupsVol)

	socketVol, socketVolMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMount, apicommon.SystemProbeContainerName)

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommon.CoreAgentContainerName)

	// env vars
	enableEnvVar := &corev1.EnvVar{
		Name:  DDServiceDiscoveryEnabled,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName}, enableEnvVar)
	managers.EnvVar().AddEnvVarToInitContainer(apicommon.InitConfigContainerName, enableEnvVar)

	if f.useSystemProbeLite {
		managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, &corev1.EnvVar{
			Name:  DDServiceDiscoveryUseSystemProbeLite,
			Value: "true",
		})
	}

	socketEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeSocket,
		Value: common.DefaultSystemProbeSocketPath,
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, socketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, socketEnvVar)

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *serviceDiscoveryFeature) ManageSingleContainerNodeAgent(feature.PodTemplateManagers, string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *serviceDiscoveryFeature) ManageClusterChecksRunner(feature.PodTemplateManagers, string) error {
	return nil
}

func (f *serviceDiscoveryFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
