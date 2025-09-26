// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package global

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func addDependencies(logger logr.Logger, ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers, fromDDAI bool) []error {
	var errs []error
	// APM Telemetry and Credentials are managed by DDA controller (manageDDADependenciesWithDDAI).
	if !fromDDAI {
		// Install info
		if err := AddInstallInfoDependencies(ddaMeta, manager); err != nil {
			errs = append(errs, err)
		}
		// APM Telemetry
		if err := AddAPMTelemetryDependencies(logger, ddaMeta, manager); err != nil {
			errs = append(errs, err)
		}
		// Credentials
		if err := AddCredentialDependencies(logger, ddaMeta, ddaSpec, manager); err != nil {
			errs = append(errs, err)
		}
	}

	return errs
}

func addComponentDependencies(logger logr.Logger, ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaStatus *v2alpha1.DatadogAgentStatus, manager feature.ResourceManagers, componentName v2alpha1.ComponentName, rc feature.RequiredComponent, fromDDAI bool) []error {
	var errs []error

	if componentName == v2alpha1.ClusterAgentComponentName {
		// DCA token is solely managed by DDA controller.
		if !fromDDAI {
			if err := AddDCATokenDependencies(logger, ddaMeta, ddaSpec, ddaStatus, manager); err != nil {
				errs = append(errs, err)
			}
		}

		// Resources as tags
		if err := resourcesAsTagsDependencies(ddaMeta, ddaSpec, manager); err != nil {
			errs = append(errs, err)
		}
	}

	if componentName == v2alpha1.NodeAgentComponentName {
		// Creates / updates system-probe-seccomp configMap to configData or default
		for _, containerName := range rc.Containers {
			if containerName == apicommon.SystemProbeContainerName {
				var seccompConfigData map[string]string
				useCustomSeccompConfigData := false

				if componentOverride, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
					if spContainer, ok := componentOverride.Containers[apicommon.SystemProbeContainerName]; ok {
						if useSystemProbeCustomSeccomp(ddaSpec) && spContainer.SeccompConfig.CustomProfile.ConfigMap != nil {
							break
						}
						if useSystemProbeCustomSeccomp(ddaSpec) && spContainer.SeccompConfig.CustomProfile.ConfigData != nil {
							seccompConfigData = map[string]string{
								common.SystemProbeSeccompKey: *spContainer.SeccompConfig.CustomProfile.ConfigData,
							}
							useCustomSeccompConfigData = true
						}
					}
				}
				if seccompConfigData == nil {
					if !useSystemProbeCustomSeccomp(ddaSpec) {
						seccompConfigData = agent.DefaultSeccompConfigDataForSystemProbe()
					}
				}
				if seccompConfigData != nil {
					err := manager.ConfigMapManager().AddConfigMap(
						common.GetDefaultSeccompConfigMapName(ddaMeta),
						ddaMeta.GetNamespace(),
						seccompConfigData,
					)

					if err == nil && useSystemProbeCustomSeccomp(ddaSpec) && useCustomSeccompConfigData {
						// Add checksum annotation to the configMap
						if seccompCM, ok := manager.Store().Get(kubernetes.ConfigMapKind, ddaMeta.GetNamespace(), common.GetDefaultSeccompConfigMapName(ddaMeta)); ok {
							configHash, _ := comparison.GenerateMD5ForSpec(seccompConfigData)
							annotations := object.MergeAnnotationsLabels(logger, seccompCM.GetAnnotations(), map[string]string{object.GetChecksumAnnotationKey(common.SystemProbeSeccompKey): configHash}, "*")
							seccompCM.SetAnnotations(annotations)
						}
					}
					errs = append(errs, err)
				}
			}
		}
	}

	// RBAC
	if err := rbacDependencies(ddaMeta, ddaSpec, manager, componentName, fromDDAI); err != nil {
		errs = append(errs, err)
	}

	// Network policy
	if err := addNetworkPolicyDependencies(ddaMeta, ddaSpec, manager, componentName); err != nil {
		errs = append(errs, err)
	}

	// Secret backend
	if err := addSecretBackendDependencies(logger, ddaMeta, ddaSpec, manager, componentName); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func AddInstallInfoDependencies(dda metav1.Object, manager feature.ResourceManagers) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.GetInstallInfoConfigMapName(dda),
			Namespace: dda.GetNamespace(),
		},
		Data: map[string]string{
			"install_info": getInstallInfoValue(),
		},
	}

	if err := manager.Store().AddOrUpdate(kubernetes.ConfigMapKind, configMap); err != nil {
		return err
	}

	return nil
}

