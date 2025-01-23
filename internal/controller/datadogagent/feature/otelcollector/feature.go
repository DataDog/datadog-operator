package otelcollector

import (
	"strconv"
	"strings"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	err := feature.Register(feature.OtelAgentIDType, buildOtelCollectorFeature)
	if err != nil {
		panic(err)
	}
}

func buildOtelCollectorFeature(options *feature.Options) feature.Feature {
	return &otelCollectorFeature{}
}

type otelCollectorFeature struct {
	customConfig    *v2alpha1.CustomConfig
	owner           metav1.Object
	configMapName   string
	ports           []*corev1.ContainerPort
	coreAgentConfig coreAgentConfig
}

type coreAgentConfig struct {
	extension_timeout *int
	extension_url     *string
	enabled           *bool
}

func (o otelCollectorFeature) ID() feature.IDType {
	return feature.OtelAgentIDType
}

func (o *otelCollectorFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	o.owner = dda
	if dda.Spec.Features.OtelCollector.Conf != nil {
		o.customConfig = dda.Spec.Features.OtelCollector.Conf
	}
	o.configMapName = constants.GetConfName(dda, o.customConfig, defaultOTelAgentConf)

	if dda.Spec.Features.OtelCollector.CoreConfig != nil {
		o.coreAgentConfig.enabled = dda.Spec.Features.OtelCollector.CoreConfig.Enabled
		o.coreAgentConfig.extension_timeout = dda.Spec.Features.OtelCollector.CoreConfig.ExtensionTimeout
		o.coreAgentConfig.extension_url = dda.Spec.Features.OtelCollector.CoreConfig.ExtensionURL
	}

	if len(dda.Spec.Features.OtelCollector.Ports) == 0 {
		o.ports = []*corev1.ContainerPort{
			{
				Name:          "otel-http",
				ContainerPort: 4318,
				HostPort:      4318,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "otel-grpc",
				ContainerPort: 4317,
				HostPort:      4317,
				Protocol:      corev1.ProtocolTCP,
			},
		}
	} else {
		o.ports = dda.Spec.Features.OtelCollector.Ports
	}

	var reqComp feature.RequiredComponents
	if apiutils.BoolValue(dda.Spec.Features.OtelCollector.Enabled) {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
					apicommon.OtelAgent,
				},
			},
		}

	}
	return reqComp
}

func (o *otelCollectorFeature) buildOTelAgentCoreConfigMap() (*corev1.ConfigMap, error) {
	if o.customConfig != nil && o.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(o.owner.GetNamespace(), o.customConfig.ConfigData, o.configMapName, otelConfigFileName)
	}
	return nil, nil
}

func (o otelCollectorFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// check if an otel collector config was provided. If not, use default.
	if o.customConfig == nil {
		o.customConfig = &v2alpha1.CustomConfig{}
	}
	if o.customConfig.ConfigData == nil && o.customConfig.ConfigMap == nil {
		var defaultConfig = defaultconfig.DefaultOtelCollectorConfig
		for _, port := range o.ports {
			if port.Name == "otel-grpc" {
				defaultConfig = strings.Replace(defaultConfig, "4317", strconv.Itoa(int(port.ContainerPort)), 1)
			}
			if port.Name == "otel-http" {
				defaultConfig = strings.Replace(defaultConfig, "4318", strconv.Itoa(int(port.ContainerPort)), 1)
			}
		}
		o.customConfig.ConfigData = &defaultConfig
	}

	// create configMap if customConfig is provided
	configMap, err := o.buildOTelAgentCoreConfigMap()
	if err != nil {
		return err
	}

	if configMap != nil {
		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, configMap); err != nil {
			return err
		}
	}
	return nil
}

func (o otelCollectorFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (o otelCollectorFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	var vol corev1.Volume
	if o.customConfig != nil && o.customConfig.ConfigMap != nil {
		// Custom config is referenced via ConfigMap
		vol = volume.GetVolumeFromConfigMap(
			o.customConfig.ConfigMap,
			o.configMapName,
			otelAgentVolumeName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(o.configMapName, otelAgentVolumeName)
	}

	// create volume
	managers.Volume().AddVolume(&vol)

	// [investigation needed]: When the user provides a custom config map, the file name *must be* otel-config.yaml. If we choose to allow
	// any file name, we would need to update both the volume mount here, as well as the otel-agent container command. I haven't seen this
	// done for other containers, which is why I think it's acceptable to force users to use the `otel-config.yaml` name.
	volMount := volume.GetVolumeMountWithSubPath(otelAgentVolumeName, common.ConfigVolumePath+"/"+otelConfigFileName, otelConfigFileName)
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.OtelAgent)

	// add ports
	for _, port := range o.ports {
		// bind container port to host port.
		port.HostPort = port.ContainerPort
		managers.Port().AddPortToContainer(apicommon.OtelAgent, port)
	}

	var enableEnvVar *corev1.EnvVar
	if o.coreAgentConfig.enabled != nil {
		if *o.coreAgentConfig.enabled {
			// only need to set env var if true, as it will default to false.
			enableEnvVar = &corev1.EnvVar{
				Name:  v2alpha1.DDOtelCollectorCoreConfigEnabled,
				Value: apiutils.BoolToString(o.coreAgentConfig.enabled),
			}
			managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName}, enableEnvVar)
		}
	} else {
		managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName}, &corev1.EnvVar{
			Name:  v2alpha1.DDOtelCollectorCoreConfigEnabled,
			Value: "true",
		})
	}

	if o.coreAgentConfig.extension_timeout != nil {
		managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName}, &corev1.EnvVar{
			Name:  v2alpha1.DDOtelCollectorCoreConfigExtensionTimeout,
			Value: strconv.Itoa(*o.coreAgentConfig.extension_timeout),
		})
	}
	if o.coreAgentConfig.extension_url != nil {
		managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName}, &corev1.EnvVar{
			Name:  v2alpha1.DDOtelCollectorCoreConfigExtensionURL,
			Value: *o.coreAgentConfig.extension_url,
		})
	}
	return nil
}

func (o otelCollectorFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o otelCollectorFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
