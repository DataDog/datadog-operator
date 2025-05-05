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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/agent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/objects"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func addDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) []error {
	var errs []error
	// Install info
	if err := addInstallInfoDependencies(logger, dda, manager); err != nil {
		errs = append(errs, err)
	}

	// Credentials
	if err := addCredentialDependencies(logger, dda, manager); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func addComponentDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers, componentName v2alpha1.ComponentName, rc feature.RequiredComponent) []error {
	var errs []error

	if componentName == v2alpha1.ClusterAgentComponentName {
		if err := addDCATokenDependencies(logger, dda, manager); err != nil {
			errs = append(errs, err)
		}

		// Resources as tags
		if err := resourcesAsTagsDependencies(logger, dda, manager); err != nil {
			errs = append(errs, err)
		}
	}

	if componentName == v2alpha1.NodeAgentComponentName {
		// Create a configmap for the default seccomp profile in the System Probe.
		// This is mounted in the init-volume container in the agent default code.
		for _, containerName := range rc.Containers {
			if containerName == apicommon.SystemProbeContainerName {
				if !useSystemProbeCustomSeccomp(dda) {
					errs = append(errs, manager.ConfigMapManager().AddConfigMap(
						common.GetDefaultSeccompConfigMapName(dda),
						dda.GetNamespace(),
						agent.DefaultSeccompConfigDataForSystemProbe(),
					))
				}
			}
		}
	}

	// RBAC
	if err := rbacDependencies(logger, dda, manager, componentName); err != nil {
		errs = append(errs, err)
	}

	// Network policy
	if err := addNetworkPolicyDependencies(dda, manager, componentName); err != nil {
		errs = append(errs, err)
	}

	// Secret backend
	if err := addSecretBackendDependencies(logger, dda, manager, componentName); err != nil {
		errs = append(errs, err)
	}

	return errs
}

func addInstallInfoDependencies(logger logr.Logger, dda metav1.Object, manager feature.ResourceManagers) error {
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

func addCredentialDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
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
		if err := manager.SecretManager().AddSecret(dda.Namespace, secretName, v2alpha1.DefaultAPIKeyKey, *global.Credentials.APIKey); err != nil {
			logger.Error(err, "Error adding api key secret")
		}
	}

	// Create app key secret
	// App key is optional
	if !appKeySecretValid {
		if global.Credentials.AppKey != nil && *global.Credentials.AppKey != "" {
			if err := manager.SecretManager().AddSecret(dda.Namespace, secretName, v2alpha1.DefaultAPPKeyKey, *global.Credentials.AppKey); err != nil {
				logger.Error(err, "Error adding app key secret")
			}
		}
	}

	return nil
}

func addDCATokenDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
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
			logger.V(2).Info("built Cluster Agent token hash", "hash", hash)
		}
	} else if dda.Status.ClusterAgent == nil || dda.Status.ClusterAgent.GeneratedToken == "" { // no token specified
		token = apiutils.GenerateRandomString(32)
	} else {
		token = dda.Status.ClusterAgent.GeneratedToken // token already generated
	}

	// Create secret
	secretName := secrets.GetDefaultDCATokenSecretName(dda)
	if err := manager.SecretManager().AddSecret(dda.Namespace, secretName, common.DefaultTokenKey, token); err != nil {
		logger.Error(err, "Error adding dca token secret")
	}

	if key != "" && hash != "" {
		// Add annotation to secret
		if err := manager.SecretManager().AddAnnotations(logger, dda.Namespace, secretName, map[string]string{key: hash}); err != nil {
			logger.Error(err, "Error adding dca token secret annotations")
		}
	}

	return nil
}

func rbacDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers, componentName v2alpha1.ComponentName) error {
	switch componentName {
	case v2alpha1.ClusterAgentComponentName:
		return clusterAgentDependencies(logger, dda, manager)
	case v2alpha1.NodeAgentComponentName:
		return nodeAgentDependencies(logger, dda, manager)
	case v2alpha1.ClusterChecksRunnerComponentName:
		return clusterChecksRunnerDependencies(logger, dda, manager)
	}

	return nil
}

func clusterAgentDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
	var errs []error
	serviceAccountName := constants.GetClusterAgentServiceAccount(dda)
	rbacResourcesName := clusteragent.GetClusterAgentRbacResourcesName(dda)

	// Service account
	if err := manager.RBACManager().AddServiceAccountByComponent(dda.Namespace, serviceAccountName, string(v2alpha1.ClusterAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// Role Creation
	if err := manager.RBACManager().AddPolicyRulesByComponent(dda.Namespace, rbacResourcesName, serviceAccountName, clusteragent.GetDefaultClusterAgentRolePolicyRules(dda), string(v2alpha1.ClusterAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// ClusterRole creation
	if err := manager.RBACManager().AddClusterPolicyRulesByComponent(dda.Namespace, rbacResourcesName, serviceAccountName, clusteragent.GetDefaultClusterAgentClusterRolePolicyRules(dda), string(v2alpha1.ClusterAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// Service
	if err := manager.Store().AddOrUpdate(kubernetes.ServicesKind, clusteragent.GetClusterAgentService(dda)); err != nil {
		errs = append(errs, err)
	}

	return nil
}

func nodeAgentDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
	var errs []error
	serviceAccountName := constants.GetAgentServiceAccount(dda)
	rbacResourcesName := agent.GetAgentRoleName(dda)

	// Service account
	if err := manager.RBACManager().AddServiceAccountByComponent(dda.Namespace, serviceAccountName, string(v2alpha1.NodeAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	// ClusterRole creation
	if err := manager.RBACManager().AddClusterPolicyRulesByComponent(dda.Namespace, rbacResourcesName, serviceAccountName, agent.GetDefaultAgentClusterRolePolicyRules(disableNonResourceRules(dda)), string(v2alpha1.NodeAgentComponentName)); err != nil {
		errs = append(errs, err)
	}

	return nil
}

func clusterChecksRunnerDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
	var errs []error
	serviceAccountName := constants.GetClusterChecksRunnerServiceAccount(dda)
	rbacResourcesName := clusterchecksrunner.GetCCRRbacResourcesName(dda)

	// Service account
	if err := manager.RBACManager().AddServiceAccountByComponent(dda.Namespace, serviceAccountName, string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
		errs = append(errs, err)
	}

	// ClusterRole creation
	if err := manager.RBACManager().AddClusterPolicyRulesByComponent(dda.Namespace, rbacResourcesName, serviceAccountName, clusterchecksrunner.GetDefaultClusterChecksRunnerClusterRolePolicyRules(dda, disableNonResourceRules(dda)), string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
		errs = append(errs, err)
	}

	return nil
}

func addNetworkPolicyDependencies(dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers, componentName v2alpha1.ComponentName) error {
	config := dda.Spec.Global
	if enabled, flavor := constants.IsNetworkPolicyEnabled(dda); enabled {
		switch flavor {
		case v2alpha1.NetworkPolicyFlavorKubernetes:
			return manager.NetworkPolicyManager().AddKubernetesNetworkPolicy(objects.BuildKubernetesNetworkPolicy(dda, componentName))
		case v2alpha1.NetworkPolicyFlavorCilium:
			var dnsSelectorEndpoints []metav1.LabelSelector
			if config.NetworkPolicy.DNSSelectorEndpoints != nil {
				dnsSelectorEndpoints = config.NetworkPolicy.DNSSelectorEndpoints
			}
			return manager.CiliumPolicyManager().AddCiliumPolicy(
				objects.BuildCiliumPolicy(
					dda,
					*config.Site,
					getURLEndpoint(dda),
					constants.IsHostNetworkEnabled(dda, v2alpha1.ClusterAgentComponentName),
					dnsSelectorEndpoints,
					componentName,
				),
			)
		}
	}

	return nil
}

func addSecretBackendDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers, component v2alpha1.ComponentName) error {
	global := dda.Spec.Global
	if global.SecretBackend != nil {
		var componentSaName string
		switch component {
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

func disableNonResourceRules(dda *v2alpha1.DatadogAgent) bool {
	return dda.Spec.Global != nil && dda.Spec.Global.DisableNonResourceRules != nil && *dda.Spec.Global.DisableNonResourceRules
}

func resourcesAsTagsDependencies(logger logr.Logger, dda *v2alpha1.DatadogAgent, manager feature.ResourceManagers) error {
	global := dda.Spec.Global

	if len(global.KubernetesResourcesLabelsAsTags) > 0 || len(global.KubernetesResourcesAnnotationsAsTags) > 0 {
		if err := manager.RBACManager().AddClusterPolicyRules(
			dda.Namespace,
			clusteragent.GetResourceMetadataAsTagsClusterRoleName(dda),
			constants.GetClusterAgentServiceAccount(dda),
			clusteragent.GetKubernetesResourceMetadataAsTagsPolicyRules(global.KubernetesResourcesLabelsAsTags, global.KubernetesResourcesAnnotationsAsTags),
		); err != nil {
			return err
		}
	}
	return nil
}
