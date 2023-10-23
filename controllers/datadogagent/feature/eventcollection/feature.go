// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eventcollection

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	common "github.com/DataDog/datadog-operator/controllers/datadogagent/common"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/controller/utils"
)

func init() {
	err := feature.Register(feature.EventCollectionIDType, buildEventCollectionFeature)
	if err != nil {
		panic(err)
	}
}

func buildEventCollectionFeature(options *feature.Options) feature.Feature {
	eventCollectionFeat := &eventCollectionFeature{}

	return eventCollectionFeat
}

type eventCollectionFeature struct {
	serviceAccountName string
	rbacSuffix         string
	owner              metav1.Object
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
		f.serviceAccountName = v2alpha1.GetClusterAgentServiceAccount(dda)
		f.rbacSuffix = common.ClusterAgentSuffix

		reqComp = feature.RequiredComponents{
			ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *eventCollectionFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	config := dda.Spec.Agent.Config
	dcaConfig := dda.Spec.ClusterAgent.Config

	if *config.LeaderElection {
		// cluster agent
		if dcaConfig != nil && apiutils.BoolValue(dcaConfig.CollectEvents) {
			f.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)
			f.rbacSuffix = common.ClusterAgentSuffix

			reqComp = feature.RequiredComponents{
				ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			}
			return reqComp
		}

		// node agent
		if apiutils.BoolValue(config.CollectEvents) {
			f.serviceAccountName = v1alpha1.GetAgentServiceAccount(dda)
			f.rbacSuffix = common.NodeAgentSuffix

			reqComp = feature.RequiredComponents{
				Agent: feature.RequiredComponent{
					IsRequired: apiutils.NewBoolPointer(true),
					Containers: []apicommonv1.AgentContainerName{
						apicommonv1.CoreAgentContainerName,
					},
				},
			}
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
	tokenResourceName := v2alpha1.GetDefaultDCATokenSecretName(f.owner)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, getRBACPolicyRules(tokenResourceName))
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDCollectKubernetesEvents,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLeaderElection,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLeaderLeaseName,
		Value: utils.GetDatadogLeaderElectionResourceName(f.owner),
	})

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDClusterAgentTokenName,
		Value: v2alpha1.GetDefaultDCATokenSecretName(f.owner),
	})

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDCollectKubernetesEvents,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLeaderElection,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLeaderLeaseName,
		Value: utils.GetDatadogLeaderElectionResourceName(f.owner),
	})

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDClusterAgentTokenName,
		Value: v2alpha1.GetDefaultDCATokenSecretName(f.owner),
	})

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
