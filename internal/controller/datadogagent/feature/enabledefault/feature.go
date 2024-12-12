// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"encoding/json"
	"fmt"
	"os"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
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
	adpEnabled              bool

	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	kubernetesResourcesLabelsAsTags      map[string]map[string]string
	kubernetesResourcesAnnotationsAsTags map[string]map[string]string
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
	serviceAccountName        string
	serviceAccountAnnotations map[string]string

	resourceMetadataAsTagsClusterRoleName string
}

type agentConfig struct {
	serviceAccountName        string
	serviceAccountAnnotations map[string]string
}

type clusterChecksRunnerConfig struct {
	serviceAccountName        string
	serviceAccountAnnotations map[string]string
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

	f.clusterAgent.serviceAccountAnnotations = v2alpha1.GetClusterAgentServiceAccountAnnotations(dda)
	f.agent.serviceAccountAnnotations = v2alpha1.GetAgentServiceAccountAnnotations(dda)
	f.clusterChecksRunner.serviceAccountAnnotations = v2alpha1.GetClusterChecksRunnerServiceAccountAnnotations(dda)

	if dda.ObjectMeta.Annotations != nil {
		f.adpEnabled = featureutils.HasAgentDataPlaneAnnotation(dda)
	}

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
				f.credentialsInfo.secretCreation.data[v2alpha1.DefaultAPIKeyKey] = *creds.APIKey
				f.credentialsInfo.apiKey.SecretName = f.credentialsInfo.secretCreation.name
				f.credentialsInfo.apiKey.SecretKey = v2alpha1.DefaultAPIKeyKey
			} else if creds.APISecret != nil {
				f.credentialsInfo.apiKey.SecretName = creds.APISecret.SecretName
				f.credentialsInfo.apiKey.SecretKey = creds.APISecret.KeyName
			}

			if creds.AppKey != nil {
				f.credentialsInfo.secretCreation.data[v2alpha1.DefaultAPPKeyKey] = *creds.AppKey
				f.credentialsInfo.appKey.SecretName = f.credentialsInfo.secretCreation.name
				f.credentialsInfo.appKey.SecretKey = v2alpha1.DefaultAPPKeyKey
			} else if creds.AppSecret != nil {
				f.credentialsInfo.appKey.SecretName = creds.AppSecret.SecretName
				f.credentialsInfo.appKey.SecretKey = creds.AppSecret.KeyName
			}
		}

		// DCA Token management
		f.dcaTokenInfo.token.SecretName = v2alpha1.GetDefaultDCATokenSecretName(dda)
		f.dcaTokenInfo.token.SecretKey = v2alpha1.DefaultTokenKey
		if dda.Spec.Global.ClusterAgentToken != nil {
			// User specifies token
			f.dcaTokenInfo.secretCreation.createSecret = true
			f.dcaTokenInfo.secretCreation.name = f.dcaTokenInfo.token.SecretName
			f.dcaTokenInfo.secretCreation.data[v2alpha1.DefaultTokenKey] = *dda.Spec.Global.ClusterAgentToken
		} else if dda.Spec.Global.ClusterAgentTokenSecret != nil {
			// User specifies token secret
			f.dcaTokenInfo.token.SecretName = dda.Spec.Global.ClusterAgentTokenSecret.SecretName
			f.dcaTokenInfo.token.SecretKey = dda.Spec.Global.ClusterAgentTokenSecret.KeyName
		} else if dda.Spec.Global.ClusterAgentToken == nil {
			// Token needs to be generated or read from status
			f.dcaTokenInfo.secretCreation.createSecret = true
			f.dcaTokenInfo.secretCreation.name = f.dcaTokenInfo.token.SecretName
			if dda.Status.ClusterAgent == nil || dda.Status.ClusterAgent.GeneratedToken == "" {
				f.dcaTokenInfo.secretCreation.data[v2alpha1.DefaultTokenKey] = apiutils.GenerateRandomString(32)
			} else {
				f.dcaTokenInfo.secretCreation.data[v2alpha1.DefaultTokenKey] = dda.Status.ClusterAgent.GeneratedToken
			}
		}

		f.kubernetesResourcesLabelsAsTags = dda.Spec.Global.KubernetesResourcesLabelsAsTags
		f.kubernetesResourcesAnnotationsAsTags = dda.Spec.Global.KubernetesResourcesAnnotationsAsTags
		f.clusterAgent.resourceMetadataAsTagsClusterRoleName = componentdca.GetResourceMetadataAsTagsClusterRoleName(dda)

		hash, err := comparison.GenerateMD5ForSpec(f.dcaTokenInfo.secretCreation.data)
		if err != nil {
			f.logger.Error(err, "couldn't generate hash for Cluster Agent token hash")
		} else {
			f.logger.V(2).Info("built Cluster Agent token hash", "hash", hash)
		}
		f.customConfigAnnotationValue = hash
		f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(string(feature.DefaultIDType))
	}

	agentContainers := make([]apicommon.AgentContainerName, 0)

	// If the OpenTelemetry Agent is enabled, add the OTel Agent to the list of required containers for the Agent
	// feature.
	//
	// NOTE: This is a temporary solution until the OTel Agent is fully integrated into the Operator via a dedicated feature.
	if dda.ObjectMeta.Annotations != nil && featureutils.HasOtelAgentAnnotation(dda) {
		agentContainers = append(agentContainers, apicommon.OtelAgent)
	}

	// If Agent Data Plane is enabled, add the ADP container to the list of required containers for the Agent feature.
	if f.adpEnabled {
		agentContainers = append(agentContainers, apicommon.AgentDataPlaneContainerName)
	}

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: &trueValue,
		},
		Agent: feature.RequiredComponent{
			IsRequired: &trueValue,
			Containers: agentContainers,
		},
	}
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

	// serviceAccountAnnotations
	if f.agent.serviceAccountAnnotations != nil {
		if err := managers.RBACManager().AddServiceAccountAnnotationsByComponent(f.owner.GetNamespace(), f.agent.serviceAccountName, f.agent.serviceAccountAnnotations, string(v2alpha1.NodeAgentComponentName)); err != nil {
			errs = append(errs, err)
		}
	}

	// ClusterRole creation
	if err := managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), componentagent.GetAgentRoleName(f.owner), f.agent.serviceAccountName, getDefaultAgentClusterRolePolicyRules(f.disableNonResourceRules)); err != nil {
		errs = append(errs, err)
	}

	// Create a configmap for the default seccomp profile in the System Probe.
	// This is mounted in the init-volume container in the agent default code.
	for _, containerName := range requiredComponent.Containers {
		if containerName == apicommon.SystemProbeContainerName {
			errs = append(errs, managers.ConfigMapManager().AddConfigMap(
				common.GetDefaultSeccompConfigMapName(f.owner),
				f.owner.GetNamespace(),
				DefaultSeccompConfigDataForSystemProbe(),
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
		if err := managers.RBACManager().AddPolicyRulesByComponent(f.owner.GetNamespace(), componentdca.GetClusterAgentRbacResourcesName(f.owner), f.clusterAgent.serviceAccountName, getDefaultClusterAgentRolePolicyRules(f.owner), string(v2alpha1.ClusterAgentComponentName)); err != nil {
			errs = append(errs, err)
		}

		// ClusterRole creation
		if err := managers.RBACManager().AddClusterPolicyRulesByComponent(f.owner.GetNamespace(), componentdca.GetClusterAgentRbacResourcesName(f.owner), f.clusterAgent.serviceAccountName, getDefaultClusterAgentClusterRolePolicyRules(f.owner), string(v2alpha1.ClusterAgentComponentName)); err != nil {
			errs = append(errs, err)
		}
	}

	// serviceAccountAnnotations
	if f.agent.serviceAccountAnnotations != nil {
		if err := managers.RBACManager().AddServiceAccountAnnotationsByComponent(f.owner.GetNamespace(), f.clusterAgent.serviceAccountName, f.clusterAgent.serviceAccountAnnotations, string(v2alpha1.ClusterAgentComponentName)); err != nil {
			errs = append(errs, err)
		}
	}

	dcaService := componentdca.GetClusterAgentService(f.owner)
	if err := managers.Store().AddOrUpdate(kubernetes.ServicesKind, dcaService); err != nil {
		return err
	}

	if len(f.kubernetesResourcesLabelsAsTags) > 0 || len(f.kubernetesResourcesAnnotationsAsTags) > 0 {
		err := managers.RBACManager().AddClusterPolicyRules(
			f.owner.GetNamespace(),
			f.clusterAgent.resourceMetadataAsTagsClusterRoleName,
			f.clusterAgent.serviceAccountName,
			getKubernetesResourceMetadataAsTagsPolicyRules(f.kubernetesResourcesLabelsAsTags, f.kubernetesResourcesAnnotationsAsTags),
		)
		if err != nil {
			errs = append(errs, err)
		}
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
		if err := managers.RBACManager().AddClusterPolicyRulesByComponent(f.owner.GetNamespace(), getCCRRbacResourcesName(f.owner), f.clusterChecksRunner.serviceAccountName, getDefaultClusterChecksRunnerClusterRolePolicyRules(f.owner, f.disableNonResourceRules), string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
			errs = append(errs, err)
		}
	}

	// serviceAccountAnnotations
	if f.agent.serviceAccountAnnotations != nil {
		if err := managers.RBACManager().AddServiceAccountAnnotationsByComponent(f.owner.GetNamespace(), f.clusterChecksRunner.serviceAccountName, f.clusterChecksRunner.serviceAccountAnnotations, string(v2alpha1.ClusterChecksRunnerComponentName)); err != nil {
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
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDClusterAgentServiceAccountName,
		Value: f.clusterAgent.serviceAccountName,
	})
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  DDAgentDaemonSet,
		Value: getDaemonSetNameFromDatadogAgent(f.owner.(*v2alpha1.DatadogAgent)),
	})
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  DDClusterAgentDeployment,
		Value: getDeploymentNameFromDatadogAgent(f.owner.(*v2alpha1.DatadogAgent)),
	})
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  DDDatadogAgentCustomResource,
		Value: f.owner.GetName(),
	})

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
		tokenEnvVar := common.BuildEnvVarFromSource(apicommon.DDClusterAgentAuthToken, common.BuildEnvVarFromSecret(f.dcaTokenInfo.token.SecretName, f.dcaTokenInfo.token.SecretKey))
		managers.EnvVar().AddEnvVar(tokenEnvVar)
	}

	if f.credentialsInfo.apiKey.SecretName != "" {
		apiKeyEnvVar := common.BuildEnvVarFromSource(apicommon.DDAPIKey, common.BuildEnvVarFromSecret(f.credentialsInfo.apiKey.SecretName, f.credentialsInfo.apiKey.SecretKey))
		managers.EnvVar().AddEnvVar(apiKeyEnvVar)
	}

	if f.credentialsInfo.appKey.SecretName != "" {
		appKeyEnvVar := common.BuildEnvVarFromSource(apicommon.DDAppKey, common.BuildEnvVarFromSecret(f.credentialsInfo.appKey.SecretName, f.credentialsInfo.appKey.SecretKey))
		managers.EnvVar().AddEnvVar(appKeyEnvVar)
	}

	if len(f.kubernetesResourcesLabelsAsTags) > 0 {
		kubernetesResourceLabelsAsTags, err := json.Marshal(f.kubernetesResourcesLabelsAsTags)
		if err != nil {
			f.logger.Error(err, "Failed to unmarshal json input")
		} else {
			managers.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDKubernetesResourcesLabelsAsTags,
				Value: string(kubernetesResourceLabelsAsTags),
			})
		}
	}

	if len(f.kubernetesResourcesAnnotationsAsTags) > 0 {
		kubernetesResourceAnnotationsAsTags, err := json.Marshal(f.kubernetesResourcesAnnotationsAsTags)
		if err != nil {
			f.logger.Error(err, "Failed to unmarshal json input")
		} else {
			managers.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  apicommon.DDKubernetesResourcesAnnotationsAsTags,
				Value: string(kubernetesResourceAnnotationsAsTags),
			})
		}
	}
}

func buildInstallInfoConfigMap(dda metav1.Object) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      common.GetInstallInfoConfigMapName(dda),
			Namespace: dda.GetNamespace(),
		},
		Data: map[string]string{
			"install_info": getInstallInfoValue(),
		},
	}

	return configMap
}

func getInstallInfoValue() string {
	toolVersion := "unknown"
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
