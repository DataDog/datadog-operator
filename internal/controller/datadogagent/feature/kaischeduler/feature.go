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
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
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
	// clusterAgentEvents is true when the Cluster Agent leader emits the PodGroup events.
	clusterAgentEvents bool
	// gpuEnabled is true when GPU monitoring is enabled, so the node Agent maps KAI fractional GPUs.
	gpuEnabled bool

	clusterAgentServiceAccountName string

	owner metav1.Object

	logger logr.Logger
}

// kaiSchedulerConfig is experimental and deliberately absent from the DatadogAgent CRD.
type kaiSchedulerConfig struct {
	// ClusterAgentEvents makes the Cluster Agent leader emit the PodGroup events (job_scheduler_events check).
	ClusterAgentEvents *bool `json:"clusterAgentEvents,omitempty"`
}

// ID returns the ID of the Feature
func (f *kaiSchedulerFeature) ID() feature.IDType {
	return feature.KAISchedulerIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
// The feature is experimental, so it is enabled through an annotation instead of the DatadogAgent CRD.
func (f *kaiSchedulerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if !featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnableKAISchedulerAnnotation) {
		return reqComp
	}

	var kai kaiSchedulerConfig
	if raw, ok := featureutils.GetFeatureConfigAnnotation(dda, featureutils.KAISchedulerConfigAnnotation); ok {
		if err := featureutils.DecodeStrictJSON(raw, &kai); err != nil {
			f.logger.Error(err, "invalid KAI Scheduler configuration annotation")
			return reqComp
		}
	}

	clusterAgentVersion := common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.ClusterAgentComponentName)
	if utils.IsAboveMinVersion(clusterAgentVersion, kaiSchedulerMinVersion, nil) {
		f.clusterAgentEnabled = true
		if kai.ClusterAgentEvents != nil && *kai.ClusterAgentEvents {
			// A version that cannot be parsed is treated as too old.
			if utils.IsAboveMinVersion(clusterAgentVersion, clusterAgentEventsMinVersion, new(false)) {
				f.clusterAgentEvents = true
			} else {
				f.logger.Info("KAI Scheduler PodGroup events in the Cluster Agent require Cluster Agent >= 7.87.0; not enabling them")
			}
		}
		reqComp.ClusterAgent.IsRequired = new(true)
		reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}
		f.clusterAgentServiceAccountName = constants.GetClusterAgentServiceAccount(dda.GetName(), ddaSpec)
	} else {
		f.logger.Info("KAI Scheduler PodGroup collection requires Cluster Agent >= 7.86.0; not enabling it")
	}

	if ddaSpec.Features != nil && ddaSpec.Features.GPU != nil && apiutils.BoolValue(ddaSpec.Features.GPU.Enabled) {
		if utils.IsAboveMinVersion(common.GetComponentVersionFromSpec(ddaSpec, v2alpha1.NodeAgentComponentName), kaiSchedulerMinVersion, nil) {
			f.gpuEnabled = true
			reqComp.Agent.IsRequired = new(true)
			reqComp.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
		} else {
			f.logger.Info("KAI Scheduler fractional GPU mapping requires Agent >= 7.86.0; not enabling it")
		}
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
	if f.clusterAgentEvents {
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDClusterAgentKAISchedulerEventsEnabled,
			Value: "true",
		})
	}
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
