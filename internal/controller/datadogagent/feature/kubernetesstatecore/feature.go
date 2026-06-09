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
	"k8s.io/utils/ptr"

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

	// podCollectionOnNode is true when PodCollectionMode=node_kubelet has
	// been requested AND the agent version is compatible.
	podCollectionOnNode bool
	// podCollectionOnNodeUserConfig is true when the user supplied their
	// own cluster-side config via .Conf alongside PodCollectionMode=node_kubelet.
	// In that case the operator deploys the node-side check but does NOT
	// mutate the user's cluster-side YAML; the user owns cluster_unassigned.
	podCollectionOnNodeUserConfig bool
	// nodeAgentConfigMapName is the name of the operator-managed ConfigMap
	// holding the pods-only check for node agents (only populated when
	// podCollectionOnNode is true).
	nodeAgentConfigMapName string

	rbacSuffix         string
	serviceAccountName string

	owner                       metav1.Object
	customConfig                *v2alpha1.CustomConfig
	configConfigMapName         string
	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	logger logr.Logger
}

const (
	// Minimum agent version that supports collection of CRD and APIService data
	// Add "-0" so that prerelase versions are considered sufficient. https://github.com/Masterminds/semver#working-with-prerelease-versions
	crdAPIServiceCollectionMinVersion = "7.46.0-0"

	// Minimum agent version that supports collection of controllerrevisions
	controllerRevisionsCollectionMinVersion = "7.72.0-0"
)

// ID returns the ID of the Feature
func (f *ksmFeature) ID() feature.IDType {
	return feature.KubernetesStateCoreIDType
}

