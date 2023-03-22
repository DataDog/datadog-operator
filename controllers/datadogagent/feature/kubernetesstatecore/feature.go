// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	common "github.com/DataDog/datadog-operator/controllers/datadogagent/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/merger"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
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
	runInClusterChecksRunner bool

	rbacSuffix         string
	serviceAccountName string

	owner                       metav1.Object
	customConfig                *apicommonv1.CustomConfig
	configConfigMapName         string
	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	logger logr.Logger
}

// ID returns the ID of the Feature
func (f *ksmFeature) ID() feature.IDType {
	return feature.KubernetesStateCoreIDType
}

// Configure use to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *ksmFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	var output feature.RequiredComponents

	if dda.Spec.Features != nil && dda.Spec.Features.KubeStateMetricsCore != nil && apiutils.BoolValue(dda.Spec.Features.KubeStateMetricsCore.Enabled) {
		output.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)

		if dda.Spec.Features.KubeStateMetricsCore.Conf != nil {
			f.customConfig = v2alpha1.ConvertCustomConfig(dda.Spec.Features.KubeStateMetricsCore.Conf)
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for ksm core custom config")
			} else {
				f.logger.V(2).Info("built ksm core from custom config", "hash", hash)
			}
			f.customConfigAnnotationValue = hash
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.KubernetesStateCoreIDType)
		}

		f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)
		f.configConfigMapName = apicommonv1.GetConfName(dda, f.customConfig, apicommon.DefaultKubeStateMetricsCoreConf)

		if dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.Enabled) {
			if apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners) {
				f.runInClusterChecksRunner = true
				f.rbacSuffix = common.ChecksRunnerSuffix
				f.serviceAccountName = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)
				output.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
			}
		}
	}

	return output
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *ksmFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	var output feature.RequiredComponents

	if dda.Spec.Features.KubeStateMetricsCore != nil && apiutils.BoolValue(dda.Spec.Features.KubeStateMetricsCore.Enabled) {
		output.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)

		if dda.Spec.ClusterAgent.Config != nil && apiutils.BoolValue(dda.Spec.ClusterAgent.Config.ClusterChecksEnabled) && apiutils.BoolValue(dda.Spec.Features.KubeStateMetricsCore.ClusterCheck) {
			if apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
				f.runInClusterChecksRunner = true
				output.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)

				f.rbacSuffix = common.ChecksRunnerSuffix
				f.serviceAccountName = v1alpha1.GetClusterChecksRunnerServiceAccount(dda)
			}
		} else {
			f.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)
		}

		if dda.Spec.Features.KubeStateMetricsCore.Conf != nil {
			f.customConfig = v1alpha1.ConvertCustomConfig(dda.Spec.Features.KubeStateMetricsCore.Conf)
		}

		f.configConfigMapName = apicommonv1.GetConfName(dda, f.customConfig, apicommon.DefaultKubeStateMetricsCoreConf)
	}

	return output
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *ksmFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// Create a configMap if CustomConfig.ConfigData is provided and CustomConfig.ConfigMap == nil,
	// OR if the default configMap is needed.
	pInfo := managers.Store().GetPlatformInfo()
	configCM, err := f.buildKSMCoreConfigMap(pInfo.IsResourceSupported("VerticalPodAutoscaler"))
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

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
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
			apicommon.KubeStateMetricCoreVolumeName,
			f.configConfigMapName,
			ksmCoreCheckFolderName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(f.configConfigMapName, apicommon.KubeStateMetricCoreVolumeName)
		volMount = corev1.VolumeMount{
			Name:      apicommon.KubeStateMetricCoreVolumeName,
			MountPath: fmt.Sprintf("%s%s/%s", apicommon.ConfigVolumePath, apicommon.ConfdVolumePath, ksmCoreCheckFolderName),
			ReadOnly:  true,
		}
	}
	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommonv1.ClusterAgentContainerName)
	managers.Volume().AddVolume(&vol)

	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDKubeStateMetricsCoreEnabled,
		Value: "true",
	})

	managers.EnvVar().AddEnvVar(&corev1.EnvVar{
		Name:  apicommon.DDKubeStateMetricsCoreConfigMap,
		Value: f.configConfigMapName,
	})

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// Remove ksm v1 conf if the cluster checks are enabled and the ksm core is enabled
	ignoreAutoConf := &corev1.EnvVar{
		Name:  apicommon.DDIgnoreAutoConf,
		Value: "kubernetes_state",
	}

	return managers.EnvVar().AddEnvVarToContainerWithMergeFunc(apicommonv1.CoreAgentContainerName, ignoreAutoConf, merger.AppendToValueEnvVarMergeFunction)
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
