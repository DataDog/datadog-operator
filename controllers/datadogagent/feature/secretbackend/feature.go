// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretbackend

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/kubernetes/rbac"
)

func init() {
	err := feature.Register(feature.SecretBackendIDType, buildSecretBackendFeature)
	if err != nil {
		panic(err)
	}
}

func buildSecretBackendFeature(options *feature.Options) feature.Feature {
	secretBackendFeat := &secretBackendFeature{}

	return secretBackendFeat
}

type secretBackendRole struct {
	namespace   string
	secretsList []string
}

type secretBackendFeature struct {
	serviceAccountNameAgent               string
	serviceAccountNameClusterAgent        string
	serviceAccountNameClusterChecksRunner string
	command                               string
	args                                  string
	timeout                               int32
	enableGlobalPermissions               bool
	roles                                 []secretBackendRole
	owner                                 metav1.Object
}

// ID returns the ID of the Feature
func (f *secretBackendFeature) ID() feature.IDType {
	return feature.SecretBackendIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *secretBackendFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	secretBackend := dda.Spec.Features.SecretBackend

	if secretBackend != nil {
		f.command = apiutils.StringValue(secretBackend.Command)
		f.args = apiutils.StringValue(secretBackend.Args)
		if secretBackend.Timeout != nil {
			f.timeout = *secretBackend.Timeout
		}
		f.enableGlobalPermissions = apiutils.BoolValue(secretBackend.EnableGlobalPermissions)

		if secretBackend.Roles != nil {
			// Disable global permissions if roles are specified
			f.enableGlobalPermissions = false
			for _, role := range secretBackend.Roles {
				f.roles = append(f.roles, secretBackendRole{
					namespace:   apiutils.StringValue(role.Namespace),
					secretsList: role.Secrets,
				})
			}
		}

		f.serviceAccountNameAgent = v2alpha1.GetAgentServiceAccount(dda)
		f.serviceAccountNameClusterAgent = v2alpha1.GetClusterAgentServiceAccount(dda)

		if v2alpha1.IsClusterChecksEnabled(dda) && v2alpha1.IsCCREnabled(dda) {
			f.serviceAccountNameClusterChecksRunner = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)
		}
	}

	// Require node Agent by default to manage dependencies
	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
		},
	}
	// If node Agent is disabled, require cluster Agent

	if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if apiutils.BoolValue(nodeAgent.Disabled) {
			reqComp = feature.RequiredComponents{
				ClusterAgent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(true),
				},
			}
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *secretBackendFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {

	if f.enableGlobalPermissions {
		rbacName := getGlobalPermSecretBackendRBACResourceName(f.owner)
		roleRef := rbacv1.RoleRef{
			APIGroup: rbac.RbacAPIGroup,
			Kind:     rbac.ClusterRoleKind,
			Name:     rbacName,
		}
		// Adding RBAC to node Agents
		if err := managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountNameAgent, secretBackendGlobalRBACPolicyRules); err != nil {
			return err
		}

		// Adding ClusterRoleBinding to cluster Agent
		if err := managers.RBACManager().AddClusterRoleBinding(f.owner.GetNamespace(), rbacName, f.serviceAccountNameClusterAgent, roleRef); err != nil {
			return err
		}
		// Adding ClusterRoleBinding to cluster checks runners
		// f.serviceAccountNameClusterChecksRunner is empty if CCRs are not enabled
		if f.serviceAccountNameClusterChecksRunner != "" {
			return managers.RBACManager().AddClusterRoleBinding(f.owner.GetNamespace(), rbacName, f.serviceAccountNameClusterChecksRunner, roleRef)
		}

	}

	if len(f.roles) > 0 {
		for _, role := range f.roles {
			ns := role.namespace
			rbacName := getNamespaceSecretReaderRBACResourceName(f.owner, ns)
			policyRule := getSecretsRolesPermissions(role)
			targetSaNamespace := f.owner.GetNamespace()
			roleRef := rbacv1.RoleRef{
				APIGroup: rbac.RbacAPIGroup,
				Kind:     rbac.RoleKind,
				Name:     rbacName,
			}
			// Adding RBAC to node Agents
			if err := managers.RBACManager().AddPolicyRules(ns, rbacName, f.serviceAccountNameAgent, policyRule, targetSaNamespace); err != nil {
				return err
			}
			// Adding RBAC to cluster Agent
			if err := managers.RBACManager().AddRoleBinding(ns, rbacName, targetSaNamespace, f.serviceAccountNameClusterAgent, roleRef); err != nil {
				return err
			}
			// Adding RBAC to cluster checks runners
			// f.serviceAccountNameClusterChecksRunner is empty if CCRs are not enabled
			if f.serviceAccountNameClusterChecksRunner != "" {
				if err := managers.RBACManager().AddRoleBinding(ns, rbacName, targetSaNamespace, f.serviceAccountNameClusterChecksRunner, roleRef); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *secretBackendFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {

	if f.command != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDSecretBackendCommand,
			Value: f.command,
		})
	}

	if f.args != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDSecretBackendArguments,
			Value: f.args,
		})
	}

	if f.timeout != 0 {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDSecretBackendTimeout,
			Value: strconv.FormatInt(int64(f.timeout), 10),
		})
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *secretBackendFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *secretBackendFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {

	if f.command != "" {
		managers.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSecretBackendCommand,
			Value: f.command,
		})
	}

	if f.args != "" {
		managers.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSecretBackendArguments,
			Value: f.args,
		})
	}

	if f.timeout != 0 {
		managers.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  apicommon.DDSecretBackendTimeout,
			Value: strconv.FormatInt(int64(f.timeout), 10),
		})
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *secretBackendFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	if f.command != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterChecksRunnersContainerName, &corev1.EnvVar{
			Name:  apicommon.DDSecretBackendCommand,
			Value: f.command,
		})
	}

	if f.args != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterChecksRunnersContainerName, &corev1.EnvVar{
			Name:  apicommon.DDSecretBackendArguments,
			Value: f.args,
		})
	}

	if f.timeout != 0 {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterChecksRunnersContainerName, &corev1.EnvVar{
			Name:  apicommon.DDSecretBackendTimeout,
			Value: strconv.FormatInt(int64(f.timeout), 10),
		})
	}

	return nil
}
