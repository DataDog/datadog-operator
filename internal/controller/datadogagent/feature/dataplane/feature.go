// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dataplane

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
	err := feature.Register(feature.DataPlaneIDType, buildDataPlaneFeature)
	if err != nil {
		panic(err)
	}
}

func buildDataPlaneFeature(options *feature.Options) feature.Feature {
	f := &dataPlaneFeature{}

	if options != nil {
		f.logger = options.Logger
	}

	return f
}

type dataPlaneFeature struct {
	logger logr.Logger

	enabled          bool
	dogstatsdEnabled bool
}

// ID returns the ID of the Feature
func (f *dataPlaneFeature) ID() feature.IDType {
	return feature.DataPlaneIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *dataPlaneFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	// Check if the deprecated annotation is being used, and log a warning if so.
	if featureutils.HasAgentDataPlaneAnnotation(dda) {
		f.logger.Info("DEPRECATION WARNING: annotation 'agent.datadoghq.com/adp-enabled' is deprecated; use 'spec.features.dataPlane.enabled' instead")
	}

	f.enabled = featureutils.IsDataPlaneEnabled(dda, ddaSpec)
	f.dogstatsdEnabled = featureutils.IsDataPlaneDogstatsdEnabled(ddaSpec)

	var reqComp feature.RequiredComponents

	if f.enabled {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: &f.enabled,
			Containers: []apicommon.AgentContainerName{apicommon.AgentDataPlaneContainerName},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *dataPlaneFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dataPlaneFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *dataPlaneFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return f.ManageNodeAgent(managers, provider)
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dataPlaneFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// We set the relevant configuration on the Core Agent specifically, which trickles down to the Data Plane when it
	// queries the Core Agent for its configuration.
	//
	// It is also used to influence the Core Agent in terms of what it chooses to run itself or allow to be delegated to
	// the data plane.
	if f.enabled {
		// When Data Plane is enabled, we signal this to the Core Agent by setting an environment variable.
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  common.DDDataPlaneEnabled,
			Value: "true",
		})

		if f.dogstatsdEnabled {
			managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
				Name:  common.DDDataPlaneDogstatsdEnabled,
				Value: "true",
			})
		}
	}

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dataPlaneFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *dataPlaneFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
