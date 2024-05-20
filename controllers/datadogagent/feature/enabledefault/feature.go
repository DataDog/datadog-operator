// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"fmt"
	"os"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	commonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/component/agent"
	componentdca "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusteragent"
	componentccr "github.com/DataDog/datadog-operator/controllers/datadogagent/component/clusterchecksrunner"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/version"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
)

func init() {
	err := feature.Register(feature.DefaultIDType, buildDefaultFeature)
	if err != nil {
		panic(err)
	}
}

func buildDefaultFeature(options *feature.Options) feature.Feature {
	dF := &defaultFeature{
		credentialsInfo: credentialsInfo{
			secretCreation: secretInfo{
				data: make(map[string]string),
			},
		},
		dcaTokenInfo: dcaTokenInfo{
			secretCreation: secretInfo{
				data: make(map[string]string),
			},
		},
	}

	if options != nil {
		dF.logger = options.Logger
	}

	return dF
}

type defaultFeature struct {
	owner metav1.Object

	credentialsInfo         credentialsInfo
	dcaTokenInfo            dcaTokenInfo
	clusterAgent            clusterAgentConfig
	agent                   agentConfig
	clusterChecksRunner     clusterChecksRunnerConfig
	logger                  logr.Logger
	disableNonResourceRules bool

	customConfigAnnotationKey   string
	customConfigAnnotationValue string
}

type credentialsInfo struct {
	apiKey         keyInfo
	appKey         keyInfo
	secretCreation secretInfo
}

type dcaTokenInfo struct {
	token          keyInfo
	secretCreation secretInfo
}

type keyInfo struct {
	SecretName string
	SecretKey  string
}

type secretInfo struct {
	createSecret bool
	name         string
	data         map[string]string
}

type clusterAgentConfig struct {
	serviceAccountName string
}

type agentConfig struct {
	serviceAccountName string
}

type clusterChecksRunnerConfig struct {
	serviceAccountName string
}

// ID returns the ID of the Feature
func (f *defaultFeature) ID() feature.IDType {
	return feature.DefaultIDType
}

func (f *defaultFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	trueValue := true
	f.owner = dda

	f.clusterAgent.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)
	f.agent.serviceAccountName = v2alpha1.GetAgentServiceAccount(dda)
	f.clusterChecksRunner.serviceAccountName = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)

	if dda.Spec.Global != nil {
		if dda.Spec.Global.DisableNonResourceRules != nil && *dda.Spec.Global.DisableNonResourceRules {
			f.disableNonResourceRules = true
		}

		if dda.Spec.Global.Credentials != nil {
			creds := dda.Spec.Global.Credentials

			if creds.APIKey != nil || creds.AppKey != nil {
				f.credentialsInfo.secretCreation.createSecret = true
				f.credentialsInfo.secretCreation.name = v2alpha1.GetDefaultCredentialsSecretName(dda)
			}

			if creds.APIKey != nil {
				f.credentialsInfo.secretCreation.data[apicommon.DefaultAPIKeyKey] = *creds.APIKey
				f.credentialsInfo.apiKey.SecretName = f.credentialsInfo.secretCreation.name
				f.credentialsInfo.apiKey.SecretKey = apicommon.DefaultAPIKeyKey
			} else if creds.APISecret != nil {
				f.credentialsInfo.apiKey.SecretName = creds.APISecret.SecretName
				f.credentialsInfo.apiKey.SecretKey = creds.APISecret.KeyName
			}

			if creds.AppKey != nil {
				f.credentialsInfo.secretCreation.data[apicommon.DefaultAPPKeyKey] = *creds.AppKey
				f.credentialsInfo.appKey.SecretName = f.credentialsInfo.secretCreation.name
				f.credentialsInfo.appKey.SecretKey = apicommon.DefaultAPPKeyKey
			} else if creds.AppSecret != nil {
				f.credentialsInfo.appKey.SecretName = creds.AppSecret.SecretName
				f.credentialsInfo.appKey.SecretKey = creds.AppSecret.KeyName
			}
		}

		// DCA Token management
		f.dcaTokenInfo.token.SecretName = v2alpha1.GetDefaultDCATokenSecretName(dda)
		f.dcaTokenInfo.token.SecretKey = apicommon.DefaultTokenKey
		if dda.Spec.Global.ClusterAgentToken != nil {
			// User specifies token
			f.dcaTokenInfo.secretCreation.createSecret = true
			f.dcaTokenInfo.secretCreation.name = f.dcaTokenInfo.token.SecretName
			f.dcaTokenInfo.secretCreation.data[apicommon.DefaultTokenKey] = *dda.Spec.Global.ClusterAgentToken
		} else if dda.Spec.Global.ClusterAgentTokenSecret != nil {
			// User specifies token secret
			f.dcaTokenInfo.token.SecretName = dda.Spec.Global.ClusterAgentTokenSecret.SecretName
			f.dcaTokenInfo.token.SecretKey = dda.Spec.Global.ClusterAgentTokenSecret.KeyName
		} else if dda.Spec.Global.ClusterAgentToken == nil {
			// Token needs to be generated or read from status
			f.dcaTokenInfo.secretCreation.createSecret = true
			f.dcaTokenInfo.secretCreation.name = f.dcaTokenInfo.token.SecretName
			if dda.Status.ClusterAgent == nil || dda.Status.ClusterAgent.GeneratedToken == "" {
				f.dcaTokenInfo.secretCreation.data[apicommon.DefaultTokenKey] = apiutils.GenerateRandomString(32)
			} else {
				f.dcaTokenInfo.secretCreation.data[apicommon.DefaultTokenKey] = dda.Status.ClusterAgent.GeneratedToken
			}
		}
		hash, err := comparison.GenerateMD5ForSpec(f.dcaTokenInfo.secretCreation.data)
		if err != nil {
			f.logger.Error(err, "couldn't generate hash for Cluster Agent token hash")
		} else {
			f.logger.V(2).Info("built Cluster Agent token hash", "hash", hash)
		}
		f.customConfigAnnotationValue = hash
		f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(string(feature.DefaultIDType))
	}

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: &trueValue,
		},
		Agent: feature.RequiredComponent{
			IsRequired: &trueValue,
		},
	}
}

