// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesstatecore

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
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
	clusterChecksEnabled bool

	rbacSuffix         string
	serviceAccountName string

	owner               metav1.Object
	customConfig        *apicommonv1.CustomConfig
	configConfigMapName string

	logger logr.Logger
}

// Configure use to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *ksmFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	f.owner = dda
	var output feature.RequiredComponents

	if dda.Spec.Features != nil && dda.Spec.Features.KubeStateMetricsCore != nil && apiutils.BoolValue(dda.Spec.Features.KubeStateMetricsCore.Enabled) {
		output.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)

		if dda.Spec.Features.KubeStateMetricsCore.Conf != nil {
			f.customConfig = v2alpha1.ConvertCustomConfig(dda.Spec.Features.KubeStateMetricsCore.Conf)
		}

		f.configConfigMapName = apicommonv1.GetConfName(dda, f.customConfig, apicommon.DefaultKubeStateMetricsCoreConf)

		if dda.Spec.Features.ClusterChecks != nil && apiutils.BoolValue(dda.Spec.Features.ClusterChecks.Enabled) {
			f.clusterChecksEnabled = true

			if apiutils.BoolValue(dda.Spec.Features.ClusterChecks.UseClusterChecksRunners) {
				f.rbacSuffix = common.ChecksRunnerSuffix
				f.serviceAccountName = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)
				output.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
			} else {
				f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)
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
			f.clusterChecksEnabled = true

			if apiutils.BoolValue(dda.Spec.ClusterChecksRunner.Enabled) {
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
	// Manage the Check Configuration in a configmap
	configCM, err := f.buildKSMCoreConfigMap()
	if err != nil {
		return err
	}
	if configCM != nil {
		managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, configCM)
	}

	// Manager RBAC permission
	rbacName := GetKubeStateMetricsRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRules("", rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *ksmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	// Manage KSM config in configmap
	vol, volMount := volume.GetCustomConfigSpecVolumes(
		f.customConfig,
		apicommon.KubeStateMetricCoreVolumeName,
		f.configConfigMapName,
		ksmCoreCheckFolderName,
	)

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
