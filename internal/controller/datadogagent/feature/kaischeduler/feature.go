// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kaischeduler

import (
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/utils"
)

func init() {
	err := feature.Register(feature.KAISchedulerIDType, buildKAISchedulerFeature)
	if err != nil {
		panic(err)
	}
}

func buildKAISchedulerFeature(options *feature.Options) feature.Feature {
	kaiFeat := &kaiSchedulerFeature{}

	if options != nil {
		kaiFeat.logger = options.Logger
	}
	return kaiFeat
}

type kaiSchedulerFeature struct {
	clusterAgentEnabled bool
	// gpuEnabled is true when GPU monitoring is enabled, so the node Agent maps KAI fractional GPUs.
	gpuEnabled bool

	clusterAgentServiceAccountName string

	owner metav1.Object

	logger logr.Logger
}

// ID returns the ID of the Feature
func (f *kaiSchedulerFeature) ID() feature.IDType {
	return feature.KAISchedulerIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *kaiSchedulerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.KAIScheduler == nil || !apiutils.BoolValue(ddaSpec.Features.KAIScheduler.Enabled) {
		return reqComp
	}

	if utils.IsAboveMinVersion(common.GetComponentVersion(dda, v2alpha1.ClusterAgentComponentName), kaiSchedulerMinVersion, nil) {
		f.clusterAgentEnabled = true
		reqComp.ClusterAgent.IsRequired = new(true)
		reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
		f.clusterAgentServiceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)
	} else {
		f.logger.Info("KAI Scheduler PodGroup collection requires Cluster Agent >= 7.86.0; not enabling it")
	}

	if ddaSpec.Features.GPU != nil && apiutils.BoolValue(ddaSpec.Features.GPU.Enabled) {
		f.gpuEnabled = true
		reqComp.Agent.IsRequired = new(true)
		reqComp.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *kaiSchedulerFeature) ManageDependencies(managers feature.ResourceManagers) error {
	if !f.clusterAgentEnabled {
		return nil
	}

	if err := managers.RBACManager().AddClusterPolicyRules(f.owner.GetNamespace(), getKAISchedulerRBACResourceName(f.owner, common.ClusterAgentSuffix), f.clusterAgentServiceAccountName, kaiSchedulerClusterAgentRBACPolicyRules); err != nil {
		return fmt.Errorf("failed to add KAI Scheduler Cluster Agent RBAC: %w", err)
	}
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *kaiSchedulerFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	if !f.clusterAgentEnabled {
		return nil
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDClusterAgentKAISchedulerEnabled,
		Value: "true",
	})
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *kaiSchedulerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *kaiSchedulerFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	if !f.gpuEnabled {
		return nil
	}

	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
		Name:  DDGPUKAISchedulerEnabled,
		Value: "true",
	})
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *kaiSchedulerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}

func (f *kaiSchedulerFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers) error {
	return nil
}
