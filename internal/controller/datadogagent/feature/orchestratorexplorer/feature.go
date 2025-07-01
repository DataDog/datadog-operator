// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package orchestratorexplorer

import (
	"fmt"
	"slices"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

const (
	currentDatadogPodAutoscalerResource = "datadoghq.com/v1alpha2/datadogpodautoscalers"
	oldDatadogPodAutoscalerResource     = "datadoghq.com/v1alpha1/datadogpodautoscalers"
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
	enabled                  bool
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
func (f *orchestratorExplorerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, ddaRCStatus *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// Merge configuration from Status.RemoteConfigConfiguration into the Spec
	f.mergeConfigs(ddaSpec, ddaRCStatus)

	orchestratorExplorer := ddaSpec.Features.OrchestratorExplorer

	if orchestratorExplorer != nil && apiutils.BoolValue(orchestratorExplorer.Enabled) {
		f.enabled = true
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.Agent.IsRequired = apiutils.NewBoolPointer(true)

		if orchestratorExplorer.Conf != nil || len(orchestratorExplorer.CustomResources) > 0 {
			f.customConfig = orchestratorExplorer.Conf

			// Used to force restart of DCA
			// use entire orchestratorExplorer to handle custom config and CRDs
			hash, err := comparison.GenerateMD5ForSpec(orchestratorExplorer)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for orchestrator explorer custom config")
			} else {
				f.logger.V(2).Info("built orchestrator explorer from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.OrchestratorExplorerIDType)
		}

		f.customResources = ddaSpec.Features.OrchestratorExplorer.CustomResources
		f.configConfigMapName = constants.GetConfName(dda, f.customConfig, defaultOrchestratorExplorerConf)
		f.scrubContainers = apiutils.BoolValue(orchestratorExplorer.ScrubContainers)
		f.extraTags = orchestratorExplorer.ExtraTags
		if orchestratorExplorer.DDUrl != nil {
			f.ddURL = *orchestratorExplorer.DDUrl
		}
		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

		// Handle automatic addition of OOTB resources
		// Autoscaling: Add DPA resource if enabled and replace older versions if present
		autoscaling := ddaSpec.Features.Autoscaling
		if autoscaling != nil && autoscaling.Workload != nil && apiutils.BoolValue(autoscaling.Workload.Enabled) {
			addRequired := true
			for i := range f.customResources {
				if f.customResources[i] == oldDatadogPodAutoscalerResource {
					f.customResources[i] = currentDatadogPodAutoscalerResource
					addRequired = false
				}
			}
			if addRequired {
				f.customResources = append(f.customResources, currentDatadogPodAutoscalerResource)
			}
		}

		// Unique the custom resources as the check will output a warning if there are duplicates
		slices.Sort(f.customResources)
		f.customResources = slices.Compact(f.customResources)

		if constants.IsClusterChecksEnabled(ddaSpec) {
			if constants.IsCCREnabled(ddaSpec) {
				f.runInClusterChecksRunner = true
				f.rbacSuffix = common.ChecksRunnerSuffix
				f.serviceAccountName = constants.GetClusterChecksRunnerServiceAccount(dda.GetName(), ddaSpec)
				reqComp.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
			}
		}
	}

	reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
	reqContainers := []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	// Process Agent is not required as of agent version 7.51.0
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil && !utils.IsAboveMinVersion(common.GetAgentVersionFromImage(*nodeAgent.Image), NoProcessAgentMinVersion) {
			f.processAgentRequired = true
			reqContainers = append(reqContainers, apicommon.ProcessAgentContainerName)
		}
	}
	reqComp.Agent.Containers = reqContainers

	if f.runInClusterChecksRunner {
		reqComp.ClusterChecksRunner.Containers = []apicommon.AgentContainerName{apicommon.ClusterChecksRunnersContainerName}
	}

	return reqComp
}

func (f *orchestratorExplorerFeature) mergeConfigs(ddaSpec *v2alpha1.DatadogAgentSpec, ddaStatus *v2alpha1.RemoteConfigConfiguration) {
	if ddaStatus == nil ||
		ddaStatus.Features == nil ||
		ddaStatus.Features.OrchestratorExplorer == nil ||
		ddaStatus.Features.OrchestratorExplorer.CustomResources == nil {
		return
	}

	if ddaSpec.Features == nil {
		ddaSpec.Features = &v2alpha1.DatadogFeatures{}
	}

	if ddaSpec.Features.OrchestratorExplorer == nil {
		ddaSpec.Features.OrchestratorExplorer = &v2alpha1.OrchestratorExplorerFeatureConfig{}
	}

	ddaSpec.Features.OrchestratorExplorer.CustomResources = append(ddaSpec.Features.OrchestratorExplorer.CustomResources, ddaStatus.Features.OrchestratorExplorer.CustomResources...)
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *orchestratorExplorerFeature) ManageDependencies(managers feature.ResourceManagers) error {
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
	// Add the env var to explicitly disable this feature
	// Otherwise, this feature is enabled by default in the Agent code
	managers.EnvVar().AddEnvVar(f.getEnabledEnvVar())
	if !f.enabled {
		return nil
	}

	// Manage orchestrator config in configmap
	var vol corev1.Volume
	var volMount corev1.VolumeMount
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		// Custom config is referenced via ConfigMap
		vol, volMount = volume.GetVolumesFromConfigMap(
			f.customConfig.ConfigMap,
			orchestratorExplorerVolumeName,
			f.configConfigMapName,
			orchestratorExplorerFolderName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(f.configConfigMapName, orchestratorExplorerVolumeName)

		volMount = corev1.VolumeMount{
			Name:      orchestratorExplorerVolumeName,
			MountPath: fmt.Sprintf("%s%s/%s", common.ConfigVolumePath, common.ConfdVolumePath, orchestratorExplorerFolderName),
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
	// Add the env var to explicitly disable this feature
	// Otherwise, this feature is enabled by default in the Agent code
	managers.EnvVar().AddEnvVar(f.getEnabledEnvVar())
	if !f.enabled {
		return nil
	}

	for _, env := range f.getEnvVars() {
		managers.EnvVar().AddEnvVarToContainer(apicommon.UnprivilegedSingleAgentContainerName, env)
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	containers := []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	if f.processAgentRequired {
		containers = append(containers, apicommon.ProcessAgentContainerName)
	}

	// Add the env var to explicitly disable this feature
	// Otherwise, this feature is enabled by default in the Agent code
	managers.EnvVar().AddEnvVarToContainers(containers, f.getEnabledEnvVar())
	if !f.enabled {
		return nil
	}

	for _, env := range f.getEnvVars() {
		managers.EnvVar().AddEnvVarToContainers(containers, env)
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *orchestratorExplorerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	if f.runInClusterChecksRunner {
		// Add the env var to explicitly disable this feature
		// Otherwise, this feature is enabled by default in the Agent code
		managers.EnvVar().AddEnvVar(f.getEnabledEnvVar())
		if !f.enabled {
			return nil
		}

		for _, env := range f.getEnvVars() {
			managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterChecksRunnersContainerName, env)
		}
	}

	return nil
}
