// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kubeactions

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func init() {
	err := feature.Register(feature.KubeActionsIDType, buildKubeActionsFeature)
	if err != nil {
		panic(err)
	}
}

func buildKubeActionsFeature(options *feature.Options) feature.Feature {
	f := &kubeActionsFeature{
		rbacSuffix: common.ClusterAgentSuffix,
	}
	if options != nil {
		f.logger = options.Logger
	}
	return f
}

type kubeActionsFeature struct {
	owner              metav1.Object
	serviceAccountName string
	rbacSuffix         string
	logger             logr.Logger
}

func (f *kubeActionsFeature) ID() feature.IDType {
	return feature.KubeActionsIDType
}

func (f *kubeActionsFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.KubeActions == nil || !apiutils.BoolValue(ddaSpec.Features.KubeActions.Enabled) {
		return reqComp
	}

	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)

	reqComp = feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: ptr.To(true),
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
	}
	return reqComp
}

func (f *kubeActionsFeature) ManageDependencies(managers feature.ResourceManagers) error {
	rbacName := getRBACResourceName(f.owner, f.rbacSuffix)
	return managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), rbacName, f.serviceAccountName, kubeActionsRBACPolicyRules)
}

func (f *kubeActionsFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDKubeActionsEnabled,
		Value: "true",
	})
	return nil
}

func (f *kubeActionsFeature) ManageSingleContainerNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubeActionsFeature) ManageNodeAgent(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubeActionsFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}

func (f *kubeActionsFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers) error {
	return nil
}
