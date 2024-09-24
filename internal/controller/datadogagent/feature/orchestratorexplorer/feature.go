// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
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

	if options != nil {
		orchestratorExplorerFeat.logger = options.Logger
	}

	return orchestratorExplorerFeat
}

type orchestratorExplorerFeature struct {
	runInClusterChecksRunner bool
	scrubContainers          bool
	extraTags                []string
	ddURL                    string
	rbacSuffix               string
	serviceAccountName       string
	owner                    metav1.Object
	customConfig             *v2alpha1.CustomConfig
	customResources          []string
	configConfigMapName      string

	logger                      logr.Logger
	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	processAgentRequired bool
}

const NoProcessAgentMinVersion = "7.51.0-0"

// ID returns the ID of the Feature
func (f *orchestratorExplorerFeature) ID() feature.IDType {
	return feature.OrchestratorExplorerIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *orchestratorExplorerFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	orchestratorExplorer := dda.Spec.Features.OrchestratorExplorer

	if orchestratorExplorer != nil && apiutils.BoolValue(orchestratorExplorer.Enabled) {
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqContainers := []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}

		// Process Agent is not required as of agent version 7.51.0
		if nodeAgent, ok := dda.Spec.Override[v2alpha1.NodeAgentComponentName]; ok {
			if nodeAgent.Image != nil && !utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), NoProcessAgentMinVersion) {
				f.processAgentRequired = true
				reqContainers = append(reqContainers, apicommon.ProcessAgentContainerName)
			}
		}

		reqComp.Agent = feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: reqContainers,
		}

		if orchestratorExplorer.Conf != nil {
			f.customConfig = orchestratorExplorer.Conf
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for orchestrator explorer custom config")
			} else {
				f.logger.V(2).Info("built orchestrator explorer from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.OrchestratorExplorerIDType)
		}
		f.customResources = dda.Spec.Features.OrchestratorExplorer.CustomResources
		f.configConfigMapName = v2alpha1.GetConfName(dda, f.customConfig, v2alpha1.DefaultOrchestratorExplorerConf)
		f.scrubContainers = apiutils.BoolValue(orchestratorExplorer.ScrubContainers)
		f.extraTags = orchestratorExplorer.ExtraTags
		if orchestratorExplorer.DDUrl != nil {
			f.ddURL = *orchestratorExplorer.DDUrl
		}
		f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)

		if v2alpha1.IsClusterChecksEnabled(dda) {
			if v2alpha1.IsCCREnabled(dda) {
				f.runInClusterChecksRunner = true
				f.rbacSuffix = common.ChecksRunnerSuffix
				f.serviceAccountName = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)
				reqComp.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
			}
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *orchestratorExplorerFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// Create a configMap if CustomConfig.ConfigData is provided and CustomConfig.ConfigMap == nil,
	// OR if the default configMap is needed.
	configCM, err := f.buildOrchestratorExplorerConfigMap()
	if err != nil {
		return err
	}
	if configCM != nil {
		// Add md5 hash annotation for custom config
		if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
			annotations := object.MergeAnnotationsLabels(f.logger, configCM.GetAnnotations(), map[string]string{f.customConfigAnnotationKey: f.customConfigAnnotationValue}, "*")
			configCM.SetAnnotations(annotations)
		}
		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, configCM); err != nil {
			return err
		}
	}

	// Manage RBAC permission
	rbacName := GetOrchestratorExplorerRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules(f.logger, f.customResources))
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	// Manage orchestrator config in configmap
	var vol corev1.Volume
	var volMount corev1.VolumeMount
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		// Custom config is referenced via ConfigMap
		vol, volMount = volume.GetVolumesFromConfigMap(
			f.customConfig.ConfigMap,
			apicommon.OrchestratorExplorerVolumeName,
			f.configConfigMapName,
			orchestratorExplorerFolderName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(f.configConfigMapName, apicommon.OrchestratorExplorerVolumeName)

		volMount = corev1.VolumeMount{
			Name:      apicommon.OrchestratorExplorerVolumeName,
			MountPath: fmt.Sprintf("%s%s/%s", apicommon.ConfigVolumePath, apicommon.ConfdVolumePath, orchestratorExplorerFolderName),
			ReadOnly:  true,
		}
	}

	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.ClusterAgentContainerName)
	managers.Volume().AddVolume(&vol)

	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}

	for _, env := range f.getEnvVars() {
		managers.EnvVar().AddEnvVar(env)
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	for _, env := range f.getEnvVars() {
		managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, env)
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	for _, env := range f.getEnvVars() {
		if f.processAgentRequired {
			managers.EnvVar().AddEnvVarToContainer(apicommon.ProcessAgentContainerName, env)
		}
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, env)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	if f.runInClusterChecksRunner {
		for _, env := range f.getEnvVars() {
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterChecksRunnersContainerName, env)
		}
	}

	return nil
}
