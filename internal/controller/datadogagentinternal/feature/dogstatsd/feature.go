// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dogstatsd

import (
	"path/filepath"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/merger"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagentinternal/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
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
	tagCardinality         string
	mapperProfiles         *v2alpha1.CustomConfig

	forceEnableLocalService bool
	localServiceName        string

	adpEnabled bool

	owner metav1.Object
}

// ID returns the ID of the Feature
func (f *dogstatsdFeature) ID() feature.IDType {
	return feature.DogstatsdIDType
}

// Configure is used to configure the feature from a v1alpha1.DatadogAgentInternal instance.
func (f *dogstatsdFeature) Configure(ddai *v1alpha1.DatadogAgentInternal) (reqComp feature.RequiredComponents) {
	dogstatsd := ddai.Spec.Features.Dogstatsd
	f.owner = ddai
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
	if dogstatsd.TagCardinality != nil {
		f.tagCardinality = *dogstatsd.TagCardinality
	}
	f.useHostNetwork = constants.IsHostNetworkEnabledDDAI(ddai, v2alpha1.NodeAgentComponentName)
	if dogstatsd.MapperProfiles != nil {
		f.mapperProfiles = dogstatsd.MapperProfiles
	}

	if ddai.Spec.Global.LocalService != nil {
		f.forceEnableLocalService = apiutils.BoolValue(ddai.Spec.Global.LocalService.ForceEnableLocalService)
	}
	f.localServiceName = constants.GetLocalAgentServiceNameDDAI(ddai)

	f.adpEnabled = featureutils.HasAgentDataPlaneAnnotation(ddai)

	reqComp = feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{
				apicommon.CoreAgentContainerName,
			},
		},
	}
	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *dogstatsdFeature) ManageDependencies(managers feature.ResourceManagers) error {
	platformInfo := managers.Store().GetPlatformInfo()
	// agent local service
	if common.ShouldCreateAgentLocalService(platformInfo.GetVersionInfo(), f.forceEnableLocalService) {
		dsdPort := &corev1.ServicePort{
			Protocol:   corev1.ProtocolUDP,
			TargetPort: intstr.FromInt(int(common.DefaultDogstatsdPort)),
			Port:       common.DefaultDogstatsdPort,
			Name:       defaultDogstatsdPortName,
		}
		if f.hostPortEnabled {
			dsdPort.Port = f.hostPortHostPort
			dsdPort.Name = dogstatsdHostPortName
			if f.useHostNetwork {
				dsdPort.TargetPort = intstr.FromInt(int(f.hostPortHostPort))
			}
		}
		serviceInternalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
		if err := managers.ServiceManager().AddService(f.localServiceName, f.owner.GetNamespace(), common.GetAgentLocalServiceSelector(f.owner), []corev1.ServicePort{*dsdPort}, &serviceInternalTrafficPolicy); err != nil {
			return err
		}
	}

	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dogstatsdFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
// It should do nothing if the feature doesn't need to configure it.
func (f *dogstatsdFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers) error {
	f.manageNodeAgent(apicommon.UnprivilegedSingleAgentContainerName, managers)

	// When ADP is enabled, we set `DD_USE_DOGSTATSD` to `false`, and `DD_ADP_ENABLED` to `true`.
	//
	// This disables DSD in the Core Agent, and additionally informs it that DSD is disabled because ADP is enabled and
	// taking over responsibilities, rather than DSD simply being disabled intentionally, such as in the case of the
	// Cluster Checks Runner.
	if f.adpEnabled {
		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  common.DDDogstatsdEnabled,
			Value: "false",
		})
	}

	return nil
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *dogstatsdFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	// When ADP is enabled, we apply the DSD configuration to the ADP container instead, and set `DD_USE_DOGSTATSD` to
	// `false` on the Core Agent container. This disables DSD in the Core Agent, and allows ADP to take over.
	//
	// While we _could_ leave the DSD-specific configuration set on the Core Agent -- it doesn't so matter as long as
	// DSD is disabled -- it's cleaner to remote it entirely to avoid confusion.
	if f.adpEnabled {
		f.manageNodeAgent(apicommon.AgentDataPlaneContainerName, managers)

		managers.EnvVar().AddEnvVarToContainer(apicommon.CoreAgentContainerName, &corev1.EnvVar{
			Name:  common.DDDogstatsdEnabled,
			Value: "false",
		})
	} else {
		f.manageNodeAgent(apicommon.CoreAgentContainerName, managers)
	}

	return nil
}

