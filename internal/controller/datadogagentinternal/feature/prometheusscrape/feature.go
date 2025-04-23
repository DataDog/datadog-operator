// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package prometheusscrape

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
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
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
				},
			},
			ClusterAgent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.ClusterAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *prometheusScrapeFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *prometheusScrapeFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDPrometheusScrapeEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
		Name:  DDPrometheusScrapeServiceEndpoints,
		Value: strconv.FormatBool(f.enableServiceEndpoints),
	})
	if f.additionalConfigs != "" {
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDPrometheusScrapeChecks,
			Value: apiutils.YAMLToJSONString(f.additionalConfigs),
		})
	}
	if f.openmetricsVersion != 0 {
		managers.EnvVar().AddEnvVarToContainer(apicommon.ClusterAgentContainerName, &corev1.EnvVar{
			Name:  DDPrometheusScrapeVersion,
			Value: strconv.Itoa(f.openmetricsVersion),
		})
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *prometheusScrapeFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *prometheusScrapeFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.manageNodeAgent(apicommon.CoreAgentContainerName, managers, provider)
	return nil
}

func (f *prometheusScrapeFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers, provider string) error {
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  DDPrometheusScrapeEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
		Name:  DDPrometheusScrapeServiceEndpoints,
		Value: strconv.FormatBool(f.enableServiceEndpoints),
	})
	if f.additionalConfigs != "" {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDPrometheusScrapeChecks,
			Value: apiutils.YAMLToJSONString(f.additionalConfigs),
		})
	}
	if f.openmetricsVersion != 0 {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDPrometheusScrapeVersion,
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
