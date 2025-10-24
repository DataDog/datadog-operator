// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/merger"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

func init() {
	err := feature.Register(feature.KubernetesStateCoreIDType, buildKSMFeature)
	if err != nil {
		panic(err)
	}
}

func buildKSMFeature(options *feature.Options) feature.Feature {
	ksmFeat := &ksmFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}

	if options != nil {
		ksmFeat.logger = options.Logger
	}

	return ksmFeat
}

type ksmFeature struct {
	runInClusterChecksRunner   bool
	collectCRDMetrics          bool
	collectCrMetrics           []v2alpha1.Resource
	collectAPIServiceMetrics   bool
	collectControllerRevisions bool

	rbacSuffix         string
	serviceAccountName string

	owner                       metav1.Object
	customConfig                *v2alpha1.CustomConfig
	configConfigMapName         string
	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	logger logr.Logger
}

// Minimum agent version that supports collection of CRD and APIService data
// Add "-0" so that prerelase versions are considered sufficient. https://github.com/Masterminds/semver#working-with-prerelease-versions
const crdAPIServiceCollectionMinVersion = "7.46.0-0"

// Minimum agent version that supports collection of controllerrevisions
const controllerRevisionsCollectionMinVersion = "7.72.0-0"

// ID returns the ID of the Feature
func (f *ksmFeature) ID() feature.IDType {
	return feature.KubernetesStateCoreIDType
}

// Configure use to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *ksmFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	f.owner = dda
	var output feature.RequiredComponents

	if ddaSpec.Features != nil && ddaSpec.Features.KubeStateMetricsCore != nil && apiutils.BoolValue(ddaSpec.Features.KubeStateMetricsCore.Enabled) {
		f.logger.Info("KubeStateMetricsCore feature enabled")
		output.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		output.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
		output.Agent.IsRequired = apiutils.NewBoolPointer(true)
		output.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}

		f.collectAPIServiceMetrics = true
		f.collectCRDMetrics = true
		f.collectCrMetrics = ddaSpec.Features.KubeStateMetricsCore.CollectCrMetrics
		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

		// Determine CollectControllerRevisions setting
		// Priority: 1) Explicit spec setting, 2) Image override version check, 3) Default image version check
		collectControllerRevisionsExplicitlySet := ddaSpec.Features.KubeStateMetricsCore.CollectControllerRevisions != nil
		controllerRevisionsSetByOverride := false // Track if we determined the value via override

		if collectControllerRevisionsExplicitlySet {
			// Explicit setting in spec - use it (will be validated against version later if override present)
			f.collectControllerRevisions = apiutils.BoolValue(ddaSpec.Features.KubeStateMetricsCore.CollectControllerRevisions)
			f.logger.Info("CollectControllerRevisions explicitly set in spec", "value", f.collectControllerRevisions)
		} else {
			// Not explicitly set - will be determined by version checks below
			f.collectControllerRevisions = false
			f.logger.Info("CollectControllerRevisions not set in spec, will auto-enable if agent version supports it")
		}

		// This check will only run in the Cluster Checks Runners or Cluster Agent (not the Node Agent)
		if ddaSpec.Features.ClusterChecks != nil && apiutils.BoolValue(ddaSpec.Features.ClusterChecks.Enabled) && apiutils.BoolValue(ddaSpec.Features.ClusterChecks.UseClusterChecksRunners) {
			f.runInClusterChecksRunner = true
			f.rbacSuffix = common.ChecksRunnerSuffix
			f.serviceAccountName = constants.GetClusterChecksRunnerServiceAccount(dda.GetName(), ddaSpec)
			output.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
			output.ClusterChecksRunner.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}

			if ccrOverride, ok := ddaSpec.Override[v2alpha1.ClusterChecksRunnerComponentName]; ok {
				if ccrOverride.Image != nil {
					agentVersion := common.GetAgentVersionFromImage(*ccrOverride.Image)
					f.logger.Info("ClusterChecksRunner image override detected", "image", *ccrOverride.Image, "version", agentVersion)

					// CRD and APIService version checks (existing logic - unchanged)
					if !utils.IsAboveMinVersion(agentVersion, crdAPIServiceCollectionMinVersion) {
						f.logger.Info("Disabling CRD and APIService collection due to agent version", "version", agentVersion, "minVersion", crdAPIServiceCollectionMinVersion)
						f.collectAPIServiceMetrics = false
						f.collectCRDMetrics = false
					}

					// ControllerRevisions version check with fallback parsing
					if !utils.IsAboveMinVersionWithFallback(agentVersion, controllerRevisionsCollectionMinVersion) {
						// Version too old - disable even if explicitly set
						if f.collectControllerRevisions {
							f.logger.Info("Disabling ControllerRevisions collection due to agent version (was explicitly enabled but version too old)", "version", agentVersion, "minVersion", controllerRevisionsCollectionMinVersion)
						}
						f.collectControllerRevisions = false
					} else if !collectControllerRevisionsExplicitlySet {
						// Version supports it and not explicitly set - auto-enable
						f.logger.Info("Auto-enabling ControllerRevisions collection based on override agent version", "version", agentVersion, "minVersion", controllerRevisionsCollectionMinVersion)
						f.collectControllerRevisions = true
					}
					controllerRevisionsSetByOverride = true
				}
			}
		} else {
			f.logger.Info("Running in ClusterAgent mode")
			if clusterAgentOverride, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
				if clusterAgentOverride.Image != nil {
					agentVersion := common.GetAgentVersionFromImage(*clusterAgentOverride.Image)
					f.logger.Info("ClusterAgent image override detected", "image", *clusterAgentOverride.Image, "version", agentVersion)

					// CRD and APIService version checks (existing logic - unchanged)
					if !utils.IsAboveMinVersion(agentVersion, crdAPIServiceCollectionMinVersion) {
						f.logger.Info("Disabling CRD and APIService collection due to agent version", "version", agentVersion, "minVersion", crdAPIServiceCollectionMinVersion)
						f.collectAPIServiceMetrics = false
						f.collectCRDMetrics = false
					}

					// ControllerRevisions version check with fallback parsing
					if !utils.IsAboveMinVersionWithFallback(agentVersion, controllerRevisionsCollectionMinVersion) {
						// Version too old - disable even if explicitly set
						if f.collectControllerRevisions {
							f.logger.Info("Disabling ControllerRevisions collection due to cluster agent version (was explicitly enabled but version too old)", "version", agentVersion, "minVersion", controllerRevisionsCollectionMinVersion)
						}
						f.collectControllerRevisions = false
					} else if !collectControllerRevisionsExplicitlySet {
						// Version supports it and not explicitly set - auto-enable
						f.logger.Info("Auto-enabling ControllerRevisions collection based on override cluster agent version", "version", agentVersion, "minVersion", controllerRevisionsCollectionMinVersion)
						f.collectControllerRevisions = true
					}
					controllerRevisionsSetByOverride = true
				}
			}
		}

		// If not explicitly set and not determined by image override, check default versions
		if !collectControllerRevisionsExplicitlySet && !controllerRevisionsSetByOverride {
			// Determine which default version to check based on deployment mode
			var defaultVersion string
			if f.runInClusterChecksRunner {
				defaultVersion = images.AgentLatestVersion
				f.logger.Info("No image override present, checking default agent version for ControllerRevisions support", "version", defaultVersion)
			} else {
				defaultVersion = images.ClusterAgentLatestVersion
				f.logger.Info("No image override present, checking default cluster agent version for ControllerRevisions support", "version", defaultVersion)
			}

			// Check if default version supports controllerrevisions
			if utils.IsAboveMinVersionWithFallback(defaultVersion, controllerRevisionsCollectionMinVersion) {
				f.logger.Info("Auto-enabling ControllerRevisions collection based on default version", "version", defaultVersion, "minVersion", controllerRevisionsCollectionMinVersion)
				f.collectControllerRevisions = true
			} else {
				f.logger.Info("Default version does not support ControllerRevisions collection, keeping disabled", "version", defaultVersion, "minVersion", controllerRevisionsCollectionMinVersion)
			}
		}

		if ddaSpec.Features.KubeStateMetricsCore.Conf != nil {
			f.customConfig = ddaSpec.Features.KubeStateMetricsCore.Conf
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for ksm core custom config")
			} else {
				f.logger.V(2).Info("built ksm core from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.KubernetesStateCoreIDType)
		}

		f.configConfigMapName = constants.GetConfName(dda, f.customConfig, defaultKubeStateMetricsCoreConf)

		// Log final configuration state
		f.logger.Info("KubeStateMetricsCore configuration finalized",
			"collectAPIServiceMetrics", f.collectAPIServiceMetrics,
			"collectCRDMetrics", f.collectCRDMetrics,
			"collectControllerRevisions", f.collectControllerRevisions,
			"runInClusterChecksRunner", f.runInClusterChecksRunner)
	}

	return output
}

