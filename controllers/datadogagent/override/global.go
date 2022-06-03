// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"encoding/json"
	"path/filepath"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
)

// ApplyGlobalSettings use to apply global setting to a PodTemplateSpec
func ApplyGlobalSettings(manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) *corev1.PodTemplateSpec {
	config := dda.Spec.Global

	// ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app.
	if *config.ClusterName != "" {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDClusterName,
			Value: *config.ClusterName,
		})
	}

	// Site is the Datadog intake site Agent data are sent to.
	if *config.Site != "" {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSite,
			Value: *config.Site,
		})
	}

	// Endpoint is the Datadog intake URL the Agent data are sent to.
	if config.Endpoint != nil && *config.Endpoint.URL != "" {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDddURL,
			Value: *config.Endpoint.URL,
		})
	}

	// Registry is the image registry to use for all Agent images.
	if *config.Registry != "" {
		for _, c := range manager.PodTemplateSpec().Spec.InitContainers {
			c.Image = *config.Registry
		}
		for _, c := range manager.PodTemplateSpec().Spec.Containers {
			c.Image = *config.Registry
		}
	}

	// LogLevel sets logging verbosity. This can be overridden by container.
	if *config.LogLevel != "" {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDLogLevel,
			Value: *config.LogLevel,
		})
	}

	// Tags contains a list of tags to attach to every metric, event and service check collected.
	if config.Tags != nil {
		tags, _ := json.Marshal(config.Tags)
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDTags,
			Value: string(tags),
		})
	}

	// Provide a mapping of Kubernetes Labels to Datadog Tags.
	if config.PodLabelsAsTags != nil {
		podLabelsAsTags, _ := json.Marshal(config.PodLabelsAsTags)
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDPodLabelsAsTags,
			Value: string(podLabelsAsTags),
		})
	}

	// Provide a mapping of Kubernetes Annotations to Datadog Tags.
	if config.PodAnnotationsAsTags != nil {
		podAnnotationsAsTags, _ := json.Marshal(config.PodAnnotationsAsTags)
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDPodAnnotationsAsTags,
			Value: string(podAnnotationsAsTags),
		})
	}

	// NetworkPolicy contains the network configuration.
	if config.NetworkPolicy != nil {
		if apiutils.BoolValue(config.NetworkPolicy.Create) {
			switch config.NetworkPolicy.Flavor {
			case v2alpha1.NetworkPolicyFlavorKubernetes:
				// node agent
				_ = resourcesManager.NetworkPolicyManager().BuildAgentKubernetesNetworkPolicy(dda)
				// dca
				_ = resourcesManager.NetworkPolicyManager().BuildDCAKubernetesNetworkPolicy(dda)
				// ccr
				if apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners) {
					_ = resourcesManager.NetworkPolicyManager().BuildCCRKubernetesNetworkPolicy(dda)
				}
			case v2alpha1.NetworkPolicyFlavorCilium:
				// TODO
				// node agent
				// dca
				// ccr
			}
		}
	}

	// LocalService contains configuration to customize the internal traffic policy service.
	gitVersion := resourcesManager.Store().GetVersionInfo()
	// Service Internal Traffic Policy exists in Kube 1.21 but it is enabled by default since 1.22
	if utils.IsAboveMinVersion(gitVersion, "1.21-0") {
		if config.LocalService != nil {
			if apiutils.BoolValue(config.LocalService.ForceEnableLocalService) || utils.IsAboveMinVersion(gitVersion, "1.22-0") {
				if config.LocalService.NameOverride != nil {
					resourcesManager.ServiceManager().BuildAgentLocalService(dda, *config.LocalService.NameOverride)
				} else {
					resourcesManager.ServiceManager().BuildAgentLocalService(dda, "")
				}
			}
		}
	}

	// Kubelet contains the kubelet configuration parameters.
	if config.Kubelet != nil {
		var kubeletHostValueFrom *corev1.EnvVarSource
		if config.Kubelet.Host != nil {
			kubeletHostValueFrom = config.Kubelet.Host
		} else {
			kubeletHostValueFrom = &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: apicommon.FieldPathStatusHostIP,
				},
			}
		}
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:      apicommon.DDKubeletHost,
			ValueFrom: kubeletHostValueFrom,
		})

		// tlsverify defaults to true in agent code
		if !*config.Kubelet.TLSVerify {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDKubeletTLSVerify,
				Value: "false",
			})
		}

		if config.Kubelet.AgentCAPath != "" || config.Kubelet.HostCAPath != "" {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDKubeletCAPath,
				Value: getAgentCAPath(config.Kubelet),
			})
		}

		if config.Kubelet.HostCAPath != "" {
			kubeletVol, kubeletVolMount := volume.GetVolumes(apicommon.KubeletCAVolumeName, config.Kubelet.HostCAPath, getAgentCAPath(config.Kubelet), true)
			manager.VolumeMount().AddVolumeMountToContainers(&kubeletVolMount, getContainerList(manager))
			manager.Volume().AddVolume(&kubeletVol)
		}
	}

	// Path to the docker runtime socket.
	if *config.DockerSocketPath != "" {
		dockerMountPath := filepath.Join(apicommon.HostCriSocketPathPrefix, *config.DockerSocketPath)
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDDockerHost,
			Value: "unix://" + dockerMountPath,
		})
		dockerVol, dockerVolMount := volume.GetVolumes(apicommon.CriSocketVolumeName, *config.DockerSocketPath, dockerMountPath, true)
		manager.VolumeMount().AddVolumeMountToContainers(&dockerVolMount, getContainerList(manager))
		manager.Volume().AddVolume(&dockerVol)
	}

	// Path to the container runtime socket (if different from Docker).
	if *config.CriSocketPath != "" {
		criSocketMountPath := filepath.Join(apicommon.HostCriSocketPathPrefix, *config.CriSocketPath)
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDCriSocketPath,
			Value: criSocketMountPath,
		})
		criVol, criVolMount := volume.GetVolumes(apicommon.CriSocketVolumeName, *config.CriSocketPath, criSocketMountPath, true)
		manager.VolumeMount().AddVolumeMountToContainers(&criVolMount, getContainerList(manager))
		manager.Volume().AddVolume(&criVol)
	}

	return manager.PodTemplateSpec()
}

func getContainerList(manager feature.PodTemplateManagers) []apicommonv1.AgentContainerName {
	contList := []apicommonv1.AgentContainerName{}
	for _, c := range manager.PodTemplateSpec().Spec.InitContainers {
		contList = append(contList, apicommonv1.AgentContainerName(c.Name))
	}
	for _, c := range manager.PodTemplateSpec().Spec.Containers {
		contList = append(contList, apicommonv1.AgentContainerName(c.Name))
	}
	return contList
}

func getAgentCAPath(kubeletConfig *apicommonv1.KubeletConfig) string {
	if kubeletConfig.AgentCAPath != "" {
		return kubeletConfig.AgentCAPath
	}
	// host path is set, use `/var/run/host-kubelet-ca.crt`
	return apicommon.DefaultKubeletAgentCAPath
	// if nothing is set, agent code defaults to `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`
}
