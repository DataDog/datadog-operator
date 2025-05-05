// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package helmcheck

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
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

	owner                 metav1.Object
	config                *corev1.ConfigMap
	configMapName         string
	configAnnotationKey   string
	configAnnotationValue string

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
		reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
		reqComp.Agent.IsRequired = apiutils.NewBoolPointer(true)
		reqComp.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}

		f.configMapName = fmt.Sprintf("%s-%s", f.owner.GetName(), defaultHelmCheckConf)
		f.collectEvents = apiutils.BoolValue(helmCheck.CollectEvents)
		f.valuesAsTags = helmCheck.ValuesAsTags
		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda)

		if constants.IsClusterChecksEnabled(dda) && constants.IsCCREnabled(dda) {
			f.runInClusterChecksRunner = true
			f.rbacSuffix = common.ChecksRunnerSuffix
			f.serviceAccountName = constants.GetClusterChecksRunnerServiceAccount(dda)
			reqComp.ClusterChecksRunner.IsRequired = apiutils.NewBoolPointer(true)
			reqComp.ClusterChecksRunner.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
		}

		// Build configMap based on feature flags.
		cm, err := f.buildHelmCheckConfigMap()
		if err != nil {
			f.logger.Error(err, "couldn't generate configMap for helm check")
		}
		f.config = cm

		// Create hash based on configMap.
		hash, err := comparison.GenerateMD5ForSpec(cm.Data)
		if err != nil {
			f.logger.Error(err, "couldn't generate hash for helm check config")
		} else {
			f.logger.V(2).Info("built helm check from config", "hash", hash)
		}

		f.configAnnotationValue = hash
		f.configAnnotationKey = object.GetChecksumAnnotationKey(feature.HelmCheckIDType)
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *helmCheckFeature) ManageDependencies(managers feature.ResourceManagers) error {
	if f.config != nil {
		// Add md5 hash annotation for configMap
		if f.configAnnotationKey != "" && f.configAnnotationValue != "" {
			annotations := object.MergeAnnotationsLabels(f.logger, f.config.GetAnnotations(), map[string]string{f.configAnnotationKey: f.configAnnotationValue}, "*")
			f.config.SetAnnotations(annotations)
		}
		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, f.config); err != nil {
			return err
		}
	}

	// Manage RBAC permission
	rbacName := getHelmCheckRBACResourceName(f.owner, f.rbacSuffix)

	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, helmCheckRBACPolicyRules)
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *helmCheckFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	// Manage Helm check config in configMap
	var vol corev1.Volume
	var volMount corev1.VolumeMount
	// Mount default volumes for configMap
	vol = volume.GetBasicVolume(f.configMapName, helmCheckConfigVolumeName)
	volMount = corev1.VolumeMount{
		Name:      helmCheckConfigVolumeName,
		MountPath: fmt.Sprintf("%s%s/%s", common.ConfigVolumePath, common.ConfdVolumePath, helmCheckFolderName),
		ReadOnly:  true,
	}

	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.ClusterAgentContainerName)
	managers.Volume().AddVolume(&vol)

	// Add md5 hash annotation for configMap
	if f.configAnnotationKey != "" && f.configAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.configAnnotationKey, f.configAnnotationValue)
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *helmCheckFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
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