func (f *defaultFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	/*
		trueValue := true
		f.owner = dda
		f.namespace = dda.GetNamespace()

		required := feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{
				IsRequired: &trueValue,
			},
			Agent: feature.RequiredComponent{
				IsRequired: &trueValue,
			},
		}

		f.clusterAgent.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)
		f.agent.serviceAccountName = v1alpha1.GetAgentServiceAccount(dda)
		f.clusterChecksRunner.serviceAccountName = v1alpha1.GetClusterChecksRunnerServiceAccount(dda)

		// get info about credential
		// If API key, app key _and_ token don't need a new secret, then don't create one.
		if dda.Spec.Credentials != nil &&
			(!v1alpha1.CheckAPIKeySufficiency(&dda.Spec.Credentials.DatadogCredentials, config.DDAPIKeyEnvVar) ||
				!v1alpha1.CheckAppKeySufficiency(&dda.Spec.Credentials.DatadogCredentials, config.DDAppKeyEnvVar)) {
			f.credentialsInfo.secretCreation.createSecret = true
			f.credentialsInfo.secretCreation.name = v1alpha1.GetDefaultCredentialsSecretName(dda)

			creds := dda.Spec.Credentials
			if creds.APIKey != "" {
				f.credentialsInfo.secretCreation.data[apicommon.DefaultAPIKeyKey] = creds.APIKey
			}
			if creds.AppKey != "" {
				f.credentialsInfo.secretCreation.data[apicommon.DefaultAPPKeyKey] = creds.AppKey
			}

			// TOKEN management
			f.dcaTokenInfo.secretCreation.createSecret = true
			f.dcaTokenInfo.secretCreation.name = v1alpha1.GetDefaultCredentialsSecretName(dda)
			f.dcaTokenInfo.token.SecretName = f.dcaTokenInfo.secretCreation.name
			f.dcaTokenInfo.token.SecretKey = apicommon.DefaultTokenKey
			if creds.Token != "" {
				f.dcaTokenInfo.secretCreation.data[apicommon.DefaultTokenKey] = creds.Token
			} else if apiutils.BoolValue(dda.Spec.ClusterAgent.Enabled) {
				defaultedToken := v1alpha1.DefaultedClusterAgentToken(&dda.Status)
				if defaultedToken != "" {
					f.dcaTokenInfo.secretCreation.data[apicommon.DefaultTokenKey] = defaultedToken
				}
			}
		}
	*/
	// to not apply this feature on v1alpha1
	// Else it break unittest in `controller_test.go` because the `store` modified the dependency resources with an additional labels.
	// which make the comparison failing.
	required := feature.RequiredComponents{}

	return required
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *defaultFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	var errs []error
	// manage credential secret
	if f.credentialsInfo.secretCreation.createSecret {
		for key, value := range f.credentialsInfo.secretCreation.data {
			if err := managers.SecretManager().AddSecret(f.owner.GetNamespace(), f.credentialsInfo.secretCreation.name, key, value); err != nil {
				errs = append(errs, err)
			}
		}
	}

	if components.ClusterAgent.IsEnabled() && f.dcaTokenInfo.secretCreation.createSecret {
		for key, value := range f.dcaTokenInfo.secretCreation.data {
			if err := managers.SecretManager().AddSecret(f.owner.GetNamespace(), f.dcaTokenInfo.secretCreation.name, key, value); err != nil {
				errs = append(errs, err)
			}
		}
		// Adding Annotation containing data hash to secret.
		if err := managers.SecretManager().AddAnnotations(f.logger, f.owner.GetNamespace(), f.dcaTokenInfo.secretCreation.name, map[string]string{f.customConfigAnnotationKey: f.customConfigAnnotationValue}); err != nil {
			errs = append(errs, err)
		}

	}

	// Create install-info configmap
	installInfoCM := buildInstallInfoConfigMap(f.owner)
	if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, installInfoCM); err != nil {
		return err
	}

	if components.Agent.IsEnabled() {
		if err := f.agentDependencies(managers, components.Agent); err != nil {
			errs = append(errs, err)
		}
	}

	if components.ClusterAgent.IsEnabled() {
		if err := f.clusterAgentDependencies(managers, components.ClusterAgent); err != nil {
			errs = append(errs, err)
		}
	}

	if components.ClusterChecksRunner.IsEnabled() {
		if err := f.clusterChecksRunnerDependencies(managers, components.ClusterChecksRunner); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.NewAggregate(errs)
}

