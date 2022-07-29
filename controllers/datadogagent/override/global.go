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
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

// ApplyGlobalSettings use to apply global setting to a PodTemplateSpec
func ApplyGlobalSettings(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, componentName v2alpha1.ComponentName) *corev1.PodTemplateSpec {
	config := dda.Spec.Global

	// ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app.
	if config.ClusterName != nil {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDClusterName,
			Value: *config.ClusterName,
		})
	}

	// Site is the Datadog intake site Agent data are sent to.
	manager.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDSite,
		Value: *config.Site,
	})

	// Endpoint is the Datadog intake URL the Agent data are sent to.
	if config.Endpoint != nil && config.Endpoint.URL != nil {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDddURL,
			Value: *config.Endpoint.URL,
		})
	}

	// Registry is the image registry to use for all Agent images.
	for _, c := range manager.PodTemplateSpec().Spec.InitContainers {
		c.Image = *config.Registry
	}
	for _, c := range manager.PodTemplateSpec().Spec.Containers {
		c.Image = *config.Registry
	}

	// LogLevel sets logging verbosity. This can be overridden by container.
	manager.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDLogLevel,
		Value: *config.LogLevel,
	})

	// NetworkPolicy contains the network configuration.
	if config.NetworkPolicy != nil {
		if apiutils.BoolValue(config.NetworkPolicy.Create) {
			var err error
			switch config.NetworkPolicy.Flavor {
			case v2alpha1.NetworkPolicyFlavorKubernetes:
				err = resourcesManager.NetworkPolicyManager().AddKubernetesNetworkPolicy(component.BuildKubernetesNetworkPolicy(dda, componentName))
			case v2alpha1.NetworkPolicyFlavorCilium:
				// TODO
			}
			if err != nil {
				logger.Info("Error adding Network Policy to the store", "error", err)
			}
		}
	}

	if componentName == v2alpha1.NodeAgentComponentName {
		// Tags contains a list of tags to attach to every metric, event and service check collected.
		if config.Tags != nil {
			tags, err := json.Marshal(config.Tags)
			if err != nil {
				logger.Info("Failed to unmarshal json input", "error", err)
			} else {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  apicommon.DDTags,
					Value: string(tags),
				})
			}
		}

		// Provide a mapping of Kubernetes Labels to Datadog Tags.
		if config.PodLabelsAsTags != nil {
			podLabelsAsTags, err := json.Marshal(config.PodLabelsAsTags)
			if err != nil {
				logger.Info("Failed to unmarshal json input", "error", err)
			} else {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  apicommon.DDPodLabelsAsTags,
					Value: string(podLabelsAsTags),
				})
			}
		}

		// Provide a mapping of Kubernetes Annotations to Datadog Tags.
		if config.PodAnnotationsAsTags != nil {
			podAnnotationsAsTags, err := json.Marshal(config.PodAnnotationsAsTags)
			if err != nil {
				logger.Info("Failed to unmarshal json input", "error", err)
			} else {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  apicommon.DDPodAnnotationsAsTags,
					Value: string(podAnnotationsAsTags),
				})
			}
		}

		// LocalService contains configuration to customize the internal traffic policy service.
		gitVersion := resourcesManager.Store().GetVersionInfo()
		forceEnableLocalService := config.LocalService != nil && apiutils.BoolValue(config.LocalService.ForceEnableLocalService)
		if component.ShouldCreateAgentLocalService(gitVersion, forceEnableLocalService) {
			var serviceName string
			if config.LocalService != nil && config.LocalService.NameOverride != nil {
				serviceName = *config.LocalService.NameOverride
			}
			err := resourcesManager.ServiceManager().AddService(component.BuildAgentLocalService(dda, serviceName))
			if err != nil {
				logger.Info("Error adding Local Service to the store", "error", err)
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

			if config.Kubelet.TLSVerify != nil {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  apicommon.DDKubeletTLSVerify,
					Value: apiutils.BoolToString(config.Kubelet.TLSVerify),
				})
			}

			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDKubeletCAPath,
				Value: config.Kubelet.AgentCAPath,
			})

			if config.Kubelet.HostCAPath != "" {
				kubeletVol, kubeletVolMount := volume.GetVolumes(apicommon.KubeletCAVolumeName, config.Kubelet.HostCAPath, config.Kubelet.AgentCAPath, true)
				manager.VolumeMount().AddVolumeMountToContainers(
					&kubeletVolMount,
					[]apicommonv1.AgentContainerName{
						apicommonv1.CoreAgentContainerName,
						apicommonv1.ProcessAgentContainerName,
						apicommonv1.TraceAgentContainerName,
					},
				)
				manager.Volume().AddVolume(&kubeletVol)
			}
		}

		// Path to the docker runtime socket.
		if config.DockerSocketPath != nil {
			dockerMountPath := filepath.Join(apicommon.HostCriSocketPathPrefix, *config.DockerSocketPath)
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DockerHost,
				Value: "unix://" + dockerMountPath,
			})
			dockerVol, dockerVolMount := volume.GetVolumes(apicommon.CriSocketVolumeName, *config.DockerSocketPath, dockerMountPath, true)
			manager.VolumeMount().AddVolumeMountToContainers(
				&dockerVolMount,
				[]apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.ProcessAgentContainerName,
					apicommonv1.SecurityAgentContainerName,
				},
			)
			manager.Volume().AddVolume(&dockerVol)
		}

		// Path to the container runtime socket (if different from Docker).
		if config.CriSocketPath != nil {
			criSocketMountPath := filepath.Join(apicommon.HostCriSocketPathPrefix, *config.CriSocketPath)
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDCriSocketPath,
				Value: criSocketMountPath,
			})
			criVol, criVolMount := volume.GetVolumes(apicommon.CriSocketVolumeName, *config.CriSocketPath, criSocketMountPath, true)
			manager.VolumeMount().AddVolumeMountToContainers(
				&criVolMount,
				[]apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.ProcessAgentContainerName,
					apicommonv1.SecurityAgentContainerName,
				},
			)
			manager.Volume().AddVolume(&criVol)
		}
	}

	return manager.PodTemplateSpec()
}