func AddAPMTelemetryDependencies(_ logr.Logger, dda metav1.Object, manager feature.ResourceManagers) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.APMTelemetryConfigMapName,
			Namespace: dda.GetNamespace(),
		},
		Data: map[string]string{
			common.APMTelemetryInstallTypeKey: common.DefaultAgentInstallType,
			common.APMTelemetryInstallIdKey:   utils.GetDatadogAgentResourceUID(dda),
			common.APMTelemetryInstallTimeKey: utils.GetDatadogAgentResourceCreationTime(dda),
		},
	}

	if err := manager.Store().AddOrUpdate(kubernetes.ConfigMapKind, configMap); err != nil {
		return err
	}

	return nil
}

func AddCredentialDependencies(logger logr.Logger, ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers) error {
	// Prioritize existing secrets
	// Credentials should be non-nil from validation
	global := ddaSpec.Global
	apiKeySecretValid := IsValidSecretConfig(global.Credentials.APISecret)
	appKeySecretValid := IsValidSecretConfig(global.Credentials.AppSecret)

	// User defined secret(s) exist for both keys, nothing to do
	if apiKeySecretValid && appKeySecretValid {
		return nil
	}

	// Secret needs to be created for at least one key
	secretName := secrets.GetDefaultCredentialsSecretName(ddaMeta)
	// Create API key secret
	if !apiKeySecretValid {
		if global.Credentials.APIKey == nil || *global.Credentials.APIKey == "" {
			return fmt.Errorf("api key must be set")
		}
		if err := manager.SecretManager().AddSecret(ddaMeta.GetNamespace(), secretName, v2alpha1.DefaultAPIKeyKey, *global.Credentials.APIKey); err != nil {
			logger.Error(err, "Error adding api key secret")
		}
	}

	// Create app key secret
	// App key is optional
	if !appKeySecretValid {
		if global.Credentials.AppKey != nil && *global.Credentials.AppKey != "" {
			if err := manager.SecretManager().AddSecret(ddaMeta.GetNamespace(), secretName, v2alpha1.DefaultAPPKeyKey, *global.Credentials.AppKey); err != nil {
				logger.Error(err, "Error adding app key secret")
			}
		}
	}

	return nil
}

func AddDCATokenDependencies(logger logr.Logger, ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaStatus *v2alpha1.DatadogAgentStatus, manager feature.ResourceManagers) error {
	global := ddaSpec.Global
	var token string

	// Prioritize existing secret
	if IsValidSecretConfig(global.ClusterAgentTokenSecret) {
		return nil
	}

	// User specifies token
	var key string
	var hash string
	var err error
	if global.ClusterAgentToken != nil && *global.ClusterAgentToken != "" {
		token = *global.ClusterAgentToken
		// Generate hash
		key = GetDCATokenChecksumAnnotationKey()
		hash, err = comparison.GenerateMD5ForSpec(map[string]string{common.DefaultTokenKey: token})
		if err != nil {
			logger.Error(err, "couldn't generate hash for Cluster Agent token hash")
		} else {
			logger.V(2).Info("built Cluster Agent token hash", "hash", hash)
		}
	} else if ddaStatus.ClusterAgent == nil || ddaStatus.ClusterAgent.GeneratedToken == "" { // no token specified
		token = apiutils.GenerateRandomString(32)
	} else {
		token = ddaStatus.ClusterAgent.GeneratedToken // token already generated
	}

	// Create secret
	secretName := secrets.GetDefaultDCATokenSecretName(ddaMeta)
	if err := manager.SecretManager().AddSecret(ddaMeta.GetNamespace(), secretName, common.DefaultTokenKey, token); err != nil {
		logger.Error(err, "Error adding dca token secret")
	}

	if key != "" && hash != "" {
		// Add annotation to secret
		if err := manager.SecretManager().AddAnnotations(logger, ddaMeta.GetNamespace(), secretName, map[string]string{key: hash}); err != nil {
			logger.Error(err, "Error adding dca token secret annotations")
		}
	}

	return nil
}