func (f *dogstatsdFeature) manageNodeAgent(agentContainerName apicommon.AgentContainerName, managers feature.PodTemplateManagers) error {
	// udp
	dogstatsdPort := &corev1.ContainerPort{
		Name:          defaultDogstatsdPortName,
		ContainerPort: common.DefaultDogstatsdPort,
		Protocol:      corev1.ProtocolUDP,
	}
	if f.hostPortEnabled {
		// f.hostPortHostPort will be 0 if HostPort is not set in v1alpha1
		// f.hostPortHostPort will default to 8125 in v2alpha1
		dsdPortEnvVarValue := common.DefaultDogstatsdPort
		if f.hostPortHostPort != 0 {
			dogstatsdPort.HostPort = f.hostPortHostPort
			// if using host network, host port should be set and needs to match container port
			if f.useHostNetwork {
				dogstatsdPort.ContainerPort = f.hostPortHostPort
				dsdPortEnvVarValue = int(f.hostPortHostPort)
			}
		}
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			// defaults to 8125 in datadog-agent code
			Name:  DDDogstatsdPort,
			Value: strconv.Itoa(dsdPortEnvVarValue),
		})
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDDogstatsdNonLocalTraffic,
			Value: "true",
		})
	}
	managers.Port().AddPortToContainer(agentContainerName, dogstatsdPort)

	// uds
	if f.udsEnabled {
		udsHostFolder := filepath.Dir(f.udsHostFilepath)
		sockName := filepath.Base(f.udsHostFilepath)
		socketVol, socketVolMount := volume.GetVolumes(common.DogstatsdSocketVolumeName, udsHostFolder, common.DogstatsdSocketLocalPath, false)
		volType := corev1.HostPathDirectoryOrCreate // We need to create the directory on the host if it does not exist.

		socketVol.VolumeSource.HostPath.Type = &volType
		managers.VolumeMount().AddVolumeMountToContainerWithMergeFunc(&socketVolMount, agentContainerName, merger.OverrideCurrentVolumeMountMergeFunction)
		managers.Volume().AddVolume(&socketVol)
		managers.EnvVar().AddEnvVar(&corev1.EnvVar{
			Name:  DDDogstatsdSocket,
			Value: filepath.Join(common.DogstatsdSocketLocalPath, sockName),
		})
	}

	if f.originDetectionEnabled {
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDDogstatsdOriginDetection,
			Value: "true",
		})
		managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
			Name:  DDDogstatsdOriginDetectionClient,
			Value: "true",
		})
		if f.udsEnabled {
			managers.PodTemplateSpec().Spec.HostPID = true
		}
		// Tag cardinality is only configured if origin detection is enabled.
		// The value validation happens at the Agent level - if the lower(string) is not `low`, `orchestrator` or `high`, the Agent defaults to `low`.
		if f.tagCardinality != "" {
			managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
				Name:  DDDogstatsdTagCardinality,
				Value: f.tagCardinality,
			})
		}
	}

	// mapper profiles
	if f.mapperProfiles != nil {
		// configdata
		if f.mapperProfiles.ConfigData != nil {
			managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
				Name:  DDDogstatsdMapperProfiles,
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
			managers.EnvVar().AddEnvVarToContainer(agentContainerName, &corev1.EnvVar{
				Name:      DDDogstatsdMapperProfiles,
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
