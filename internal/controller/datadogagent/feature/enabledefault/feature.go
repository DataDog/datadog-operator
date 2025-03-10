// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package enabledefault

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	componentagent "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/agent"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagent/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/version"
)

func init() {
	err := feature.Register(feature.DefaultIDType, buildDefaultFeature)
	if err != nil {
		panic(err)
	}
}

func buildDefaultFeature(options *feature.Options) feature.Feature {
	dF := &defaultFeature{}

	if options != nil {
		dF.logger = options.Logger
	}

	return dF
}

type defaultFeature struct {
	owner metav1.Object

	clusterAgent            clusterAgentConfig
	agent                   agentConfig
	clusterChecksRunner     clusterChecksRunnerConfig
	logger                  logr.Logger
	disableNonResourceRules bool
	adpEnabled              bool

	kubernetesResourcesLabelsAsTags      map[string]map[string]string
	kubernetesResourcesAnnotationsAsTags map[string]map[string]string
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

	f.clusterAgent.serviceAccountName = constants.GetClusterAgentServiceAccount(dda)
	f.agent.serviceAccountName = constants.GetAgentServiceAccount(dda)
	f.clusterChecksRunner.serviceAccountName = constants.GetClusterChecksRunnerServiceAccount(dda)

	f.clusterAgent.serviceAccountAnnotations = constants.GetClusterAgentServiceAccountAnnotations(dda)
	f.agent.serviceAccountAnnotations = constants.GetAgentServiceAccountAnnotations(dda)
	f.clusterChecksRunner.serviceAccountAnnotations = constants.GetClusterChecksRunnerServiceAccountAnnotations(dda)

	if dda.ObjectMeta.Annotations != nil {
		f.adpEnabled = featureutils.HasAgentDataPlaneAnnotation(dda)
	}

	if dda.Spec.Global != nil {
		if dda.Spec.Global.DisableNonResourceRules != nil && *dda.Spec.Global.DisableNonResourceRules {
			f.disableNonResourceRules = true
		}

		f.kubernetesResourcesLabelsAsTags = dda.Spec.Global.KubernetesResourcesLabelsAsTags
		f.kubernetesResourcesAnnotationsAsTags = dda.Spec.Global.KubernetesResourcesAnnotationsAsTags
		f.clusterAgent.resourceMetadataAsTagsClusterRoleName = componentdca.GetResourceMetadataAsTagsClusterRoleName(dda)
	}

	agentContainers := make([]apicommon.AgentContainerName, 0)

	// If Agent Data Plane is enabled, add the ADP container to the list of required containers for the Agent feature.
	if f.adpEnabled {
		agentContainers = append(agentContainers, apicommon.AgentDataPlaneContainerName)
	}

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: &trueValue,
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
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
	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  DDClusterAgentServiceAccountName,
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

	if f.adpEnabled {
		// When ADP is enabled, we signal this to the Core Agent by setting an environment variable.
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  common.DDADPEnabled,
			Value: "true",
		})
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *defaultFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	f.addDefaultCommonEnvs(managers)

	return nil
}

func (f *defaultFeature) addDefaultCommonEnvs(managers feature.PodTemplateManagers) {
	if len(f.kubernetesResourcesLabelsAsTags) > 0 {
		kubernetesResourceLabelsAsTags, err := json.Marshal(f.kubernetesResourcesLabelsAsTags)
		if err != nil {
			f.logger.Error(err, "Failed to unmarshal json input")
		} else {
			managers.EnvVar().AddEnvVar(&corev1.EnvVar{
				Name:  DDKubernetesResourcesLabelsAsTags,
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
				Name:  DDKubernetesResourcesAnnotationsAsTags,
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
	if envVar := os.Getenv(InstallInfoToolVersion); envVar != "" {
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
