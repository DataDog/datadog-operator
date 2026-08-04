// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubernetesactions

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

func clusterAgentVersion(ddaSpec *v2alpha1.DatadogAgentSpec) string {
	if ddaSpec == nil {
		return images.AgentLatestVersion
	}
	if clusterAgent, ok := ddaSpec.Override[v2alpha1.ClusterAgentComponentName]; ok {
		if clusterAgent.Image != nil {
			return common.GetAgentVersionFromImage(*clusterAgent.Image)
		}
	}
	return images.AgentLatestVersion
}

func init() {
	err := feature.Register(feature.KubernetesActionsIDType, buildKubernetesActionsFeature)
	if err != nil {
		panic(err)
	}
}

func buildKubernetesActionsFeature(options *feature.Options) feature.Feature {
	f := &kubernetesActionsFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}
	if options != nil {
		f.logger = options.Logger
	}
	return f
}

type kubernetesActionsFeature struct {
	owner              metav1.Object
	serviceAccountName string
	rbacSuffix         string
	logger             logr.Logger
}

func (f *kubernetesActionsFeature) ID() feature.IDType {
	return feature.KubernetesActionsIDType
}

func (f *kubernetesActionsFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.KubernetesActions == nil || !apiutils.BoolValue(ddaSpec.Features.KubernetesActions.Enabled) {
		return reqComp
	}

	if !utils.IsAboveMinVersion(clusterAgentVersion(ddaSpec), ClusterAgentMinVersion, nil) {
		f.logger.V(1).Info("cluster agent version is too low for Kubernetes Actions", "min", ClusterAgentMinVersion)
		return reqComp
	}

	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

	reqComp = feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: new(true),
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
	}
	return reqComp
}

func (f *kubernetesActionsFeature) ManageDependencies(managers feature.ResourceManagers) error {
	rbacName := getRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, kubernetesActionsRBACPolicyRules)
}

func (f *kubernetesActionsFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDKubeActionsEnabled,
		Value: "true",
	})
	return nil
}

func (f *kubernetesActionsFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubernetesActionsFeature) ManageNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubernetesActionsFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubernetesActionsFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers) error {
	return nil
}
