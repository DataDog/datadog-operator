// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"strconv"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"

	apicommon "github.com/DataDog/datadog-operator/apis/datadoghq/common"
	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"
)

func init() {
	err := feature.Register(feature.DogstatsdIDType, buildDogstatsdFeature)
	if err != nil {
		panic(err)
	}
}

func buildDogstatsdFeature(options *feature.Options) feature.Feature {
	dogstatsdFeat := &dogstatsdFeature{}

	return dogstatsdFeat
}

type dogstatsdFeature struct {
	hostPortEnabled  bool
	hostPortHostPort int32

	udsEnabled      bool
	udsHostFilepath string

	useHostNetwork         bool
	originDetectionEnabled bool
	mapperProfiles         *apicommonv1.CustomConfig
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
func (f *dogstatsdFeature) Configure(dda *v2alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	dogstatsd := dda.Spec.Features.Dogstatsd
	if apiutils.BoolValue(dogstatsd.HostPortConfig.Enabled) {
		f.hostPortEnabled = true
		f.hostPortHostPort = *dogstatsd.HostPortConfig.Port
	}
	// UDS is enabled by default
	if apiutils.BoolValue(dogstatsd.UnixDomainSocketConfig.Enabled) {
		f.udsEnabled = true
	}
	f.udsHostFilepath = *dogstatsd.UnixDomainSocketConfig.Path
	if apiutils.BoolValue(dogstatsd.OriginDetectionEnabled) {
		f.originDetectionEnabled = true
	}
	f.useHostNetwork = v2alpha1.IsHostNetworkEnabled(dda)
	if dogstatsd.MapperProfiles != nil {
		f.mapperProfiles = v2alpha1.ConvertCustomConfig(dogstatsd.MapperProfiles)
	}
	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{
				apicommonv1.CoreAgentContainerName,
			},
		},
	}
	return reqComp
}

// ConfigureV1 use to configure the feature from a v1alpha1.DatadogAgent instance.
func (f *dogstatsdFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) (reqComp feature.RequiredComponents) {
	config := dda.Spec.Agent.Config
	if config.HostPort != nil {
		f.hostPortEnabled = true
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
	f.useHostNetwork = v1alpha1.IsHostNetworkEnabled(dda)
	if config.Dogstatsd.MapperProfiles != nil {
		f.mapperProfiles = v1alpha1.ConvertCustomConfig(config.Dogstatsd.MapperProfiles)
	}
	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommonv1.AgentContainerName{
				apicommonv1.CoreAgentContainerName,
			},
		},
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *dogstatsdFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dogstatsdFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dogstatsdFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// udp
	dogstatsdPort := &corev1.ContainerPort{
		Name:          apicommon.DogstatsdHostPortName,
		ContainerPort: apicommon.DogstatsdHostPortHostPort,
		Protocol:      corev1.ProtocolUDP,
	}
	if f.hostPortEnabled {
		// f.hostPortHostPort will be 0 if HostPort is not set in v1alpha1
		// f.hostPortHostPort will default to 8125 in v2alpha1
		if f.hostPortHostPort != 0 {
			dogstatsdPort.HostPort = f.hostPortHostPort
			// if using host network, host port should be set and needs to match container port
			if f.useHostNetwork {
				dogstatsdPort.ContainerPort = f.hostPortHostPort
				managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
					Name:  apicommon.DDDogstatsdPort,
					Value: strconv.FormatInt(int64(f.hostPortHostPort), 10),
				})
			}
		}
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDDogstatsdNonLocalTraffic,
			Value: "true",
		})
	}
	managers.Port().AddPortToContainer(apicommonv1.CoreAgentContainerName, dogstatsdPort)

	// uds
	if f.udsEnabled {
		socketVol, socketVolMount := volume.GetVolumes(apicommon.DogstatsdUDSSocketName, f.udsHostFilepath, f.udsHostFilepath, true)
		managers.Volume().AddVolumeToContainer(&socketVol, &socketVolMount, apicommonv1.CoreAgentContainerName)
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDDogstatsdSocket,
			Value: f.udsHostFilepath,
		})
	}

	if f.originDetectionEnabled {
		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
			Name:  apicommon.DDDogstatsdOriginDetection,
			Value: "true",
		})
		if f.udsEnabled {
			managers.PodTemplateSpec().Spec.HostPID = true
		}
	}

	// mapper profiles
	if f.mapperProfiles != nil {
		// configdata
		if f.mapperProfiles.ConfigData != nil {
			managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
				Name:  apicommon.DDDogstatsdMapperProfiles,
				Value: apiutils.YAMLToJSONString(*f.mapperProfiles.ConfigData),
			})
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
func (f *dogstatsdFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
