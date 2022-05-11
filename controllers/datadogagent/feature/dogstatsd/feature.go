// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"sigs.k8s.io/yaml"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.DogStatsDIDType, buildDogStatsDFeature)
	if err != nil {
		panic(err)
	}
}

func buildDogStatsDFeature(options *feature.Options) feature.Feature {
	dogStatsDFeat := &dogStatsDFeature{}

	return dogStatsDFeat
}

type dogStatsDFeature struct {
	hostPortEnabled  bool
	hostPortHostPort int32

	udsEnabled      bool
	udsHostFilepath string

	originDetectionEnabled bool
	mapperProfiles         *apicommonv1.CustomConfig
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *dogStatsDFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	dogstatsd := dda.Spec.Features.Dogstatsd
	if apiutils.BoolValue(dogstatsd.HostPortConfig.Enabled) {
		f.hostPortEnabled = true
	}
	if dogstatsd.HostPortConfig.Port != nil {
		f.hostPortHostPort = *dogstatsd.HostPortConfig.Port
	}
	if apiutils.BoolValue(dogstatsd.UnixDomainSocketConfig.Enabled) {
		f.udsEnabled = true
	}
	f.udsHostFilepath = *dogstatsd.UnixDomainSocketConfig.Path
	if apiutils.BoolValue(dogstatsd.OriginDetectionEnabled) {
		f.originDetectionEnabled = true
	}
	if dogstatsd.MapperProfiles != nil {
		f.mapperProfiles = v2alpha1.ConvertCustomConfig(dogstatsd.MapperProfiles)
	}
	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			Containers: []apicommonv1.AgentContainerName{
				apicommonv1.CoreAgentContainerName,
			},
		},
	}
	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *dogStatsDFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	config := dda.Spec.Agent.Config
	f.hostPortEnabled = true
	if config.HostPort != nil {
		f.hostPortHostPort = *config.HostPort
	}
	if apiutils.BoolValue(config.Dogstatsd.UnixDomainSocket.Enabled) {
		f.udsEnabled = true
	}
	if config.Dogstatsd.UnixDomainSocket.HostFilepath != nil {
		f.udsHostFilepath = *config.Dogstatsd.UnixDomainSocket.HostFilepath
	}
	if apiutils.BoolValue(config.Dogstatsd.DogstatsdOriginDetection) {
		f.originDetectionEnabled = true
	}
	if config.Dogstatsd.MapperProfiles != nil {
		f.mapperProfiles = v1alpha1.ConvertCustomConfig(config.Dogstatsd.MapperProfiles)
	}
	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			Containers: []apicommonv1.AgentContainerName{
				apicommonv1.CoreAgentContainerName,
			},
		},
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *dogStatsDFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dogStatsDFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dogStatsDFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// udp
	if f.hostPortEnabled {
		if f.hostPortHostPort != 0 {
			managers.Port().AddPortToContainer(apicommonv1.CoreAgentContainerName, &corev1.ContainerPort{
				Name:          apicommon.DogStatsDHostPortName,
				HostPort:      f.hostPortHostPort,
				ContainerPort: apicommon.DogStatsDHostPortHostPort,
				Protocol:      corev1.ProtocolUDP,
			})
		} else {
			managers.Port().AddPortToContainer(apicommonv1.CoreAgentContainerName, &corev1.ContainerPort{
				Name:          apicommon.DogStatsDHostPortName,
				HostPort:      apicommon.DogStatsDHostPortHostPort,
				ContainerPort: apicommon.DogStatsDHostPortHostPort,
				Protocol:      corev1.ProtocolUDP,
			})
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDDogStatsDNonLocalTraffic,
			Value: "true",
		})
		// only add if uds origin detection is not enabled to prevent duplicates
		if f.originDetectionEnabled && !f.udsEnabled {
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDDogstatsdOriginDetection,
				Value: "true",
			})
		}
	}

	// uds
	if f.udsEnabled {
		socketVol, socketVolMount := volume.GetVolumes(apicommon.DogStatsDUDSSocketName, f.udsHostFilepath, f.udsHostFilepath, apicommon.DogStatsDUDSHostFilepathReadOnly)
		managers.Volume().AddVolumeToContainer(&socketVol, &socketVolMount, apicommonv1.CoreAgentContainerName)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDDogStatsDSocket,
			Value: f.udsHostFilepath,
		})
		if f.originDetectionEnabled {
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDDogstatsdOriginDetection,
				Value: "true",
			})
			// set hostPID
			managers.PodTemplateSpec().Spec.HostPID = true
		}
	}

	// mapper profiles
	if f.mapperProfiles != nil {
		// configdata
		if f.mapperProfiles.ConfigData != nil {
			if jsonValue, err := yaml.YAMLToJSON([]byte(*f.mapperProfiles.ConfigData)); err == nil {
				managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
					Name:  apicommon.DDDogstatsdMapperProfiles,
					Value: string(jsonValue),
				})
			}
			// ignore configmap if configdata is set
			return nil
		}
		// configmap
		if f.mapperProfiles.ConfigMap != nil {
			cmSelector := corev1.ConfigMapKeySelector{}
			cmSelector.Name = f.mapperProfiles.ConfigMap.Name
			cmSelector.Key = f.mapperProfiles.ConfigMap.Items[0].Key
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
				Name:      apicommon.DDDogstatsdMapperProfiles,
				ValueFrom: &corev1.EnvVarSource{ConfigMapKeyRef: &cmSelector},
			})
		}
	}
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dogStatsDFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
