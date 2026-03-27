// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package servicediscovery

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	if err := feature.Register(feature.ServiceDiscoveryType, buildFeature); err != nil {
		panic(err)
	}
}

func buildFeature(*feature.Options) feature.Feature {
	return &serviceDiscoveryFeature{}
}

type serviceDiscoveryFeature struct {
	networkStatsEnabled   bool
	userExplicitlyEnabled bool
	// features holds a pointer to the live DDA features struct so that ManageNodeAgent
	// can re-evaluate hasOtherSystemProbeFeatures after Remote Config state has been
	// merged by other features' Configure calls (e.g. USM merges RC state into the spec).
	features *v2alpha1.DatadogFeatures
}

// ID returns the ID of the Feature
func (f *serviceDiscoveryFeature) ID() feature.IDType {
	return feature.ServiceDiscoveryType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *serviceDiscoveryFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features == nil || ddaSpec.Features.ServiceDiscovery == nil {
		return reqComp
	}

	sd := ddaSpec.Features.ServiceDiscovery

	// Explicit Enabled=false always disables the feature, even if EnabledByDefault=true.
	if sd.Enabled != nil && !*sd.Enabled {
		return reqComp
	}
	// Feature requires either an explicit opt-in or a default enablement.
	if !apiutils.BoolValue(sd.Enabled) && !apiutils.BoolValue(sd.EnabledByDefault) {
		return reqComp
	}

	reqComp.Agent = feature.RequiredComponent{
		IsRequired: apiutils.NewBoolPointer(true),
		Containers: []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName},
	}

	f.networkStatsEnabled = true
	if sd.NetworkStats != nil {
		f.networkStatsEnabled = apiutils.BoolValue(sd.NetworkStats.Enabled)
	}

	f.features = ddaSpec.Features
	f.userExplicitlyEnabled = apiutils.BoolValue(sd.Enabled)

	return reqComp
}

// systemProbeLiteCommand returns the shell command for the system-probe container when
// system-probe-lite is preferred. If userOptedIn is true (user explicitly enabled discovery),
// system-probe is used as the fallback — the user has accepted the resource cost.
// Otherwise (enabled by default), the fallback is sleep infinity to avoid unexpectedly
// running system-probe on older agent images where the discovery feature may not be supported.
func systemProbeLiteCommand(socketPath string, userOptedIn bool) string {
	fallback := "sleep infinity"
	if userOptedIn {
		fallback = "system-probe --config=/etc/datadog-agent/system-probe.yaml"
	}
	return fmt.Sprintf("system-probe-lite run --socket %s --log-level ${DD_LOG_LEVEL:-info} || %s", socketPath, fallback)
}

// hasOtherSystemProbeFeatures returns true if any feature besides service discovery
// requires the full system-probe binary. When true, system-probe-lite cannot be used.
func hasOtherSystemProbeFeatures(features *v2alpha1.DatadogFeatures) bool {
	if features == nil {
		return false
	}
	return (features.NPM != nil && apiutils.BoolValue(features.NPM.Enabled)) ||
		(features.CWS != nil && apiutils.BoolValue(features.CWS.Enabled)) ||
		(features.CSPM != nil && apiutils.BoolValue(features.CSPM.Enabled) && apiutils.BoolValue(features.CSPM.RunInSystemProbe)) ||
		(features.USM != nil && apiutils.BoolValue(features.USM.Enabled)) ||
		(features.OOMKill != nil && apiutils.BoolValue(features.OOMKill.Enabled)) ||
		(features.TCPQueueLength != nil && apiutils.BoolValue(features.TCPQueueLength.Enabled)) ||
		(features.EBPFCheck != nil && apiutils.BoolValue(features.EBPFCheck.Enabled)) ||
		(features.GPU != nil && apiutils.BoolValue(features.GPU.Enabled) && apiutils.BoolValue(features.GPU.PrivilegedMode))
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

	if f.networkStatsEnabled {
		// debugfs volume mount
		debugfsVol, debugfsMount := volume.GetVolumes(common.DebugfsVolumeName, common.DebugfsPath, common.DebugfsPath, false)
		managers.VolumeMount().AddVolumeMountToContainer(&debugfsMount, apicommon.SystemProbeContainerName)
		managers.Volume().AddVolume(&debugfsVol)

		// modules volume mount
		modulesVol, modulesVolMount := volume.GetVolumes(common.ModulesVolumeName, common.ModulesVolumePath, common.ModulesVolumePath, true)
		managers.VolumeMount().AddVolumeMountToContainer(&modulesVolMount, apicommon.SystemProbeContainerName)
		managers.Volume().AddVolume(&modulesVol)

		// src volume mount
		_, providerValue := kubernetes.GetProviderLabelKeyValue(provider)
		if providerValue != kubernetes.GKECosType {
			srcVol, srcVolMount := volume.GetVolumes(common.SrcVolumeName, common.SrcVolumePath, common.SrcVolumePath, true)
			managers.VolumeMount().AddVolumeMountToContainer(&srcVolMount, apicommon.SystemProbeContainerName)
			managers.Volume().AddVolume(&srcVol)
		}
	}

	_, socketVolMountReadOnly := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&socketVolMountReadOnly, apicommon.CoreAgentContainerName)

	// env vars
	enableEnvVar := &corev1.EnvVar{
		Name:  DDServiceDiscoveryEnabled,
		Value: "true",
	}

	netStatsEnvVar := &corev1.EnvVar{
		Name:  DDServiceDiscoveryNetworkStatsEnabled,
		Value: apiutils.BoolToString(&f.networkStatsEnabled),
	}

	managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.SystemProbeContainerName}, enableEnvVar)
	managers.EnvVar().AddEnvVarToInitContainer(apicommon.InitConfigContainerName, enableEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, netStatsEnvVar)

	socketEnvVar := &corev1.EnvVar{
		Name:  common.DDSystemProbeSocket,
		Value: common.DefaultSystemProbeSocketPath,
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, socketEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, socketEnvVar)

	// Direct PodTemplateSpec mutation: no managers API for command overrides.
	// Re-evaluate here (not cached from Configure) so that RC state merged by other
	// features' Configure calls (e.g. USM) is taken into account.
	if !hasOtherSystemProbeFeatures(f.features) {
		for i := range managers.PodTemplateSpec().Spec.Containers {
			c := &managers.PodTemplateSpec().Spec.Containers[i]
			if c.Name == string(apicommon.SystemProbeContainerName) {
				c.Command = []string{"/bin/sh", "-c"}
				c.Args = []string{systemProbeLiteCommand(common.DefaultSystemProbeSocketPath, f.userExplicitlyEnabled)}
				break
			}
		}
	}

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
