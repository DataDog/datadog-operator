// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package highavailability

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.HighAvailabilityIDType, buildHighAvailabilityFeature)
	if err != nil {
		panic(err)
	}
}

func buildHighAvailabilityFeature(options *feature.Options) feature.Feature {
	highAvailabilityFeat := &highAvailabilityFeature{}

	return highAvailabilityFeat
}

type highAvailabilityFeature struct {
}

// ID returns the ID of the Feature
func (f *highAvailabilityFeature) ID() feature.IDType {
	return feature.HighAvailabilityIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *highAvailabilityFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	highAvailability := dda.Spec.Features.HighAvailability
	if highAvailability != nil && apiutils.BoolValue(highAvailability.Enabled) {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
					apicommonv1.AgentDataPlaneContainerName,
				},
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *highAvailabilityFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	// TODO: Should we be doing anything here? Not clear to me if new features should be supported
	// symmetrically across both CRD versions.

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *highAvailabilityFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *highAvailabilityFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageMultiProcessNodeAgent allows a feature to configure the multi-process container for Node Agent's corev1.PodTemplateSpec
// if multi-process container usage is enabled and can be used with the current feature set
// It should do nothing if the feature doesn't need to configure it.
func (f *highAvailabilityFeature) ManageMultiProcessNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return f.manageNodeAgent(apicommonv1.UnprivilegedMultiProcessAgentContainerName, managers, provider)
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *highAvailabilityFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return f.manageNodeAgent(apicommonv1.CoreAgentContainerName, managers, provider)
}

func (f *highAvailabilityFeature) manageNodeAgent(agentContainerName apicommonv1.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {
	// In multi-process mode, make sure the ADP service gets enabled during startup.
	if agentContainerName == apicommonv1.UnprivilegedMultiProcessAgentContainerName {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDHAEnabled,
			Value: "true",
		})
	}

	// Configure the core Agent to use ADP by funneling it through OPW configuration.
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDObservabilityPipelinesWorkerMetricsEnabled,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDObservabilityPipelinesWorkerMetricsUrl,
		Value: apicommon.DefaultHAUrl,
	})

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDObservabilityPipelinesWorkerLogsEnabled,
		Value: "true",
	})

	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDObservabilityPipelinesWorkerLogsUrl,
		Value: apicommon.DefaultHAUrl,
	})

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *highAvailabilityFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
