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
	nodeAgentEnable    bool
	clusterAgentEnable bool
	serviceAccountName string
	rbacSuffix         string
	owner              metav1.Object
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *eventCollectionFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	// v2alpha1 configures event collection using the cluster agent only
	// leader election is enabled by default
	if dda.Spec.Features.EventCollection != nil && apiutils.BoolValue(dda.Spec.Features.EventCollection.CollectKubernetesEvents) {
		f.clusterAgentEnable = true
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
		if *dcaConfig.CollectEvents {
			f.clusterAgentEnable = true
			f.serviceAccountName = v1alpha1.GetClusterAgentServiceAccount(dda)
			f.rbacSuffix = common.ClusterAgentSuffix

			reqComp = feature.RequiredComponents{
				ClusterAgent: feature.RequiredComponent{IsRequired: apiutils.NewBoolPointer(true)},
			}
			return reqComp
		}

		// node agent
		if *config.CollectEvents {
			f.nodeAgentEnable = true
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
func (f *eventCollectionFeature) ManageDependencies(managers feature.ResourceManagers) error {
	// Manage RBAC
	rbacName := getRBACResourceName(f.owner, f.rbacSuffix)
	managers.RBACManager().AddClusterPolicyRules("", rbacName, f.serviceAccountName, getRBACPolicyRules())

	// hardcoding leader election RBAC for now
	// can look into separating this out later if this needs to be configurable for other features
	managers.RBACManager().AddClusterPolicyRules("", rbacName, f.serviceAccountName, getLeaderElectionRBACPolicyRules())

	return nil
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

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDCollectKubernetesEvents,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDLeaderElection,
		Value: "true",
	})

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *eventCollectionFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
