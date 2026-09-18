// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package checkrunner

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
)

func init() {
	err := feature.Register(feature.CheckRunnerIDType, buildCheckRunnerFeature)
	if err != nil {
		panic(err)
	}
}

func buildCheckRunnerFeature(options *feature.Options) feature.Feature {
	f := &checkRunnerFeature{}

	if options != nil {
		f.logger = options.Logger
		f.defaultDataPlaneEnabled = options.DefaultDataPlaneEnabled
	}

	return f
}

type checkRunnerFeature struct {
	logger logr.Logger

	enabled                 bool
	defaultDataPlaneEnabled bool
}

// ID returns the ID of the Feature
func (f *checkRunnerFeature) ID() feature.IDType {
	return feature.CheckRunnerIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *checkRunnerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	f.enabled = featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnableCheckRunnerAnnotation)

	var reqComp feature.RequiredComponents

	if f.enabled {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: &f.enabled,
			Containers: []apicommon.AgentContainerName{apicommon.AgentCheckRunnerContainerName},
		}

		if !featureutils.IsDataPlaneEnabled(dda, ddaSpec, f.defaultDataPlaneEnabled) {
			f.logger.Info("The checkrunner feature requires the dataPlane feature.")
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *checkRunnerFeature) ManageDependencies(_ feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec.
func (f *checkRunnerFeature) ManageClusterAgent(_ feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
func (f *checkRunnerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	if !f.enabled {
		return nil
	}

	f.configureNodeAgent(managers, apicommon.UnprivilegedSingleAgentContainerName)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec.
func (f *checkRunnerFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	if !f.enabled {
		return nil
	}

	f.configureNodeAgent(managers, apicommon.CoreAgentContainerName)
	return nil
}

// configureNodeAgent configures the Core Agent, Check Runner (ACR) and Agent Data Plane (ADP)
// containers of the Node Agent. In SingleContainerStrategy, all three containers are the same container.
func (f *checkRunnerFeature) configureNodeAgent(managers feature.PodTemplateManagers, coreAgentContainer apicommon.AgentContainerName) {
	// ACR configuration
	// Enable the Check Runner.
	managers.EnvVar().AddEnvVarToContainer(coreAgentContainer, &corev1.EnvVar{
		Name:  ddCheckRunnerEnabled,
		Value: "true",
	})
	// Force sub-agent of the Core Agent mode and configure communication with the Data Plane.
	managers.EnvVar().AddEnvVarToContainer(coreAgentContainer, &corev1.EnvVar{
		Name:  ddCheckRunnerStandaloneMode,
		Value: "false",
	})
	managers.EnvVar().AddEnvVarToContainer(coreAgentContainer, &corev1.EnvVar{
		Name:  ddCheckRunnerEndpointsIPCEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(coreAgentContainer, &corev1.EnvVar{
		Name:  ddCheckRunnerEndpointsIPCEndpoint,
		Value: "http://localhost:5105",
	})

	// ADP configuration
	// Enable Checks IPC source (the endpoint ACR sends events to)
	managers.EnvVar().AddEnvVarToContainer(coreAgentContainer, &corev1.EnvVar{
		Name:  common.DDDataPlaneEnabled,
		Value: "true",
	})
	managers.EnvVar().AddEnvVarToContainer(coreAgentContainer, &corev1.EnvVar{
		Name:  common.DDDataPlaneChecksEnabled,
		Value: "true",
	})
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec.
func (f *checkRunnerFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers) error {
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OTel Agent Gateway's corev1.PodTemplateSpec.
func (f *checkRunnerFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers) error {
	return nil
}
