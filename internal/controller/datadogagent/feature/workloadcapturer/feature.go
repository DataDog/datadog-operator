// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadcapturer

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/pkg/images"
)

const (
	featureName              = "workloadcapturer"
	containerName            = "workload-capture-proxy"
	dogstatsdForwardPort     = 18125
	dogstatsdForwardHost     = "localhost"  // Pod-local
	featureConfigPath        = ".spec.features.workloadCapturer"
)

func init() {
	if err := feature.Register(feature.WorkloadCapturerIDType, buildFeature); err != nil {
		panic(err)
	}
}

func buildFeature(*feature.Options) feature.Feature {
	return &workloadCapturerFeature{}
}

type workloadCapturerFeature struct {
	customImage    *v2alpha1.AgentImageConfig
	globalRegistry string
	owner          metav1.Object
}

// ID returns the ID of the Feature
func (f *workloadCapturerFeature) ID() feature.IDType {
	return feature.WorkloadCapturerIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *workloadCapturerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if ddaSpec.Features == nil || ddaSpec.Features.WorkloadCapturer == nil || !apiutils.BoolValue(ddaSpec.Features.WorkloadCapturer.Enabled) {
		return reqComp
	}

	// Store custom image config and registry if provided
	f.customImage = ddaSpec.Features.WorkloadCapturer.Image
	if ddaSpec.Global != nil && ddaSpec.Global.Registry != nil {
		f.globalRegistry = *ddaSpec.Global.Registry
	}

	// Configure agent to forward dogstatsd traffic to this capturer
	f.configureDogStatsDForwarding(dda, ddaSpec)

	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.CoreAgentContainerName,
				apicommon.WorkloadCapturerContainerName,
			},
		},
	}

	return reqComp
}

// configureDogStatsDForwarding injects forwarding configuration into agent container
func (f *workloadCapturerFeature) configureDogStatsDForwarding(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec) {
	// Inject forwarding configuration into agent container
	envVars := []corev1.EnvVar{
		{
			Name:  "DD_STATSD_FORWARD_HOST",
			Value: dogstatsdForwardHost,
		},
		{
			Name:  "DD_STATSD_FORWARD_PORT",
			Value: fmt.Sprintf("%d", dogstatsdForwardPort),
		},
	}

	// Add to core agent container
	for _, envVar := range envVars {
		// Note: This would be handled in the agent feature's ManageNodeAgent
		// For now, we're just storing the configuration
		_ = envVar
	}
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *workloadCapturerFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
func (f *workloadCapturerFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for single container mode
func (f *workloadCapturerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Not supported in single container mode
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
func (f *workloadCapturerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	// Override capturer container image if custom image specified
	if f.customImage != nil {
		for i, container := range managers.PodTemplateSpec().Spec.Containers {
			if container.Name == containerName {
				// For workload-capture-proxy, use the image name directly without registry prefix
				// to support local images loaded into KIND
				var customImagePath string
				if f.customImage.Name != "" {
					customImagePath = f.customImage.Name
					if f.customImage.Tag != "" {
						customImagePath += ":" + f.customImage.Tag
					} else {
						customImagePath += ":latest"
					}
				} else {
					// Fallback to standard image assembly
					customImagePath = images.AssembleImage(f.customImage, f.globalRegistry)
				}
				managers.PodTemplateSpec().Spec.Containers[i].Image = customImagePath
				// Set imagePullPolicy to Never for local images to prevent pulling from registry
				managers.PodTemplateSpec().Spec.Containers[i].ImagePullPolicy = corev1.PullNever
			}
		}
	}

	// Inject DogStatsD forwarding configuration into agent container
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
		Name:  "DD_STATSD_FORWARD_HOST",
		Value: dogstatsdForwardHost,
	})
	managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
		Name:  "DD_STATSD_FORWARD_PORT",
		Value: fmt.Sprintf("%d", dogstatsdForwardPort),
	})

	// TODO(production): Add retry logic to agent's statsd forwarding or use startup probe
	// For now, relying on agent being slower to start than the fast Rust capturer binary

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
func (f *workloadCapturerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
