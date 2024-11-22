// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"encoding/json"
	"fmt"
	"path/filepath"

	"strconv"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	rbacv1 "k8s.io/api/rbac/v1"

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
	resourcesManager feature.ResourceManagers, singleContainerStrategyEnabled bool) *corev1.PodTemplateSpec {
	return applyGlobalSettings(logger, manager, dda, resourcesManager, v2alpha1.NodeAgentComponentName, singleContainerStrategyEnabled)
}

// ApplyGlobalSettings use to apply global setting to a PodTemplateSpec
func applyGlobalSettings(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, componentName v2alpha1.ComponentName, singleContainerStrategyEnabled bool) *corev1.PodTemplateSpec {
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
	if *config.Registry != v2alpha1.DefaultImageRegistry {
		image := v2alpha1.DefaultAgentImageName
		version := defaulting.AgentLatestVersion
		if componentName == v2alpha1.ClusterAgentComponentName {
			image = v2alpha1.DefaultClusterAgentImageName
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
				err = resourcesManager.NetworkPolicyManager().AddKubernetesNetworkPolicy(objects.BuildKubernetesNetworkPolicy(dda, componentName))
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
					objects.BuildCiliumPolicy(
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

	// Env is a list of custom global variables that are set across all agents.
	if config.Env != nil {
		for _, envVar := range config.Env {
			manager.EnvVar().AddEnvVar(&envVar)
		}
	}

	// Configure checks tag cardinality if provided
	if componentName == v2alpha1.NodeAgentComponentName {
		if config.ChecksTagCardinality != nil {
			// The value validation happens at the Agent level - if the lower(string) is not `low`, `orchestrator` or `high`, the Agent defaults to `low`.
			// Ref: https://github.com/DataDog/datadog-agent/blob/1d08a6a9783fe271ea3813ddf9abf60244abdf2c/comp/core/tagger/taggerimpl/tagger.go#L173-L177
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDChecksTagCardinality,
				Value: *config.ChecksTagCardinality,
			})
		}
	}

	if config.OriginDetectionUnified != nil && config.OriginDetectionUnified.Enabled != nil {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDOriginDetectionUnified,
			Value: apiutils.BoolToString(config.OriginDetectionUnified.Enabled),
		})
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

	// Provide a mapping of Kubernetes Namespace Annotations to Datadog Tags.
	if config.NamespaceAnnotationsAsTags != nil {
		namespaceAnnotationsAsTags, err := json.Marshal(config.NamespaceAnnotationsAsTags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDNamespaceAnnotationsAsTags,
				Value: string(namespaceAnnotationsAsTags),
			})
		}
	}

	// Provide a mapping of Kubernetes Resource Labels to Datadog Tags.
	if config.KubernetesResourcesLabelsAsTags != nil {
		kubernetesResourceLabelsAsTags, err := json.Marshal(config.KubernetesResourcesLabelsAsTags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDKubernetesResourcesLabelsAsTags,
				Value: string(kubernetesResourceLabelsAsTags),
			})
		}
	}

	// Provide a mapping of Kubernetes Resource Annotations to Datadog Tags.
	if config.KubernetesResourcesLabelsAsTags != nil {
		kubernetesResourceAnnotationsAsTags, err := json.Marshal(config.KubernetesResourcesAnnotationsAsTags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDKubernetesResourcesAnnotationsAsTags,
				Value: string(kubernetesResourceAnnotationsAsTags),
			})
		}
	}

	if componentName == v2alpha1.ClusterAgentComponentName {
		if err := resourcesManager.RBACManager().AddClusterPolicyRules(
			dda.Namespace,
			componentdca.GetResourceMetadataAsTagsClusterRoleName(dda),
			v2alpha1.GetClusterAgentServiceAccount(dda),
			getKubernetesResourceMetadataAsTagsPolicyRules(config.KubernetesResourcesLabelsAsTags, config.KubernetesResourcesAnnotationsAsTags),
		); err != nil {
			logger.Error(err, "error adding kubernetes resource metadata as tags clusterrole and clusterrolebinding to store")
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
				if singleContainerStrategyEnabled {
					manager.VolumeMount().AddVolumeMountToContainers(
						&kubeletVolMount,
						[]apicommon.AgentContainerName{
							apicommon.UnprivilegedSingleAgentContainerName,
						},
					)
					manager.Volume().AddVolume(&kubeletVol)
				} else {
					manager.VolumeMount().AddVolumeMountToContainers(
						&kubeletVolMount,
						[]apicommon.AgentContainerName{
							apicommon.CoreAgentContainerName,
							apicommon.ProcessAgentContainerName,
							apicommon.TraceAgentContainerName,
							apicommon.SecurityAgentContainerName,
							apicommon.AgentDataPlaneContainerName,
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

			if singleContainerStrategyEnabled {
				manager.VolumeMount().AddVolumeMountToContainers(
					&runtimeVolMount,
					[]apicommon.AgentContainerName{
						apicommon.UnprivilegedSingleAgentContainerName,
					},
				)
				manager.Volume().AddVolume(&runtimeVol)
			} else {
				manager.VolumeMount().AddVolumeMountToContainers(
					&runtimeVolMount,
					[]apicommon.AgentContainerName{
						apicommon.CoreAgentContainerName,
						apicommon.ProcessAgentContainerName,
						apicommon.TraceAgentContainerName,
						apicommon.SecurityAgentContainerName,
						apicommon.AgentDataPlaneContainerName,
					},
				)
				manager.Volume().AddVolume(&runtimeVol)
			}
		}
	}

	// Apply SecretBackend config
	if config.SecretBackend != nil {
		// Set secret backend command
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSecretBackendCommand,
			Value: apiutils.StringValue(config.SecretBackend.Command),
		})

		// Set secret backend arguments
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSecretBackendArguments,
			Value: apiutils.StringValue(config.SecretBackend.Args),
		})

		// Set secret backend timeout
		if config.SecretBackend.Timeout != nil {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDSecretBackendTimeout,
				Value: strconv.FormatInt(int64(*config.SecretBackend.Timeout), 10),
			})
		}

		var componentSaName string
		switch componentName {
		case v2alpha1.ClusterAgentComponentName:
			componentSaName = v2alpha1.GetClusterAgentServiceAccount(dda)
		case v2alpha1.NodeAgentComponentName:
			componentSaName = v2alpha1.GetAgentServiceAccount(dda)
		case v2alpha1.ClusterChecksRunnerComponentName:
			componentSaName = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)
		}

		agentName := dda.GetName()
		agentNs := dda.GetNamespace()
		rbacSuffix := "secret-backend"

		// Set global RBAC config (only if specific roles are not defined)
		if apiutils.BoolValue(config.SecretBackend.EnableGlobalPermissions) && config.SecretBackend.Roles == nil {

			var secretBackendGlobalRBACPolicyRules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{rbac.CoreAPIGroup},
					Resources: []string{rbac.SecretsResource},
					Verbs:     []string{rbac.GetVerb},
				},
			}

			roleName := fmt.Sprintf("%s-%s-%s", agentNs, agentName, rbacSuffix)

			if err := resourcesManager.RBACManager().AddClusterPolicyRules(agentNs, roleName, componentSaName, secretBackendGlobalRBACPolicyRules); err != nil {
				logger.Error(err, "Error adding cluster-wide secrets RBAC policy")
			}
		}

		// Set specific roles for the secret backend
		if config.SecretBackend.Roles != nil {
			for _, role := range config.SecretBackend.Roles {
				secretNs := apiutils.StringValue(role.Namespace)
				roleName := fmt.Sprintf("%s-%s-%s", secretNs, agentName, rbacSuffix)
				policyRule := []rbacv1.PolicyRule{
					{
						APIGroups:     []string{rbac.CoreAPIGroup},
						Resources:     []string{rbac.SecretsResource},
						ResourceNames: role.Secrets,
						Verbs:         []string{rbac.GetVerb},
					},
				}
				if err := resourcesManager.RBACManager().AddPolicyRules(secretNs, roleName, componentSaName, policyRule, agentNs); err != nil {
					logger.Error(err, "Error adding secrets RBAC policy")
				}
			}
		}
	}

	// Apply FIPS config
	if config.FIPS != nil && apiutils.BoolValue(config.FIPS.Enabled) {
		applyFIPSConfig(logger, manager, dda, resourcesManager)
	}

	return manager.PodTemplateSpec()
}
