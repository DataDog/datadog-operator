// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheusscrape

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.PrometheusScrapeIDType, buildPrometheusScrapeFeature)
	if err != nil {
		panic(err)
	}
}

func buildPrometheusScrapeFeature(options *feature.Options) feature.Feature {
	prometheusScrapeFeat := &prometheusScrapeFeature{}

	return prometheusScrapeFeat
}

type prometheusScrapeFeature struct {
	enableServiceEndpoints bool
	additionalConfigs      string
	openmetricsVersion     int
}

// ID returns the ID of the Feature
func (f *prometheusScrapeFeature) ID() feature.IDType {
	return feature.PrometheusScrapeIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *prometheusScrapeFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	if dda.Spec.Features == nil {
		return
	}

	prometheusScrape := dda.Spec.Features.PrometheusScrape

	if prometheusScrape != nil && apiutils.BoolValue(prometheusScrape.Enabled) {
		f.enableServiceEndpoints = apiutils.BoolValue(prometheusScrape.EnableServiceEndpoints)
		if prometheusScrape.AdditionalConfigs != nil {
			f.additionalConfigs = *prometheusScrape.AdditionalConfigs
		}
		if prometheusScrape.Version != nil {
			f.openmetricsVersion = *prometheusScrape.Version
		}
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
				},
			},
			ClusterAgent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.ClusterAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *prometheusScrapeFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	prometheusScrape := dda.Spec.Features.PrometheusScrape

	if apiutils.BoolValue(prometheusScrape.Enabled) {
		f.enableServiceEndpoints = apiutils.BoolValue(prometheusScrape.ServiceEndpoints)
		if prometheusScrape.AdditionalConfigs != nil {
			f.additionalConfigs = *prometheusScrape.AdditionalConfigs
		}
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.CoreAgentContainerName,
				},
			},
			ClusterAgent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommonv1.AgentContainerName{
					apicommonv1.ClusterAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *prometheusScrapeFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *prometheusScrapeFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDPrometheusScrapeEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
		Value: strconv.FormatBool(f.enableServiceEndpoints),
	})
	if f.additionalConfigs != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDPrometheusScrapeChecks,
			Value: apiutils.YAMLToJSONString(f.additionalConfigs),
		})
	}
	if f.openmetricsVersion != 0 {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDPrometheusScrapeVersion,
			Value: strconv.Itoa(f.openmetricsVersion),
		})
	}

	return nil
}

// ManageMultiProcessNodeAgent allows a feature to configure the mono-container Node Agent's corev1.PodTemplateSpec
// if mono-container usage is enabled and can be used with the current feature set
// It should do nothing if the feature doesn't need to configure it.
func (f *prometheusScrapeFeature) ManageMultiProcessNodeAgent(managers feature.PodTemplateManagers) error {
	f.manageNodeAgent(apicommonv1.NonPrivilegedMultiProcessAgentContainerName, managers)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *prometheusScrapeFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	f.manageNodeAgent(apicommonv1.CoreAgentContainerName, managers)
	return nil
}

func (f *prometheusScrapeFeature) manageNodeAgent(agentContainerName apicommonv1.AgentContainerName, managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDPrometheusScrapeEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  apicommon.DDPrometheusScrapeServiceEndpoints,
		Value: strconv.FormatBool(f.enableServiceEndpoints),
	})
	if f.additionalConfigs != "" {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDPrometheusScrapeChecks,
			Value: apiutils.YAMLToJSONString(f.additionalConfigs),
		})
	}
	if f.openmetricsVersion != 0 {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDPrometheusScrapeVersion,
			Value: strconv.Itoa(f.openmetricsVersion),
		})
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *prometheusScrapeFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
