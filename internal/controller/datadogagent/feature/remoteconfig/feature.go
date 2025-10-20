// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

const (
	rcDBVolumeName = "datadogrun"
	rcDBVolumePath = "/opt/datadog-agent/run/"
)

var (
	rcVolume = &corev1.Volume{
		Name: rcDBVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	rcVolumeMount = &corev1.VolumeMount{
		Name:      rcDBVolumeName,
		MountPath: rcDBVolumePath,
	}
)

func init() {
	err := feature.Register(feature.RemoteConfigurationIDType, buildRCFeature)
	if err != nil {
		panic(err)
	}
}

func buildRCFeature(options *feature.Options) feature.Feature {
	rcFeat := &rcFeature{}

	if options != nil {
		rcFeat.logger = options.Logger
	}

	return rcFeat
}

type rcFeature struct {
	logger logr.Logger

	enabled bool
}

// ID returns the ID of the Feature
func (f *rcFeature) ID() feature.IDType {
	return feature.RemoteConfigurationIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *rcFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {

	if ddaSpec.Features != nil && ddaSpec.Features.RemoteConfiguration != nil && ddaSpec.Features.RemoteConfiguration.Enabled != nil {
		// If a value exists, explicitly enable or disable Remote Config and override the default
		f.enabled = apiutils.BoolValue(ddaSpec.Features.RemoteConfiguration.Enabled)
		// If Remote Config is enabled, we need to enable the Agent and Cluster Agent components.
		// We need to only set the IsRequired to true if the feature is enabled as setting it to false will take priority over other features.
		// Ref: https://github.com/DataDog/datadog-operator/blob/c4b6e498048a11fbe99d1ea51d2870c6be578799/internal/controller/datadogagent/feature/types.go#L37
		if f.enabled {
			reqComp.Agent.IsRequired = ddaSpec.Features.RemoteConfiguration.Enabled
			reqComp.ClusterAgent.IsRequired = ddaSpec.Features.RemoteConfiguration.Enabled
		}
	}

	reqComp.Agent.Containers = []apicommon.AgentContainerName{apicommon.CoreAgentContainerName}
	reqComp.ClusterAgent.Containers = []apicommon.AgentContainerName{apicommon.ClusterAgentContainerName}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *rcFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *rcFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	enabledEnvVar := &corev1.EnvVar{
		Name:  DDRemoteConfigurationEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	}
	managers.EnvVar().AddEnvVar(enabledEnvVar)

	if f.enabled {
		// Volume to create the Remote Config Database
		// Mandatory as the cluster agent root FS is read only by default
		managers.Volume().AddVolume(rcVolume)
		managers.VolumeMount().AddVolumeMount(rcVolumeMount)
	}

	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *rcFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	f.ManageNodeAgent(managers, provider)
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *rcFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	enabledEnvVar := &corev1.EnvVar{
		Name:  DDRemoteConfigurationEnabled,
		Value: apiutils.BoolToString(&f.enabled),
	}
	managers.EnvVar().AddEnvVar(enabledEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *rcFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (f *rcFeature) ManageOTelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