func (f *defaultFeature) agentDependencies(managers feature.ResourceManagers, requiredComponent feature.RequiredComponent) error {
	var errs []error
	// serviceAccount
	if f.agent.serviceAccountName != "" {
		if err := managers.RBACManager().AddServiceAccountByComponent(f.owner.GetNamespace(), f.agent.serviceAccountName, string(v2alpha1.NodeAgentComponentName)); err != nil {
			errs = append(errs, err)
		}
	}

	// ClusterRole creation
	if err := managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), agent.GetAgentRoleName(f.owner), f.agent.serviceAccountName, agent.GetDefaultAgentClusterRolePolicyRules(f.disableNonResourceRules)); err != nil {
		errs = append(errs, err)
	}

	// Create a configmap for the default seccomp profile in the System Probe.
	// This is mounted in the init-volume container in the agent default code.
	for _, containerName := range requiredComponent.Containers {
		if containerName == commonv1.SystemProbeContainerName {
			errs = append(errs, managers.ConfigMapManager().AddConfigMap(
				component.GetDefaultSeccompConfigMapName(f.owner),
				f.owner.GetNamespace(),
				agent.DefaultSeccompConfigDataForSystemProbe(),
			))
		}
	}

	return errors.NewAggregate(errs)
}

func (f *defaultFeature) clusterAgentDependencies(managers feature.ResourceManagers, component feature.RequiredComponent) error {
	_ = component
	var errs []error
	if f.clusterAgent.serviceAccountName != "" {
		// Service Account creation
		if err := managers.RBACManager().AddServiceAccountByComponent(f.owner.GetNamespace(), f.clusterAgent.serviceAccountName, string(v2alpha1.ClusterAgentComponentName)); err != nil {
			errs = append(errs, err)
		}

		// Role Creation
		if err := managers.RBACManager().AddPolicyRulesByComponent(f.owner.GetNamespace(), componentdca.GetClusterAgentRbacResourcesName(f.owner), f.clusterAgent.serviceAccountName, componentdca.GetDefaultClusterAgentRolePolicyRules(f.owner), string(v2alpha1.ClusterAgentComponentName)); err != nil {
			errs = append(errs, err)
		}

		// ClusterRole creation
		if err := managers.RBACManager().AddClusterPolicyRulesByComponent(f.owner.GetNamespace(), componentdca.GetClusterAgentRbacResourcesName(f.owner), f.clusterAgent.serviceAccountName, componentdca.GetDefaultClusterAgentClusterRolePolicyRules(f.owner), string(v2alpha1.ClusterAgentComponentName)); err != nil {
			errs = append(errs, err)
		}
	}

	dcaService := componentdca.GetClusterAgentService(f.owner)
	if err := managers.Store().AddOrUpdate(kubernetes.ServicesKind, dcaService); err != nil {
		return err
	}

	return errors.NewAggregate(errs)
}

