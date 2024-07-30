// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package secretsbackend

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.SecretsBackendIDType, buildSecretsBackendFeature)
	if err != nil {
		panic(err)
	}
}

func buildSecretsBackendFeature(options *feature.Options) feature.Feature {
	secretsBackendFeat := &secretsBackendFeature{}

	return secretsBackendFeat
}

type secretsBackendFeature struct {
	serviceAccountNameAgent               string
	serviceAccountNameClusterAgent        string
	serviceAccountNameClusterChecksRunner string
	command                               string
	args                                  string
	timeout                               int32
	enableGlobalPermissions               bool
	owner                                 metav1.Object
}

// ID returns the ID of the Feature
func (f *secretsBackendFeature) ID() feature.IDType {
	return feature.SecretsBackendIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *secretsBackendFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	secretsBackend := dda.Spec.Features.SecretsBackend

	if secretsBackend != nil {
		f.command = apiutils.StringValue(secretsBackend.Command)
		f.args = apiutils.StringValue(secretsBackend.Args)
		if secretsBackend.Timeout != nil {
			f.timeout = *secretsBackend.Timeout
		}
		f.enableGlobalPermissions = apiutils.BoolValue(secretsBackend.EnableGlobalPermissions)
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
	if apiutils.BoolValue(dda.Spec.Override[v2alpha1.NodeAgentComponentName].Disabled) {
		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *secretsBackendFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	return feature.RequiredComponents{}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *secretsBackendFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	rbacName := getSecretsBackendRBACResourceName(f.owner)

	if f.enableGlobalPermissions {
		// Adding RBAC to node Agents
		if err := managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountNameAgent, secretsBackendGlobalRBACPolicyRules); err != nil {
			return err
		}
		// Adding RBAC to cluster Agent
		if err := managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountNameClusterAgent, secretsBackendGlobalRBACPolicyRules); err != nil {
			return err
		}
		// Adding RBAC to cluster checks runners
		// f.serviceAccountNameClusterChecksRunner is empty if CCRs are not enabled
		if f.serviceAccountNameClusterChecksRunner != "" {
			return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountNameClusterChecksRunner, secretsBackendGlobalRBACPolicyRules)
		}

	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *secretsBackendFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {

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

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *secretsBackendFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *secretsBackendFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {

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
func (f *secretsBackendFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
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