func rbacDependencies(ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers, componentName v2alpha1.ComponentName, fromDDAI bool) error {
	switch componentName {
	case v2alpha1.ClusterAgentComponentName:
		return clusterAgentDependencies(ddaMeta, ddaSpec, manager, fromDDAI)
	case v2alpha1.NodeAgentComponentName:
		return nodeAgentDependencies(ddaMeta, ddaSpec, manager)
	case v2alpha1.ClusterChecksRunnerComponentName:
		return clusterChecksRunnerDependencies(ddaMeta, ddaSpec, manager)
	}

	return nil
}

func clusterAgentDependencies(ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers, fromDDAI bool) error {
	var errs []error
	serviceAccountName := constants.GetClusterAgentServiceAccount(ddaMeta.GetName(), ddaSpec)
	rbacResourcesName := clusteragent.GetClusterAgentRbacResourcesName(ddaMeta)

	// Service account
	if err := manager.RBACManager().AddServiceAccountByComponent(ddaMeta.GetNamespace(), serviceAccountName, string(v2alpha1.ClusterAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// Role Creation
	if err := manager.RBACManager().AddPolicyRulesByComponent(ddaMeta.GetNamespace(), rbacResourcesName, serviceAccountName, clusteragent.GetDefaultClusterAgentRolePolicyRules(ddaMeta), string(v2alpha1.ClusterAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// ClusterRole creation
	if err := manager.RBACManager().AddClusterPolicyRulesByComponent(ddaMeta.GetNamespace(), rbacResourcesName, serviceAccountName, clusteragent.GetDefaultClusterAgentClusterRolePolicyRules(ddaMeta), string(v2alpha1.ClusterAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// Service is managed by DDA controller (manageDDADependenciesWithDDAI).
	if !fromDDAI {
		// Service
		service := clusteragent.GetClusterAgentService(ddaMeta)
		if err := manager.ServiceManager().AddService(service.Name, service.Namespace, service.Spec.Selector, service.Spec.Ports, service.Spec.InternalTrafficPolicy); err != nil {
			errs = append(errs, err)
		}
	}

	return nil
}

func nodeAgentDependencies(ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers) error {
	var errs []error
	serviceAccountName := constants.GetAgentServiceAccount(ddaMeta.GetName(), ddaSpec)
	rbacResourcesName := agent.GetAgentRoleName(ddaMeta)
	useFineGrainedAuthorization := *ddaSpec.Global.Kubelet.FineGrainedAuthorization

	// Service account
	if err := manager.RBACManager().AddServiceAccountByComponent(ddaMeta.GetNamespace(), serviceAccountName, string(v2alpha1.NodeAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// ClusterRole creation
	if err := manager.RBACManager().AddClusterPolicyRulesByComponent(ddaMeta.GetNamespace(), rbacResourcesName, serviceAccountName, agent.GetDefaultAgentClusterRolePolicyRules(disableNonResourceRules(ddaSpec), useFineGrainedAuthorization), string(v2alpha1.NodeAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	return nil
}

func clusterChecksRunnerDependencies(ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers) error {
	var errs []error
	serviceAccountName := constants.GetClusterChecksRunnerServiceAccount(ddaMeta.GetName(), ddaSpec)
	rbacResourcesName := clusterchecksrunner.GetCCRRbacResourcesName(ddaMeta)

	// Service account
	if err := manager.RBACManager().AddServiceAccountByComponent(ddaMeta.GetNamespace(), serviceAccountName, string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
		errs = append(errs, err)
	}

	// ClusterRole creation
	if err := manager.RBACManager().AddClusterPolicyRulesByComponent(ddaMeta.GetNamespace(), rbacResourcesName, serviceAccountName, clusterchecksrunner.GetDefaultClusterChecksRunnerClusterRolePolicyRules(ddaMeta, disableNonResourceRules(ddaSpec)), string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
		errs = append(errs, err)
	}

	return nil
}

func addNetworkPolicyDependencies(ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers, componentName v2alpha1.ComponentName) error {
	config := ddaSpec.Global
	if enabled, flavor := constants.IsNetworkPolicyEnabled(ddaSpec); enabled {
		switch flavor {
		case v2alpha1.NetworkPolicyFlavorKubernetes:
			return manager.NetworkPolicyManager().AddKubernetesNetworkPolicy(objects.BuildKubernetesNetworkPolicy(ddaMeta, componentName))
		case v2alpha1.NetworkPolicyFlavorCilium:
			var dnsSelectorEndpoints []metav1.LabelSelector
			if config.NetworkPolicy.DNSSelectorEndpoints != nil {
				dnsSelectorEndpoints = config.NetworkPolicy.DNSSelectorEndpoints
			}
			return manager.CiliumPolicyManager().AddCiliumPolicy(
				objects.BuildCiliumPolicy(
					ddaMeta,
					*config.Site,
					getURLEndpoint(ddaSpec),
					constants.IsHostNetworkEnabled(ddaSpec, v2alpha1.ClusterAgentComponentName),
					dnsSelectorEndpoints,
					componentName,
				),
			)
		}
	}

	return nil
}

func addSecretBackendDependencies(logger logr.Logger, ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers, component v2alpha1.ComponentName) error {
	global := ddaSpec.Global
	if global.SecretBackend != nil {
		var componentSaName string
		switch component {
		case v2alpha1.ClusterAgentComponentName:
			componentSaName = constants.GetClusterAgentServiceAccount(ddaMeta.GetName(), ddaSpec)
		case v2alpha1.NodeAgentComponentName:
			componentSaName = constants.GetAgentServiceAccount(ddaMeta.GetName(), ddaSpec)
		case v2alpha1.ClusterChecksRunnerComponentName:
			componentSaName = constants.GetClusterChecksRunnerServiceAccount(ddaMeta.GetName(), ddaSpec)
		}

		agentName := ddaMeta.GetName()
		agentNs := ddaMeta.GetNamespace()
		rbacSuffix := "secret-backend"

		// Set global RBAC config (only if specific roles are not defined)
		if apiutils.BoolValue(global.SecretBackend.EnableGlobalPermissions) && global.SecretBackend.Roles == nil {
			var secretBackendGlobalRBACPolicyRules = []rbacv1.PolicyRule{
				{
					APIGroups: []string{rbac.CoreAPIGroup},
					Resources: []string{rbac.SecretsResource},
					Verbs:     []string{rbac.GetVerb},
				},
			}

			roleName := fmt.Sprintf("%s-%s-%s", agentNs, agentName, rbacSuffix)

			if err := manager.RBACManager().AddClusterPolicyRules(agentNs, roleName, componentSaName, secretBackendGlobalRBACPolicyRules); err != nil {
				logger.Error(err, "Error adding cluster-wide secrets RBAC policy")
			}
		}

		// Set specific roles for the secret backend
		if global.SecretBackend.Roles != nil {
			for _, role := range global.SecretBackend.Roles {
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
				if err := manager.RBACManager().AddPolicyRules(secretNs, roleName, componentSaName, policyRule, agentNs); err != nil {
					logger.Error(err, "Error adding secrets RBAC policy")
				}
			}
		}
	}

	return nil
}

func disableNonResourceRules(ddaSpec *v2alpha1.DatadogAgentSpec) bool {
	return ddaSpec.Global != nil && ddaSpec.Global.DisableNonResourceRules != nil && *ddaSpec.Global.DisableNonResourceRules
}

func resourcesAsTagsDependencies(ddaMeta metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, manager feature.ResourceManagers) error {
	global := ddaSpec.Global

	if len(global.KubernetesResourcesLabelsAsTags) > 0 || len(global.KubernetesResourcesAnnotationsAsTags) > 0 {
		if err := manager.RBACManager().AddClusterPolicyRules(
			ddaMeta.GetNamespace(),
			clusteragent.GetResourceMetadataAsTagsClusterRoleName(ddaMeta),
			constants.GetClusterAgentServiceAccount(ddaMeta.GetName(), ddaSpec),
			clusteragent.GetKubernetesResourceMetadataAsTagsPolicyRules(global.KubernetesResourcesLabelsAsTags, global.KubernetesResourcesAnnotationsAsTags),
		); err != nil {
			return err
		}
	}
	return nil
}
