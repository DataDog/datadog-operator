// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"
	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	err := feature.Register(feature.HelmCheckIDType, buildHelmCheckFeature)
	if err != nil {
		panic(err)
	}
}

func buildHelmCheckFeature(options *feature.Options) feature.Feature {
	helmCheckFeat := &helmCheckFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}

	if options != nil {
		helmCheckFeat.logger = options.Logger
	}
	return helmCheckFeat
}

type helmCheckFeature struct {
	runInClusterChecksRunner bool
	collectEvents            bool
	valuesAsTags             map[string]string

	serviceAccountName string
	rbacSuffix         string

	owner                       metav1.Object
	customConfig                *apicommonv1.CustomConfig
	configMapName               string
	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	logger logr.Logger
}

// ID returns the ID of the Feature
func (f *helmCheckFeature) ID() feature.IDType {
	return feature.HelmCheckIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *helmCheckFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	helmCheck := dda.Spec.Features.HelmCheck

	if helmCheck != nil && apiutils.BoolValue(helmCheck.Enabled) {
		reqComp.ClusterAgent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.Agent.IsRequired = apiutils.NewBoolPointer(true)

		if helmCheck.Conf != nil {
			f.customConfig = v2alpha1.ConvertCustomConfig(helmCheck.Conf)
			hash, err := comparison.GenerateMD5ForSpec(f.customConfig)
			if err != nil {
				f.logger.Error(err, "couldn't generate hash for helm check custom config")
			} else {
				f.logger.V(2).Info("built helm check from custom config", "hash", hash)
			}
			f.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.HelmCheckIDType)
			f.customConfigAnnotationValue = hash
		}

		f.configMapName = apicommonv1.GetConfName(dda, f.customConfig, apicommon.DefaultHelmCheckConf)
		f.collectEvents = apiutils.BoolValue(helmCheck.CollectEvents)
		f.valuesAsTags = helmCheck.ValuesAsTags
		f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)

		if v2alpha1.IsClusterChecksEnabled(dda) && v2alpha1.IsCCREnabled(dda) {
			f.runInClusterChecksRunner = true
			f.rbacSuffix = common.ChecksRunnerSuffix
			f.serviceAccountName = v2alpha1.GetClusterChecksRunnerServiceAccount(dda)
			reqComp.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *helmCheckFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	return feature.RequiredComponents{}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *helmCheckFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// Create a configMap if CustomConfig.ConfigData is provided and CustomConfig.ConfigMap == nil,
	// OR if the default configMap is needed.
	cm, err := f.buildHelmCheckConfigMap()
	if err != nil {
		return err
	}
	if cm != nil {
		// Add md5 hash annotation for custom config
		if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
			annotations := object.MergeAnnotationsLabels(f.logger, cm.GetAnnotations(), map[string]string{f.customConfigAnnotationKey: f.customConfigAnnotationValue}, "*")
			cm.SetAnnotations(annotations)
		}

		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
			return err
		}
	}

	// Manage RBAC permission
	rbacName := getHelmCheckRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules())
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *helmCheckFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	// Manage Helm check config in configMap
	var vol corev1.Volume
	var volMount corev1.VolumeMount
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		// Custom config is referenced via ConfigMap
		vol, volMount = volume.GetVolumesFromConfigMap(
			f.customConfig.ConfigMap,
			apicommon.HelmCheckConfigVolumeName,
			f.configMapName,
			helmCheckFolderName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(f.configMapName, apicommon.HelmCheckConfigVolumeName)
		volMount = corev1.VolumeMount{
			Name:      apicommon.HelmCheckConfigVolumeName,
			MountPath: fmt.Sprintf("%s%s/%s", apicommon.ConfigVolumePath, apicommon.ConfdVolumePath, helmCheckFolderName),
			ReadOnly:  true,
		}
	}

	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommonv1.ClusterAgentContainerName)
	managers.Volume().AddVolume(&vol)

	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}

	return nil
}

// ManageMultiProcessNodeAgent allows a feature to configure the multi-process container for Node Agent's corev1.PodTemplateSpec
// if multi-process container usage is enabled and can be used with the current feature set
// It should do nothing if the feature doesn't need to configure it.
func (f *helmCheckFeature) ManageMultiProcessNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *helmCheckFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *helmCheckFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
