// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package override

import (
	"encoding/json"
	"fmt"
	"path"
	"path/filepath"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/defaulting"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func ApplyGlobalSettingsClusterAgent(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, requiredComponents feature.RequiredComponents) *corev1.PodTemplateSpec {
	return applyGlobalSettings(logger, manager, dda, resourcesManager, v2alpha1.ClusterAgentComponentName, false, requiredComponents)
}

func ApplyGlobalSettingsClusterChecksRunner(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, requiredComponents feature.RequiredComponents) *corev1.PodTemplateSpec {
	return applyGlobalSettings(logger, manager, dda, resourcesManager, v2alpha1.ClusterChecksRunnerComponentName, false, requiredComponents)
}

func ApplyGlobalSettingsNodeAgent(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, singleContainerStrategyEnabled bool, requiredComponents feature.RequiredComponents) *corev1.PodTemplateSpec {
	return applyGlobalSettings(logger, manager, dda, resourcesManager, v2alpha1.NodeAgentComponentName, singleContainerStrategyEnabled, requiredComponents)
}

// ApplyGlobalSettings use to apply global setting to a PodTemplateSpec
func applyGlobalSettings(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers,
	componentName v2alpha1.ComponentName, singleContainerStrategyEnabled bool, requiredComponents feature.RequiredComponents) *corev1.PodTemplateSpec {
	config := dda.Spec.Global

	// ClusterName sets a unique cluster name for the deployment to easily scope monitoring data in the Datadog app.
	if config.ClusterName != nil {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  constants.DDClusterName,
			Value: *config.ClusterName,
		})
	}

	// Site is the Datadog intake site Agent data are sent to.
	manager.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  constants.DDSite,
		Value: *config.Site,
	})

	// Endpoint is the Datadog intake URL the Agent data are sent to.
	if config.Endpoint != nil && config.Endpoint.URL != nil {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  constants.DDddURL,
			Value: *config.Endpoint.URL,
		})
	}

	// Registry is the image registry to use for all Agent images.
	if *config.Registry != defaulting.DefaultImageRegistry {
		image := defaulting.DefaultAgentImageName
		version := defaulting.AgentLatestVersion
		if componentName == v2alpha1.ClusterAgentComponentName {
			image = defaulting.DefaultClusterAgentImageName
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
		Name:  DDLogLevel,
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
						constants.IsHostNetworkEnabled(dda, v2alpha1.ClusterAgentComponentName),
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
				Name:  DDTags,
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

	if config.OriginDetectionUnified != nil && config.OriginDetectionUnified.Enabled != nil {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  DDOriginDetectionUnified,
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
				Name:  DDPodLabelsAsTags,
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
				Name:  DDPodAnnotationsAsTags,
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
				Name:  DDNodeLabelsAsTags,
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
				Name:  DDNamespaceLabelsAsTags,
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
				Name:  DDNamespaceAnnotationsAsTags,
				Value: string(namespaceAnnotationsAsTags),
			})
		}
	}

	if componentName == v2alpha1.NodeAgentComponentName {
		// Kubelet contains the kubelet configuration parameters.
		// The environment variable `DD_KUBERNETES_KUBELET_HOST` defaults to `status.hostIP` if not overriden.
		if config.Kubelet != nil {
			if config.Kubelet.Host != nil {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:      common.DDKubeletHost,
					ValueFrom: config.Kubelet.Host,
				})
			}
			if config.Kubelet.TLSVerify != nil {
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  DDKubeletTLSVerify,
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
					agentCAPath = common.KubeletAgentCAPath
				}
				kubeletVol, kubeletVolMount := volume.GetVolumes(kubeletCAVolumeName, config.Kubelet.HostCAPath, agentCAPath, true)
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
					Name:  DDKubeletCAPath,
					Value: agentCAPath,
				})
			}
			if config.Kubelet.PodResourcesSocketPath != "" {
				manager.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
					Name:  DDKubernetesPodResourcesSocket,
					Value: path.Join(config.Kubelet.PodResourcesSocketPath, "kubelet.sock"),
				})

				podResourcesVol, podResourcesMount := volume.GetVolumes(common.KubeletPodResourcesVolumeName, config.Kubelet.PodResourcesSocketPath, config.Kubelet.PodResourcesSocketPath, false)
				if singleContainerStrategyEnabled {
					manager.VolumeMount().AddVolumeMountToContainer(
						&podResourcesMount,
						apicommon.UnprivilegedSingleAgentContainerName,
					)
					manager.Volume().AddVolume(&podResourcesVol)
				} else {
					manager.VolumeMount().AddVolumeMountToContainer(
						&podResourcesMount,
						apicommon.CoreAgentContainerName,
					)
					manager.Volume().AddVolume(&podResourcesVol)
				}
			}
			// Configure checks tag cardinality if provided
			if config.ChecksTagCardinality != nil {
				// The value validation happens at the Agent level - if the lower(string) is not `low`, `orchestrator` or `high`, the Agent defaults to `low`.
				// Ref: https://github.com/DataDog/datadog-agent/blob/1d08a6a9783fe271ea3813ddf9abf60244abdf2c/comp/core/tagger/taggerimpl/tagger.go#L173-L177
				manager.EnvVar().AddEnvVar(&corev1.EnvVar{
					Name:  DDChecksTagCardinality,
					Value: *config.ChecksTagCardinality,
				})
			}
		}

		var runtimeVol corev1.Volume
		var runtimeVolMount corev1.VolumeMount
		// Path to the docker runtime socket.
		if config.DockerSocketPath != nil {
			dockerMountPath := filepath.Join(common.HostCriSocketPathPrefix, *config.DockerSocketPath)
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DockerHost,
				Value: "unix://" + dockerMountPath,
			})
			runtimeVol, runtimeVolMount = volume.GetVolumes(common.CriSocketVolumeName, *config.DockerSocketPath, dockerMountPath, true)
		} else if config.CriSocketPath != nil {
			// Path to the container runtime socket (if different from Docker).
			criSocketMountPath := filepath.Join(common.HostCriSocketPathPrefix, *config.CriSocketPath)
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDCriSocketPath,
				Value: criSocketMountPath,
			})
			runtimeVol, runtimeVolMount = volume.GetVolumes(common.CriSocketVolumeName, *config.CriSocketPath, criSocketMountPath, true)
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

	// Credentials
	if err := handleCredentials(dda, resourcesManager, manager); err != nil {
		logger.Error(err, "Failed to create API and/or APP keys")
	}

	// DCA token
	if requiredComponents.ClusterAgent.IsEnabled() {
		// dca token
		if err := handleDCAToken(logger, dda, resourcesManager, manager); err != nil {
			logger.Error(err, "Failed to create DCA token")
		}
	}

	// Apply SecretBackend config
	if config.SecretBackend != nil {
		// Set secret backend command
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  DDSecretBackendCommand,
			Value: apiutils.StringValue(config.SecretBackend.Command),
		})

		// Set secret backend arguments
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  DDSecretBackendArguments,
			Value: apiutils.StringValue(config.SecretBackend.Args),
		})

		// Set secret backend timeout
		if config.SecretBackend.Timeout != nil {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDSecretBackendTimeout,
				Value: strconv.FormatInt(int64(*config.SecretBackend.Timeout), 10),
			})
		}

		var componentSaName string
		switch componentName {
		case v2alpha1.ClusterAgentComponentName:
			componentSaName = constants.GetClusterAgentServiceAccount(dda)
		case v2alpha1.NodeAgentComponentName:
			componentSaName = constants.GetAgentServiceAccount(dda)
		case v2alpha1.ClusterChecksRunnerComponentName:
			componentSaName = constants.GetClusterChecksRunnerServiceAccount(dda)
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

// handleCredentials will be split between dependency and pod changes when global is refactored
func handleCredentials(dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, podTemplateManager feature.PodTemplateManagers) error {
	if err := credentialDependencies(dda, resourcesManager); err != nil {
		return err
	}
	credentialResource(dda, podTemplateManager)
	return nil
}

func credentialDependencies(dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) error {
	// Prioritize existing secrets
	// Credentials should be non-nil from validation
	global := dda.Spec.Global
	apiKeySecretValid := isValidSecretConfig(global.Credentials.APISecret)
	appKeySecretValid := isValidSecretConfig(global.Credentials.AppSecret)

	// User defined secret(s) exist for both keys, nothing to do
	if apiKeySecretValid && appKeySecretValid {
		return nil
	}

	// Secret needs to be created for at least one key
	secretName := secrets.GetDefaultCredentialsSecretName(dda)
	// Create API key secret
	if !apiKeySecretValid {
		if global.Credentials.APIKey == nil || *global.Credentials.APIKey == "" {
			return fmt.Errorf("api key must be set")
		}
		if err := resourcesManager.SecretManager().AddSecret(dda.Namespace, secretName, v2alpha1.DefaultAPIKeyKey, *global.Credentials.APIKey); err != nil {
			return err
		}
	}

	// Create app key secret
	// App key is optional
	if !appKeySecretValid {
		if global.Credentials.AppKey != nil && *global.Credentials.AppKey != "" {
			if err := resourcesManager.SecretManager().AddSecret(dda.Namespace, secretName, v2alpha1.DefaultAPPKeyKey, *global.Credentials.AppKey); err != nil {
				return err
			}
		}
	}

	return nil
}

func credentialResource(dda *v2alpha1.DatadogAgent, podTemplateManager feature.PodTemplateManagers) {
	// Default credential names
	defaultSecretName := secrets.GetDefaultCredentialsSecretName(dda)
	apiKeySecretName := defaultSecretName
	appKeySecretName := ""
	apiKeySecretKey := v2alpha1.DefaultAPIKeyKey
	appKeySecretKey := v2alpha1.DefaultAPPKeyKey

	global := dda.Spec.Global
	// App key is optional
	if appKey := apiutils.StringValue(global.Credentials.AppKey); appKey != "" {
		appKeySecretName = defaultSecretName
	}

	// User specified names
	if isValidSecretConfig(global.Credentials.APISecret) {
		apiKeySecretName = global.Credentials.APISecret.SecretName
		apiKeySecretKey = global.Credentials.APISecret.KeyName
	}
	if isValidSecretConfig(global.Credentials.AppSecret) {
		appKeySecretName = global.Credentials.AppSecret.SecretName
		appKeySecretKey = global.Credentials.AppSecret.KeyName
	}

	// Add secret env vars to pod
	apiKeyEnvVar := common.BuildEnvVarFromSource(constants.DDAPIKey, common.BuildEnvVarFromSecret(apiKeySecretName, apiKeySecretKey))
	podTemplateManager.EnvVar().AddEnvVar(apiKeyEnvVar)

	if appKeySecretName != "" {
		appKeyEnvVar := common.BuildEnvVarFromSource(constants.DDAppKey, common.BuildEnvVarFromSecret(appKeySecretName, appKeySecretKey))
		podTemplateManager.EnvVar().AddEnvVar(appKeyEnvVar)
	}
}

// handleDCAToken will be split between dependency and pod changes when global is refactored
func handleDCAToken(logger logr.Logger, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, podTemplateManager feature.PodTemplateManagers) error {
	if err := dcaTokenDependencies(logger, dda, resourcesManager); err != nil {
		return err
	}
	dcaTokenResource(logger, dda, resourcesManager, podTemplateManager)
	return nil
}

func dcaTokenDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers) error {
	global := dda.Spec.Global
	var token string

	// Prioritize existing secret
	if isValidSecretConfig(global.ClusterAgentTokenSecret) {
		return nil
	}

	// User specifies token
	var key string
	var hash string
	var err error
	if global.ClusterAgentToken != nil && *global.ClusterAgentToken != "" {
		token = *global.ClusterAgentToken
		// Generate hash
		key = getDCATokenChecksumAnnotationKey()
		hash, err = comparison.GenerateMD5ForSpec(map[string]string{common.DefaultTokenKey: token})
		if err != nil {
			logger.Error(err, "couldn't generate hash for Cluster Agent token hash")
		} else {
			logger.Info("built Cluster Agent token hash", "hash", hash)
			// logger.V(2).Info("built Cluster Agent token hash", "hash", hash)
		}
	} else if dda.Status.ClusterAgent == nil || dda.Status.ClusterAgent.GeneratedToken == "" { // no token specified
		token = apiutils.GenerateRandomString(32)
	} else {
		token = dda.Status.ClusterAgent.GeneratedToken // token already generated
	}

	// Create secret
	secretName := secrets.GetDefaultDCATokenSecretName(dda)
	if err := resourcesManager.SecretManager().AddSecret(dda.Namespace, secretName, common.DefaultTokenKey, token); err != nil {
		return err
	}

	if key != "" && hash != "" {
		// Add annotation to secret
		if err := resourcesManager.SecretManager().AddAnnotations(logger, dda.Namespace, secretName, map[string]string{key: hash}); err != nil {
			return err
		}
	}

	return nil
}

