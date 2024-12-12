package otelagent

import (
	"strconv"
	"strings"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagent/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	otelAgentVolumeName = "otel-agent-config-volume"
	otelConfigFileName  = "otel-config.yaml"
)

func init() {
	err := feature.Register(feature.OtelAgentIDType, buildOtelAgentFeature)
	if err != nil {
		panic(err)
	}
}

func buildOtelAgentFeature(options *feature.Options) feature.Feature {
	return &otelAgentFeature{}
}

type otelAgentFeature struct {
	customConfig  *v2alpha1.CustomConfig
	owner         metav1.Object
	configMapName string
	ports         []*corev1.ContainerPort
}

func (o otelAgentFeature) ID() feature.IDType {
	return feature.OtelAgentIDType
}

func (o *otelAgentFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	o.owner = dda
	if dda.Spec.Features.OTelAgent.Conf != nil {
		o.customConfig = dda.Spec.Features.OTelAgent.Conf
	}
	o.configMapName = v2alpha1.GetConfName(dda, o.customConfig, v2alpha1.DefaultOTelAgentConf)

	if len(dda.Spec.Features.OTelAgent.Ports) == 0 {
		dda.Spec.Features.OTelAgent.Ports = []*corev1.ContainerPort{
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
	}
	o.ports = dda.Spec.Features.OTelAgent.Ports

	var reqComp feature.RequiredComponents
	if apiutils.BoolValue(dda.Spec.Features.OTelAgent.Enabled) {
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

func (o *otelAgentFeature) buildOTelAgentCoreConfigMap() (*corev1.ConfigMap, error) {
	if o.customConfig != nil && o.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(o.owner.GetNamespace(), o.customConfig.ConfigData, o.configMapName, otelConfigFileName)
	}
	return nil, nil
}

func (o otelAgentFeature) ManageDependencies(managers feature.ResourceManagers, components feature.RequiredComponents) error {
	// check if an otel collector config was provided. If not, use default.
	if o.customConfig == nil {
		o.customConfig = &v2alpha1.CustomConfig{}
	}
	if o.customConfig.ConfigData == nil && o.customConfig.ConfigMap == nil {
		var defaultConfig = defaultconfig.DefaultOtelCollectorConfig
		for _, port := range o.ports {
			if port.Name == "otel-grpc" {
				defaultConfig = strings.ReplaceAll(defaultConfig, "4317", strconv.Itoa(int(port.ContainerPort)))
			}
			if port.Name == "otel-http" {
				defaultConfig = strings.ReplaceAll(defaultConfig, "4318", strconv.Itoa(int(port.ContainerPort)))
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

func (o otelAgentFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (o otelAgentFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
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
	volMount := volume.GetVolumeMountWithSubPath(otelAgentVolumeName, apicommon.ConfigVolumePath+"/"+otelConfigFileName, otelConfigFileName)
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.OtelAgent)

	// add ports
	for _, port := range o.ports {
		port.HostPort = port.ContainerPort
		managers.Port().AddPortToContainer(apicommon.OtelAgent, port)
	}

	return nil
}

func (o otelAgentFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o otelAgentFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
