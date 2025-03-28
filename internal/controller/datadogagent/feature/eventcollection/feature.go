// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	common "github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	"github.com/DataDog/datadog-operator/pkg/secrets"
)

func init() {
	err := feature.Register(feature.EventCollectionIDType, buildEventCollectionFeature)
	if err != nil {
		panic(err)
	}
}

func buildEventCollectionFeature(options *feature.Options) feature.Feature {
	eventCollectionFeat := &eventCollectionFeature{}

	if options != nil {
		eventCollectionFeat.logger = options.Logger
	}

	return eventCollectionFeat
}

type eventCollectionFeature struct {
	serviceAccountName string
	rbacSuffix         string
	owner              metav1.Object

	configMapName      string
	unbundleEvents     bool
	unbundleEventTypes []v2alpha1.EventTypes

	cmAnnotationKey   string
	cmAnnotationValue string

	logger logr.Logger
}

// ID returns the ID of the Feature
func (f *eventCollectionFeature) ID() feature.IDType {
	return feature.EventCollectionIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *eventCollectionFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// v2alpha1 configures event collection using the cluster agent only
	// leader election is enabled by default
	if dda.Spec.Features != nil && dda.Spec.Features.EventCollection != nil && apiutils.BoolValue(dda.Spec.Features.EventCollection.CollectKubernetesEvents) {
		f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda)
		f.rbacSuffix = common.ClusterAgentSuffix

		if apiutils.BoolValue(dda.Spec.Features.EventCollection.UnbundleEvents) {
			if len(dda.Spec.Features.EventCollection.CollectedEventTypes) > 0 {
				f.configMapName = constants.GetConfName(dda, nil, defaultKubeAPIServerConf)
				f.unbundleEvents = *dda.Spec.Features.EventCollection.UnbundleEvents
				f.unbundleEventTypes = dda.Spec.Features.EventCollection.CollectedEventTypes
			} else {
				f.logger.Info("UnbundleEvents is enabled but no CollectedEventTypes are specified, disabling unbundleEvents")
			}
		}

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *eventCollectionFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// Manage RBAC
	rbacName := getRBACResourceName(f.owner, f.rbacSuffix)

	// hardcoding leader election RBAC for now
	// can look into separating this out later if this needs to be configurable for other features
	leaderElectionResourceName := utils.GetDatadogLeaderElectionResourceName(f.owner)
	err := managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getLeaderElectionRBACPolicyRules(leaderElectionResourceName))
	if err != nil {
		return err
	}

	// event collection RBAC
	tokenResourceName := secrets.GetDefaultDCATokenSecretName(f.owner)
	err = managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules(tokenResourceName))
	if err != nil {
		return err
	}

	if f.configMapName != "" {
		// creating ConfigMap for event collection if required
		cm, err := buildDefaultConfigMap(f.owner.GetNamespace(), f.configMapName, f.unbundleEvents, f.unbundleEventTypes)
		if err != nil {
			return err
		}

		// Add md5 hash annotation for configMap
		f.cmAnnotationKey = object.GetChecksumAnnotationKey(feature.KubernetesAPIServerIDType)
		f.cmAnnotationValue, err = comparison.GenerateMD5ForSpec(cm.Data)
		if err != nil {
			return err
		}

		if f.cmAnnotationKey != "" && f.cmAnnotationValue != "" {
			annotations := object.MergeAnnotationsLabels(f.logger, cm.Annotations, map[string]string{f.cmAnnotationKey: f.cmAnnotationValue}, "*")
			cm.SetAnnotations(annotations)
		}

		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, cm); err != nil {
			return err
		}
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	// Env vars
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDCollectKubernetesEvents,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  common.DDLeaderElection,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDLeaderLeaseName,
		Value: utils.GetDatadogLeaderElectionResourceName(f.owner),
	})

	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  common.DDClusterAgentTokenName,
		Value: secrets.GetDefaultDCATokenSecretName(f.owner),
	})

	// ConfigMap for event collection if required
	if f.configMapName != "" {
		vol := volume.GetBasicVolume(f.configMapName, kubernetesAPIServerCheckConfigVolumeName)
		volMount := corev1.VolumeMount{
			Name:      kubernetesAPIServerCheckConfigVolumeName,
			MountPath: fmt.Sprintf("%s%s/%s", common.ConfigVolumePath, common.ConfdVolumePath, kubeAPIServerConfigFolderName),
			ReadOnly:  true,
		}

		managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.ClusterAgentContainerName)
		managers.Volume().AddVolume(&vol)

		// Add md5 hash annotation for configMap
		if f.cmAnnotationKey != "" && f.cmAnnotationValue != "" {
			managers.Annotation().AddAnnotation(f.cmAnnotationKey, f.cmAnnotationValue)
		}
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageNodeAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}