type collectorOptions struct {
	enableVPA                 bool
	enableAPIService          bool
	enableCRD                 bool
	enableControllerRevisions bool
	customResources           []v2alpha1.Resource
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *ksmFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	// Create a configMap if CustomConfig.ConfigData is provided and CustomConfig.ConfigMap == nil,
	// OR if the default configMap is needed.
	pInfo := managers.Store().GetPlatformInfo()
	collectorOpts := collectorOptions{
		enableVPA:                 pInfo.IsResourceSupported("VerticalPodAutoscaler"),
		enableAPIService:          f.collectAPIServiceMetrics,
		enableCRD:                 f.collectCRDMetrics,
		enableControllerRevisions: f.collectControllerRevisions,
		customResources:           f.collectCrMetrics,
	}
	configCM, err := f.buildKSMCoreConfigMap(collectorOpts)
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
	rbacName := GetKubeStateMetricsRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules(collectorOpts))
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	// Manage KSM config in configmap
	var vol corev1.Volume
	var volMount corev1.VolumeMount
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		// Custom config is referenced via ConfigMap
		vol, volMount = volume.GetVolumesFromConfigMap(
			f.customConfig.ConfigMap,
			ksmCoreVolumeName,
			f.configConfigMapName,
			ksmCoreCheckFolderName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(f.configConfigMapName, ksmCoreVolumeName)
		volMount = corev1.VolumeMount{
			Name:      ksmCoreVolumeName,
			MountPath: fmt.Sprintf("%s%s/%s", common.ConfigVolumePath, common.ConfdVolumePath, ksmCoreCheckFolderName),
			ReadOnly:  true,
		}
	}
	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.ClusterAgentContainerName)
	managers.Volume().AddVolume(&vol)

	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  DDKubeStateMetricsCoreEnabled,
		Value: "true",
	})

	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  DDKubeStateMetricsCoreConfigMap,
		Value: f.configConfigMapName,
	})

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Remove ksm v1 conf if the cluster checks are enabled and the ksm core is enabled
	ignoreAutoConf := &corev1.EnvVar{
		Name:  DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}

	return managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.UnprivilegedSingleAgentContainerName, ignoreAutoConf, merger.AppendToValueEnvVarMergeFunction)
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Remove ksm v1 conf if the cluster checks are enabled and the ksm core is enabled
	ignoreAutoConf := &corev1.EnvVar{
		Name:  DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}

	return managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.CoreAgentContainerName, ignoreAutoConf, merger.AppendToValueEnvVarMergeFunction)
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
