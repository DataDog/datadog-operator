// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/defaulting"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ApplyGlobalSettingsClusterAgent(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers) *corev1.PodTemplateSpec {
	return applyGlobalSettings(logger, manager, dda, resourcesManager, v2alpha1.ClusterAgentComponentName, false)
}

func ApplyGlobalSettingsClusterChecksRunner(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers) *corev1.PodTemplateSpec {
	return applyGlobalSettings(logger, manager, dda, resourcesManager, v2alpha1.ClusterChecksRunnerComponentName, false)
}

func ApplyGlobalSettingsNodeAgent(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, usesMultiProcessCoreAgent bool) *corev1.PodTemplateSpec {
	return applyGlobalSettings(logger, manager, dda, resourcesManager, v2alpha1.NodeAgentComponentName, usesMultiProcessCoreAgent)
}

// ApplyGlobalSettings use to apply global setting to a PodTemplateSpec
func applyGlobalSettings(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, componentName v2alpha1.ComponentName, usesMultiProcessCoreAgent bool) *corev1.PodTemplateSpec {
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
	if *config.Registry != apicommon.DefaultImageRegistry {
		image := apicommon.DefaultAgentImageName
		version := defaulting.AgentLatestVersion
		if componentName == v2alpha1.ClusterAgentComponentName {
			image = apicommon.DefaultClusterAgentImageName
			version = defaulting.ClusterAgentLatestVersion
		}
		fullImage := fmt.Sprintf("%s/%s:%s", *config.Registry, image, version)

		for idx := range manager.PodTemplateSpec().Spec.InitContainers {
			manager.PodTemplateSpec().Spec.InitContainers[idx].Image = fullImage
		}

		for idx := range manager.PodTemplateSpec().Spec.Containers {
			manager.PodTemplateSpec().Spec.Containers[idx].Image = fullImage
		}
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
				var ddURL string
				var dnsSelectorEndpoints []metav1.LabelSelector
				if config.Endpoint != nil && *config.Endpoint.URL != "" {
					ddURL = *config.Endpoint.URL
				}
				if config.NetworkPolicy.DNSSelectorEndpoints != nil {
					dnsSelectorEndpoints = config.NetworkPolicy.DNSSelectorEndpoints
				}
				err = resourcesManager.CiliumPolicyManager().AddCiliumPolicy(
					component.BuildCiliumPolicy(
						dda,
						*config.Site,
						ddURL,
						v2alpha1.IsHostNetworkEnabled(dda, v2alpha1.ClusterAgentComponentName),
						dnsSelectorEndpoints,
						componentName,
					),
				)
			}
			if err != nil {
				logger.Error(err, "Error adding Network Policy to the store")
			}
		}
	}

	// Tags contains a list of tags to attach to every metric, event and service check collected.
	if config.Tags != nil {
		tags, err := json.Marshal(config.Tags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
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
			logger.Error(err, "Failed to unmarshal json input")
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
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDPodAnnotationsAsTags,
				Value: string(podAnnotationsAsTags),
			})
		}
	}

	// Provide a mapping of Kubernetes Node Labels to Datadog Tags.
	if config.NodeLabelsAsTags != nil {
		nodeLabelsAsTags, err := json.Marshal(config.NodeLabelsAsTags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDNodeLabelsAsTags,
				Value: string(nodeLabelsAsTags),
			})
		}
	}

	// Provide a mapping of Kubernetes Namespace Labels to Datadog Tags.
	if config.NamespaceLabelsAsTags != nil {
		namespaceLabelsAsTags, err := json.Marshal(config.NamespaceLabelsAsTags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDNamespaceLabelsAsTags,
				Value: string(namespaceLabelsAsTags),
			})
		}
	}

	if componentName == v2alpha1.NodeAgentComponentName {
		// Kubelet contains the kubelet configuration parameters.
		// The environment variable `DD_KUBERNETES_KUBELET_HOST` defaults to `status.hostIP` if not overriden.
		if config.Kubelet != nil {
			if config.Kubelet.Host != nil {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:      apicommon.DDKubeletHost,
					ValueFrom: config.Kubelet.Host,
				})
			}
			if config.Kubelet.TLSVerify != nil {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  apicommon.DDKubeletTLSVerify,
					Value: apiutils.BoolToString(config.Kubelet.TLSVerify),
				})
			}
			if config.Kubelet.HostCAPath != "" {
				var agentCAPath string
				// If the user configures a Kubelet CA certificate, it is mounted in AgentCAPath.
				// The default mount value is `/var/run/host-kubelet-ca.crt`, which can be overriden by the user-provided parameter.
				if config.Kubelet.AgentCAPath != "" {
					agentCAPath = config.Kubelet.AgentCAPath
				} else {
					agentCAPath = apicommon.KubeletAgentCAPath
				}
				kubeletVol, kubeletVolMount := volume.GetVolumes(apicommon.KubeletCAVolumeName, config.Kubelet.HostCAPath, agentCAPath, true)
				if usesMultiProcessCoreAgent {
					manager.VolumeMount().AddVolumeMountToContainers(
						&kubeletVolMount,
						[]apicommonv1.AgentContainerName{
							apicommonv1.UnprivilegedSingleAgentContainerName,
						},
					)
					manager.Volume().AddVolume(&kubeletVol)
				} else {
					manager.VolumeMount().AddVolumeMountToContainers(
						&kubeletVolMount,
						[]apicommonv1.AgentContainerName{
							apicommonv1.CoreAgentContainerName,
							apicommonv1.ProcessAgentContainerName,
							apicommonv1.TraceAgentContainerName,
							apicommonv1.SecurityAgentContainerName,
						},
					)
					manager.Volume().AddVolume(&kubeletVol)
				}
				// If the HostCAPath is overridden, set the environment variable `DD_KUBELET_CLIENT_CA`. The default value in the Agent is `/var/run/secrets/kubernetes.io/serviceaccount/ca.crt`.
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  apicommon.DDKubeletCAPath,
					Value: agentCAPath,
				})
			}
		}

		var runtimeVol corev1.Volume
		var runtimeVolMount corev1.VolumeMount
		// Path to the docker runtime socket.
		if config.DockerSocketPath != nil {
			dockerMountPath := filepath.Join(apicommon.HostCriSocketPathPrefix, *config.DockerSocketPath)
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DockerHost,
				Value: "unix://" + dockerMountPath,
			})
			runtimeVol, runtimeVolMount = volume.GetVolumes(apicommon.CriSocketVolumeName, *config.DockerSocketPath, dockerMountPath, true)
		} else if config.CriSocketPath != nil {
			// Path to the container runtime socket (if different from Docker).
			criSocketMountPath := filepath.Join(apicommon.HostCriSocketPathPrefix, *config.CriSocketPath)
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDCriSocketPath,
				Value: criSocketMountPath,
			})
			runtimeVol, runtimeVolMount = volume.GetVolumes(apicommon.CriSocketVolumeName, *config.CriSocketPath, criSocketMountPath, true)
		}
		if runtimeVol.Name != "" && runtimeVolMount.Name != "" {

			if usesMultiProcessCoreAgent {
				manager.VolumeMount().AddVolumeMountToContainers(
					&runtimeVolMount,
					[]apicommonv1.AgentContainerName{
						apicommonv1.UnprivilegedSingleAgentContainerName,
					},
				)
				manager.Volume().AddVolume(&runtimeVol)
			} else {
				manager.VolumeMount().AddVolumeMountToContainers(
					&runtimeVolMount,
					[]apicommonv1.AgentContainerName{
						apicommonv1.CoreAgentContainerName,
						apicommonv1.ProcessAgentContainerName,
						apicommonv1.TraceAgentContainerName,
						apicommonv1.SecurityAgentContainerName,
					},
				)
				manager.Volume().AddVolume(&runtimeVol)
			}
		}
	}

	return manager.PodTemplateSpec()
}
