// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscaling

import (
	"errors"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	componentdca "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/component/clusteragent"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
)

func init() {
	err := feature.Register(feature.AutoscalingIDType, buildAutoscalingFeature)
	if err != nil {
		panic(err)
	}
}

func buildAutoscalingFeature(options *feature.Options) feature.Feature {
	autoscalingFeat := &autoscalingFeature{}

	if options != nil {
		autoscalingFeat.logger = options.Logger
	}

	return autoscalingFeat
}

type autoscalingFeature struct {
	serviceAccountName           string
	owner                        metav1.Object
	logger                       logr.Logger
	admissionControllerActivated bool
}

// ID returns the ID of the Feature
func (f *autoscalingFeature) ID() feature.IDType {
	return feature.AutoscalingIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *autoscalingFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda
	if dda.Spec.Features == nil {
		return feature.RequiredComponents{}
	}

	autoscaling := dda.Spec.Features.Autoscaling
	if autoscaling == nil || autoscaling.Workload == nil || !apiutils.BoolValue(autoscaling.Workload.Enabled) {
		return feature.RequiredComponents{}
	}

	admission := dda.Spec.Features.AdmissionController
	f.admissionControllerActivated = apiutils.BoolValue(admission.Enabled)
	f.serviceAccountName = constants.GetClusterAgentServiceAccount(dda)

	return feature.RequiredComponents{
		ClusterAgent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName},
		},
	}
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *autoscalingFeature) ManageDependencies(managers feature.ResourceManagers) error {
	// Hack to trigger an error if admission feature is not enabled as we cannot return an error in configure
	if !f.admissionControllerActivated {
		return errors.New("admission controller feature must be enabled to use the autoscaling feature")
	}

	return managers.RBACManager().AddClusterPolicyRulesByComponent(f.owner.GetNamespace(), componentdca.GetClusterAgentRbacResourcesName(f.owner)+"-autoscaling", f.serviceAccountName, getDCAClusterPolicyRules(), string(v2alpha1.ClusterAgentComponentName))
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *autoscalingFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDAutoscalingWorkloadEnabled,
		Value: "true",
	})

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *autoscalingFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *autoscalingFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *autoscalingFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
