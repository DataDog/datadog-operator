// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package liveprocess

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.LiveProcessIDType, buildLiveProcessFeature)
	if err != nil {
		panic(err)
	}
}

func buildLiveProcessFeature(options *feature.Options) feature.Feature {
	liveProcessFeat := &liveProcessFeature{}

	return liveProcessFeat
}

type liveProcessFeature struct {
	enable bool
	owner  metav1.Object
	// CELENE other things?!
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *liveProcessFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if dda.Spec.Features.LiveProcessCollection != nil && apiutils.BoolValue(dda.Spec.Features.LiveProcessCollection.Enabled) {
		f.enable = true
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				Required: &f.enable,
				RequiredContainers: []apicommonv1.AgentContainerName{
					apicommonv1.ProcessAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *liveProcessFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	f.owner = dda

	if dda.Spec.Agent.Process != nil && *dda.Spec.Agent.Process.ProcessCollectionEnabled {
		f.enable = true
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				Required: &f.enable,
				RequiredContainers: []apicommonv1.AgentContainerName{
					apicommonv1.ProcessAgentContainerName,
				},
			},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *liveProcessFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// passwd volume mount
	passwdVol, passwdVolMount := volume.GetVolumes(passwdVolumeName, passwdVolumePath, passwdVolumePath)
	managers.Volume().AddVolumeToContainer(&passwdVol, &passwdVolMount, apicommonv1.ProcessAgentContainerName)

	enableEnvVar := &corev1.EnvVar{
		Name:  DDProcessAgentEnabledEnvVar,
		Value: "true",
	}

	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, enableEnvVar)
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.ProcessAgentContainerName, enableEnvVar)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *liveProcessFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
