// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	common "github.com/DataDog/datadog-operator/controllers/datadogagent/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.OrchestratorExplorerIDType, buildOrchestratorExplorerFeature)
	if err != nil {
		panic(err)
	}
}

func buildOrchestratorExplorerFeature(options *feature.Options) feature.Feature {
	orchestratorExplorerFeat := &orchestratorExplorerFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}

	return orchestratorExplorerFeat
}

type orchestratorExplorerFeature struct {
	clusterChecksEnabled bool
	scrubContainers      bool
	extraTags            []string
	ddURL                string
	rbacSuffix           string
	serviceAccountName   string
	owner                metav1.Object
	customConfig         *apicommonv1.CustomConfig
	configConfigMapName  string
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *orchestratorExplorerFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	orchestratorExplorer := dda.Spec.Features.OrchestratorExplorer

	if orchestratorExplorer != nil && apiutils.BoolValue(orchestratorExplorer.Enabled) {
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.ProcessAgentContainerName},
		}

		if orchestratorExplorer.Conf != nil {
			f.customConfig = v2alpha1.ConvertCustomConfig(orchestratorExplorer.Conf)
		}
		f.configConfigMapName = apicommonv1.GetConfName(dda, f.customConfig, apicommon.DefaultOrchestratorExplorerConf)
		f.scrubContainers = apiutils.BoolValue(orchestratorExplorer.ScrubContainers)
		f.extraTags = orchestratorExplorer.ExtraTags
		if orchestratorExplorer.DDUrl != nil {
			f.ddURL = *orchestratorExplorer.DDUrl
		}
		f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)

		if v2alpha1.IsClusterChecksEnabled(dda) {
			f.clusterChecksEnabled = true

			if v2alpha1.IsCCREnabled(dda) {
				f.rbacSuffix = common.ChecksRunnerSuffix
				f.serviceAccountName = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)
				reqComp.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
			}
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *orchestratorExplorerFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	orchestratorExplorer := dda.Spec.Features.OrchestratorExplorer

	if orchestratorExplorer != nil && apiutils.BoolValue(orchestratorExplorer.Enabled) {
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{apicommonv1.CoreAgentContainerName, apicommonv1.ProcessAgentContainerName},
		}

		if orchestratorExplorer.Conf != nil {
			f.customConfig = v1alpha1.ConvertCustomConfig(orchestratorExplorer.Conf)
		}
		f.configConfigMapName = apicommonv1.GetConfName(dda, f.customConfig, apicommon.DefaultOrchestratorExplorerConf)
		if orchestratorExplorer.Scrubbing != nil {
			f.scrubContainers = apiutils.BoolValue(orchestratorExplorer.Scrubbing.Containers)
		}
		f.extraTags = orchestratorExplorer.ExtraTags
		if orchestratorExplorer.DDUrl != nil {
			f.ddURL = *orchestratorExplorer.DDUrl
		}
		f.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)

		if v1alpha1.IsClusterChecksEnabled(dda) && apiutils.BoolValue(orchestratorExplorer.ClusterCheck) {
			f.clusterChecksEnabled = true

			if v1alpha1.IsCCREnabled(dda) {
				reqComp.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)

				f.rbacSuffix = common.ChecksRunnerSuffix
				f.serviceAccountName = v1alpha1.GetClusterChecksRunnerServiceAccount(dda)
			}
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *orchestratorExplorerFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// Manage the Check Configuration in a configmap
	configCM, err := f.buildOrchestratorExplorerConfigMap()
	if err != nil {
		return err
	}
	if configCM != nil {
		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, configCM); err != nil {
			return err
		}
	}

	// Manager RBAC permission
	rbacName := GetOrchestratorExplorerRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	// Manage orchestrator config in configmap
	vol, volMount := volume.GetCustomConfigSpecVolumes(
		f.customConfig,
		apicommon.OrchestratorExplorerVolumeName,
		f.configConfigMapName,
		orchestratorExplorerCheckFolderName,
	)

	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommonv1.ClusterAgentContainerName)
	managers.Volume().AddVolume(&vol)

	for _, env := range f.getEnvVars() {
		managers.EnvVar().AddEnvVar(env)
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	for _, env := range f.getEnvVars() {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, env)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