// Configure use to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *ksmFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	f.owner = dda
	var output feature.RequiredComponents

	if ddaSpec.Features != nil && ddaSpec.Features.KubeStateMetricsCore != nil && apiutils.BoolValue(ddaSpec.Features.KubeStateMetricsCore.Enabled) {
		output.ClusterAgent.IsRequired = ptr.To(true)
		output.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
		output.Agent.IsRequired = ptr.To(true)
		output.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}

		f.collectAPIServiceMetrics = true
		f.collectCRDMetrics = true
		f.collectCrMetrics = ddaSpec.Features.KubeStateMetricsCore.CollectCrMetrics
		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

		// Determine CollectControllerRevisions setting
		// Default to true, then check version requirements
		f.collectControllerRevisions = true

		// This check will only run in the Cluster Checks Runners or Cluster Agent (not the Node Agent)
		if ddaSpec.Features.ClusterChecks != nil && apiutils.BoolValue(ddaSpec.Features.ClusterChecks.Enabled) && apiutils.BoolValue(ddaSpec.Features.ClusterChecks.UseClusterChecksRunners) {
			f.runInClusterChecksRunner = true
			f.rbacSuffix = common.ChecksRunnerSuffix
			f.serviceAccountName = constants.GetClusterChecksRunnerServiceAccount(dda.GetName(), ddaSpec)
			output.ClusterChecksRunner.IsRequired = ptr.To(true)
			output.ClusterChecksRunner.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}

			if ccrOverride, ok := ddaSpec.Override[v2alpha1.ClusterChecksRunnerComponentName]; ok {
				if ccrOverride.Image != nil {
					agentVersion := common.GetAgentVersionFromImage(*ccrOverride.Image)

					// CRD and APIService version checks
					if !utils.IsAboveMinVersion(agentVersion, crdAPIServiceCollectionMinVersion, nil) {
						f.collectAPIServiceMetrics = false
						f.collectCRDMetrics = false
					}

					// ControllerRevisions version check - enable if version supports it
					fallback := false
					if !utils.IsAboveMinVersion(agentVersion, controllerRevisionsCollectionMinVersion, &fallback) {
						f.collectControllerRevisions = false
					}
				}
			}
		} else {
			if clusterAgentOverride, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
				if clusterAgentOverride.Image != nil {
					agentVersion := common.GetAgentVersionFromImage(*clusterAgentOverride.Image)

					// CRD and APIService version checks
					if !utils.IsAboveMinVersion(agentVersion, crdAPIServiceCollectionMinVersion, nil) {
						f.collectAPIServiceMetrics = false
						f.collectCRDMetrics = false
					}

					// ControllerRevisions version check - enable if version supports it
					fallback := false
					if !utils.IsAboveMinVersion(agentVersion, controllerRevisionsCollectionMinVersion, &fallback) {
						f.collectControllerRevisions = false
					}
				}
			}
		}

		// Capture the user-supplied custom config (if any) so PodCollectionMode
		// resolution below can decide whether to mutate the cluster-side YAML.
		if ddaSpec.Features.KubeStateMetricsCore.Conf != nil {
			f.customConfig = ddaSpec.Features.KubeStateMetricsCore.Conf
		}

		// Resolve PodCollectionMode. When node_kubelet is requested AND every
		// component that loads the check is version-compatible, switch the
		// cluster-side instance to pod_collection_mode: cluster_unassigned
		// (only when the operator owns the cluster-side config) and deploy a
		// pods-only check to every node agent. When the user supplies their
		// own .Conf, the operator still deploys the node-side check but does
		// not mutate the user's YAML; they are responsible for setting
		// cluster_unassigned themselves.
		if mode := ddaSpec.Features.KubeStateMetricsCore.PodCollectionMode; mode != nil &&
			*mode == v2alpha1.KSMPodCollectionModeNodeKubelet {
			f.podCollectionOnNode = true
			// Version compatibility check on BOTH sides: the cluster-side
			// component that runs the cluster_unassigned check (CCR if cluster-
			// checks-runners are enabled, otherwise the cluster-agent) AND the
			// node-agent that runs the node_kubelet check. If either image tag
			// is parseable AND below the supported floor, skip the feature so
			// the operator doesn't mount an unsupported file into a node-agent
			// that would silently fall back to default mode and double-collect.
			// Unparseable tags (`:dev`, custom registries, etc.) are assumed
			// compatible — matches the existing pattern for the apiservices/CRD
			// checks above.
			componentsToCheck := []v2alpha1.ComponentName{v2alpha1.NodeAgentComponentName}
			if f.runInClusterChecksRunner {
				componentsToCheck = append(componentsToCheck, v2alpha1.ClusterChecksRunnerComponentName)
			} else {
				componentsToCheck = append(componentsToCheck, v2alpha1.ClusterAgentComponentName)
			}
			for _, comp := range componentsToCheck {
				ovr, ok := ddaSpec.Override[comp]
				if !ok || ovr == nil || ovr.Image == nil {
					continue
				}
				agentVersion := common.GetAgentVersionFromImage(*ovr.Image)
				fallback := true // assume compatible when unparseable
				if !utils.IsAboveMinVersion(agentVersion, podCollectionOnNodeMinVersion, &fallback) {
					f.logger.Info(
						"PodCollectionMode=node_kubelet requires agent >= 7.60; falling back to default",
						"component", string(comp),
						"version", agentVersion,
					)
					f.podCollectionOnNode = false
					break
				}
			}
			if f.podCollectionOnNode {
				f.podCollectionOnNodeUserConfig = f.customConfig != nil
				f.nodeAgentConfigMapName = constants.GetConfName(dda, nil, defaultKSMPodsOnNodeConf)
				if f.podCollectionOnNodeUserConfig {
					f.logger.Info(
						"PodCollectionMode=node_kubelet was set alongside features.kubeStateMetricsCore.conf; " +
							"the operator will deploy the node-side check but will not modify the user-supplied " +
							"cluster-side config. To avoid double pod collection ensure the cluster-side instance " +
							"either omits `pods` from `collectors` OR sets `pod_collection_mode: cluster_unassigned`. " +
							"Note that omitting `collectors` entirely falls back to upstream KSM defaults, which " +
							"include `pods`.",
					)
				}
			}
		}

		// Compute the checksum annotation. With f.podCollectionOnNode resolved
		// above, toggling the field changes the input here, which propagates
		// to the cluster-agent pod-template annotation and forces a rollout.
		if f.customConfig != nil {
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for ksm core custom config")
			} else {
				f.logger.V(2).Info("built ksm core from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.KubernetesStateCoreIDType)
		} else {
			// Dynamic checksum for the default configuration. Includes every
			// input that affects the rendered cluster-side ConfigMap so that
			// toggling any of them forces a rollout of the consumer.
			defaultConfigData := map[string]any{
				"collect_crds":           f.collectCRDMetrics,
				"collect_apiservices":    f.collectAPIServiceMetrics,
				"collect_cr_metrics":     f.collectCrMetrics,
				"pod_collection_on_node": f.podCollectionOnNode,
			}

			hash, err := comparison.GenerateMD5ForSpec(defaultConfigData)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for default ksm core config")
			} else {
				f.logger.V(2).Info("generated default ksm core config hash", "hash", hash, "config", defaultConfigData)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.KubernetesStateCoreIDType)
		}

		f.configConfigMapName = constants.GetConfName(dda, f.customConfig, defaultKubeStateMetricsCoreConf)
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
func (f *ksmFeature) ManageDependencies(managers feature.ResourceManagers) error {
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

	// When PodCollectionMode=node_kubelet, ship a separate pods-only check
	// to every node agent regardless of whether the cluster-side config was
	// user-supplied.
	if f.podCollectionOnNode {
		nodeCM := f.buildKSMCorePodsOnNodeConfigMap()
		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, nodeCM); err != nil {
			return err
		}
	}

	// Manage RBAC permission
	rbacName := GetKubeStateMetricsRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules(collectorOpts))
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
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
func (f *ksmFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	// Remove ksm v1 conf if the cluster checks are enabled and the ksm core is enabled
	ignoreAutoConf := &corev1.EnvVar{
		Name:  DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}

	if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.UnprivilegedSingleAgentContainerName, ignoreAutoConf, merger.AppendToValueEnvVarMergeFunction); err != nil {
		return err
	}

	f.mountPodsOnNodeCheck(managers, apicommon.UnprivilegedSingleAgentContainerName)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// Remove ksm v1 conf if the cluster checks are enabled and the ksm core is enabled
	ignoreAutoConf := &corev1.EnvVar{
		Name:  DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}

	if err := managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommon.CoreAgentContainerName, ignoreAutoConf, merger.AppendToValueEnvVarMergeFunction); err != nil {
		return err
	}

	f.mountPodsOnNodeCheck(managers, apicommon.CoreAgentContainerName)
	return nil
}

// mountPodsOnNodeCheck mounts the node-side pods-only KSM check into the given
// container when PodCollectionMode=node_kubelet is enabled.
func (f *ksmFeature) mountPodsOnNodeCheck(managers feature.PodTemplateManagers, containerName apicommon.AgentContainerName) {
	if !f.podCollectionOnNode {
		return
	}
	vol := volume.GetBasicVolume(f.nodeAgentConfigMapName, ksmCorePodsOnNodeVolumeName)
	volMount := corev1.VolumeMount{
		Name:      ksmCorePodsOnNodeVolumeName,
		MountPath: fmt.Sprintf("%s%s/%s", common.ConfigVolumePath, common.ConfdVolumePath, ksmCoreCheckFolderName),
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, containerName)
	managers.Volume().AddVolume(&vol)
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}

func (f *ksmFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}