func dcaTokenResource(logger logr.Logger, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, podTemplateManager feature.PodTemplateManagers) {
	secretName := secrets.GetDefaultDCATokenSecretName(dda)
	secretKey := common.DefaultTokenKey

	global := dda.Spec.Global
	if isValidSecretConfig(global.ClusterAgentTokenSecret) {
		secretName = global.ClusterAgentTokenSecret.SecretName
		secretKey = global.ClusterAgentTokenSecret.KeyName
	}
	// Add secret env var to pod
	tokenEnvVar := common.BuildEnvVarFromSource(DDClusterAgentAuthToken, common.BuildEnvVarFromSecret(secretName, secretKey))
	podTemplateManager.EnvVar().AddEnvVar(tokenEnvVar)

	// Add annotation to pod template if secret has annotation
	if obj, exists := resourcesManager.Store().Get(kubernetes.SecretsKind, dda.Namespace, secretName); exists {
		key := getDCATokenChecksumAnnotationKey()
		if val, ok := obj.GetAnnotations()[key]; ok {
			podTemplateManager.Annotation().AddAnnotation(key, val)
		}
	}
}

func isValidSecretConfig(secretConfig *v2alpha1.SecretConfig) bool {
	if secretConfig == nil {
		return false
	}
	if secretConfig.SecretName == "" || secretConfig.KeyName == "" {
		return false
	}

	return true
}
