// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"encoding/json"
	"strconv"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

// ApplyGlobalDependencies applies the global dependencies for a DatadogAgent instance.
func ApplyGlobalDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, resourceManagers feature.ResourceManagers) []error {
	return addDependencies(logger, dda, resourceManagers)
}

// ApplyGlobalComponentDependencies applies the global dependencies for a component.
func ApplyGlobalComponentDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, resourceManagers feature.ResourceManagers, componentName v2alpha1.ComponentName, rc feature.RequiredComponent) []error {
	if rc.IsEnabled() {
		return addComponentDependencies(logger, dda, resourceManagers, componentName, rc)
	}
	return nil
}

// ApplyGlobalSettingsClusterAgent applies the global settings for the ClusterAgent component.
func ApplyGlobalSettingsClusterAgent(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, requiredComponents feature.RequiredComponents) {
	applyGlobalSettings(logger, manager, dda, resourcesManager, requiredComponents)
	applyClusterAgentResources(manager, dda)
}

// ApplyGlobalSettingsClusterChecksRunner applies the global settings for the ClusterChecksRunner component.
func ApplyGlobalSettingsClusterChecksRunner(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, requiredComponents feature.RequiredComponents) {
	applyGlobalSettings(logger, manager, dda, resourcesManager, requiredComponents)
	applyClusterChecksRunnerResources(manager, dda)
}

// ApplyGlobalSettingsNodeAgent applies the global settings for the NodeAgent component.
func ApplyGlobalSettingsNodeAgent(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent,
	resourcesManager feature.ResourceManagers, singleContainerStrategyEnabled bool, requiredComponents feature.RequiredComponents, provider string) {
	applyGlobalSettings(logger, manager, dda, resourcesManager, requiredComponents)
	applyNodeAgentResources(manager, dda, singleContainerStrategyEnabled, provider)
}

// ApplyGlobalSettings use to apply global setting to a PodTemplateSpec
func applyGlobalSettings(logger logr.Logger, manager feature.PodTemplateManagers, dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, requiredComponents feature.RequiredComponents) {
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
	if ep := getURLEndpoint(dda); ep != "" {
		manager.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  constants.DDddURL,
			Value: ep,
		})
	}

	// LogLevel sets logging verbosity. This can be overridden by container.
	manager.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  constants.DDLogLevel,
		Value: *config.LogLevel,
	})

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

	// Resources as tags
	if len(config.KubernetesResourcesLabelsAsTags) > 0 {
		kubernetesResourceLabelsAsTags, err := json.Marshal(config.KubernetesResourcesLabelsAsTags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDKubernetesResourcesLabelsAsTags,
				Value: string(kubernetesResourceLabelsAsTags),
			})
		}
	}

	if len(config.KubernetesResourcesAnnotationsAsTags) > 0 {
		kubernetesResourceAnnotationsAsTags, err := json.Marshal(config.KubernetesResourcesAnnotationsAsTags)
		if err != nil {
			logger.Error(err, "Failed to unmarshal json input")
		} else {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDKubernetesResourcesAnnotationsAsTags,
				Value: string(kubernetesResourceAnnotationsAsTags),
			})
		}
	}

	// Credentials
	credentialResource(dda, manager)

	// DCA token
	if requiredComponents.ClusterAgent.IsEnabled() {
		dcaTokenResource(dda, resourcesManager, manager)
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

		// Set secret backend refresh interval
		if config.SecretBackend.RefreshInterval != nil && *config.SecretBackend.RefreshInterval > 0 {
			manager.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDSecretRefreshInterval,
				Value: strconv.FormatInt(int64(*config.SecretBackend.RefreshInterval), 10),
			})
		}
	}

	// Update images with Global Registry and UseFIPSAgent configurations
	updateContainerImages(config, manager)

	// Apply FIPS proxy settings - UseFIPSAgent must be false
	if !*config.UseFIPSAgent && config.FIPS != nil && apiutils.BoolValue(config.FIPS.Enabled) {
		applyFIPSConfig(logger, manager, dda, resourcesManager)
	}

}

func updateContainerImages(config *v2alpha1.GlobalConfig, podTemplateManager feature.PodTemplateManagers) {
	image := &images.Image{}
	for i, container := range podTemplateManager.PodTemplateSpec().Spec.Containers {
		image = images.FromString(container.Image).
			WithRegistry(*config.Registry).
			WithFIPS(*config.UseFIPSAgent)
		// Note: if an image tag override is configured, this image tag will be overwritten
		podTemplateManager.PodTemplateSpec().Spec.Containers[i].Image = image.ToString()
	}

	for i, container := range podTemplateManager.PodTemplateSpec().Spec.InitContainers {
		image = images.FromString(container.Image)
		image.WithRegistry(*config.Registry)
		image.WithFIPS(*config.UseFIPSAgent)
		// Note: if an image tag override is configured, this image tag will be overwritten
		podTemplateManager.PodTemplateSpec().Spec.InitContainers[i].Image = image.ToString()
	}
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
	if IsValidSecretConfig(global.Credentials.APISecret) {
		apiKeySecretName = global.Credentials.APISecret.SecretName
		apiKeySecretKey = global.Credentials.APISecret.KeyName
	}
	if IsValidSecretConfig(global.Credentials.AppSecret) {
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

func dcaTokenResource(dda *v2alpha1.DatadogAgent, resourcesManager feature.ResourceManagers, podTemplateManager feature.PodTemplateManagers) {
	secretName := secrets.GetDefaultDCATokenSecretName(dda)
	secretKey := common.DefaultTokenKey

	global := dda.Spec.Global
	if IsValidSecretConfig(global.ClusterAgentTokenSecret) {
		secretName = global.ClusterAgentTokenSecret.SecretName
		secretKey = global.ClusterAgentTokenSecret.KeyName
	}
	// Add secret env var to pod
	tokenEnvVar := common.BuildEnvVarFromSource(DDClusterAgentAuthToken, common.BuildEnvVarFromSecret(secretName, secretKey))
	podTemplateManager.EnvVar().AddEnvVar(tokenEnvVar)

	// Add annotation to pod template if secret has annotation
	if obj, exists := resourcesManager.Store().Get(kubernetes.SecretsKind, dda.Namespace, secretName); exists {
		key := GetDCATokenChecksumAnnotationKey()
		if val, ok := obj.GetAnnotations()[key]; ok {
			podTemplateManager.Annotation().AddAnnotation(key, val)
		}
	}
}