func (f *defaultFeature) clusterChecksRunnerDependencies(managers feature.ResourceManagers, component feature.RequiredComponent) error {
	_ = component
	var errs []error
	if f.clusterChecksRunner.serviceAccountName != "" {
		// Service Account creation
		if err := managers.RBACManager().AddServiceAccountByComponent(f.owner.GetNamespace(), f.clusterChecksRunner.serviceAccountName, string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
			errs = append(errs, err)
		}

		// ClusterRole creation
		if err := managers.RBACManager().AddClusterPolicyRulesByComponent(f.owner.GetNamespace(), componentccr.GetCCRRbacResourcesName(f.owner), f.clusterChecksRunner.serviceAccountName, componentccr.GetDefaultClusterChecksRunnerClusterRolePolicyRules(f.owner, f.disableNonResourceRules), string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.NewAggregate(errs)
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	f.addDefaultCommonEnvs(managers)
	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.ManageNodeAgent(managers, provider)

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.addDefaultCommonEnvs(managers)
	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	f.addDefaultCommonEnvs(managers)
	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}

	return nil
}

func (f *defaultFeature) addDefaultCommonEnvs(managers feature.PodTemplateManagers) {
	if f.dcaTokenInfo.token.SecretName != "" {
		tokenEnvVar := component.BuildEnvVarFromSource(apicommon.DDClusterAgentAuthToken, component.BuildEnvVarFromSecret(f.dcaTokenInfo.token.SecretName, f.dcaTokenInfo.token.SecretKey))
		managers.EnvVar().AddEnvVar(tokenEnvVar)
	}

	if f.credentialsInfo.apiKey.SecretName != "" {
		apiKeyEnvVar := component.BuildEnvVarFromSource(apicommon.DDAPIKey, component.BuildEnvVarFromSecret(f.credentialsInfo.apiKey.SecretName, f.credentialsInfo.apiKey.SecretKey))
		managers.EnvVar().AddEnvVar(apiKeyEnvVar)
	}

	if f.credentialsInfo.appKey.SecretName != "" {
		appKeyEnvVar := component.BuildEnvVarFromSource(apicommon.DDAppKey, component.BuildEnvVarFromSecret(f.credentialsInfo.appKey.SecretName, f.credentialsInfo.appKey.SecretKey))
		managers.EnvVar().AddEnvVar(appKeyEnvVar)
	}
}

func buildInstallInfoConfigMap(dda metav1.Object) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      component.GetInstallInfoConfigMapName(dda),
			Namespace: dda.GetNamespace(),
		},
		Data: map[string]string{
			"install_info": getInstallInfoValue(),
		},
	}

	return configMap
}

func getInstallInfoValue() string {
	toolVersion := "datadog-operator"
	if envVar := os.Getenv(apicommon.InstallInfoToolVersion); envVar != "" {
		toolVersion = envVar
	}

	return fmt.Sprintf(installInfoDataTmpl, toolVersion, version.Version)
}

const installInfoDataTmpl = `---
install_method:
  tool: datadog-operator
  tool_version: %s
  installer_version: %s
`
