package otelagentgateway

import (
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	err := feature.Register(feature.OtelAgentGatewayIDType, buildotelAgentGatewayFeatures)
	if err != nil {
		panic(err)
	}
}

func buildotelAgentGatewayFeatures(options *feature.Options) feature.Feature {
	otelCollectorFeat := &otelAgentGatewayFeatures{}

	if options != nil {
		otelCollectorFeat.logger = options.Logger
	}

	return otelCollectorFeat
}

type otelAgentGatewayFeatures struct {
	customConfig    *v2alpha1.CustomConfig
	owner           metav1.Object
	configMapName   string
	ports           []*corev1.ContainerPort

	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	forceEnableLocalService bool
	localServiceName        string

	logger logr.Logger
}

func (o *otelAgentGatewayFeatures) ID() feature.IDType {
	return feature.OtelAgentIDType
}

func (o *otelAgentGatewayFeatures) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	o.owner = dda
	if ddaSpec.Features.OtelAgentGateway.Conf != nil {
		o.customConfig = ddaSpec.Features.OtelAgentGateway.Conf
	}
	o.configMapName = constants.GetConfName(dda, o.customConfig, defaultOTelAgentConf)

	if ddaSpec.Global.LocalService != nil {
		o.forceEnableLocalService = apiutils.BoolValue(ddaSpec.Global.LocalService.ForceEnableLocalService)
	}
	o.localServiceName = constants.GetLocalAgentServiceName(dda.GetName(), ddaSpec)

	if len(ddaSpec.Features.OtelAgentGateway.Ports) == 0 {
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
		o.ports = ddaSpec.Features.OtelAgentGateway.Ports
	}

	var reqComp feature.RequiredComponents
	if apiutils.BoolValue(ddaSpec.Features.OtelAgentGateway.Enabled) {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.OtelAgent,
				},
			},
		}

	}
	return reqComp
}

func (o *otelAgentGatewayFeatures) buildOTelAgentCoreConfigMap() (*corev1.ConfigMap, error) {
	if o.customConfig != nil && o.customConfig.ConfigData != nil {
		cm, err := configmap.BuildConfigMapConfigData(o.owner.GetNamespace(), o.customConfig.ConfigData, o.configMapName, otelConfigFileName)
		if err != nil {
			return nil, err
		}

		// Add md5 hash annotation for configMap
		o.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.OtelAgentIDType)
		o.customConfigAnnotationValue, err = comparison.GenerateMD5ForSpec(o.customConfig.ConfigData)
		if err != nil {
			return cm, err
		}

		if o.customConfigAnnotationKey != "" && o.customConfigAnnotationValue != "" {
			annotations := object.MergeAnnotationsLabels(o.logger, cm.Annotations, map[string]string{o.customConfigAnnotationKey: o.customConfigAnnotationValue}, "*")
			cm.SetAnnotations(annotations)
		}

		return cm, nil
	}
	return nil, nil
}

func (o *otelAgentGatewayFeatures) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	// check if an otel collector config was provided. If not, use default.
	if o.customConfig == nil {
		o.customConfig = &v2alpha1.CustomConfig{}
	}

	grpcPort := 4317
	httpPort := 4318
	for _, port := range o.ports {
		if port.Name == "otel-grpc" {
			grpcPort = int(port.ContainerPort)
		}
		if port.Name == "otel-http" {
			httpPort = int(port.ContainerPort)
		}
	}

	if o.customConfig.ConfigData == nil && o.customConfig.ConfigMap == nil {
		var defaultConfig = defaultconfig.DefaultOtelCollectorConfig
		if grpcPort != 4317 {
			defaultConfig = strings.Replace(defaultConfig, "4317", strconv.Itoa(grpcPort), 1)
		}
		if httpPort != 4318 {
			defaultConfig = strings.Replace(defaultConfig, "4318", strconv.Itoa(httpPort), 1)
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

	platformInfo := managers.Store().GetPlatformInfo()
	internalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
	if common.ShouldCreateAgentLocalService(platformInfo.GetVersionInfo(), o.forceEnableLocalService) {
		otlpGrpcPort := &corev1.ServicePort{
			Name:       "otlpgrpcport",
			Port:       int32(grpcPort),
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(grpcPort),
		}
		otlpHttpPort := &corev1.ServicePort{
			Name:       "otlphttpport",
			Port:       int32(httpPort),
			Protocol:   corev1.ProtocolTCP,
			TargetPort: intstr.FromInt(httpPort),
		}
		if err := managers.ServiceManager().AddService(
			o.localServiceName,
			o.owner.GetNamespace(),
			common.GetAgentLocalServiceSelector(o.owner),
			[]corev1.ServicePort{*otlpGrpcPort, *otlpHttpPort},
			&internalTrafficPolicy,
		); err != nil {
			return err
		}
	}

	return nil
}

func (o *otelAgentGatewayFeatures) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *otelAgentGatewayFeatures) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *otelAgentGatewayFeatures) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *otelAgentGatewayFeatures) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *otelAgentGatewayFeatures) ManageOTelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	// // Use -full image for all containers
	image := &images.Image{}
	for i, container := range managers.PodTemplateSpec().Spec.Containers {
		image = images.FromString(container.Image).
			WithFull(true)
		// Note: if an image tag override is configured, this image tag will be overwritten
		managers.PodTemplateSpec().Spec.Containers[i].Image = image.ToString()
	}

	for i, container := range managers.PodTemplateSpec().Spec.InitContainers {
		image = images.FromString(container.Image).
			WithFull(true)
		// Note: if an image tag override is configured, this image tag will be overwritten
		managers.PodTemplateSpec().Spec.InitContainers[i].Image = image.ToString()
	}

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
	commands := []string{}
	if o.customConfig != nil && o.customConfig.ConfigMap != nil && len(o.customConfig.ConfigMap.Items) > 0 {
		for _, item := range o.customConfig.ConfigMap.Items {
			commands = append(commands, common.ConfigVolumePath+"/otel/"+item.Path)
		}
		volMount := corev1.VolumeMount{
			Name:      otelAgentVolumeName,
			MountPath: common.ConfigVolumePath + "/otel/",
		}
		managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.OtelAgent)

	} else {
		// This part in used in three paths:
		// - no conf.ConfigMap.Items provided, but conf.ConfigMap.Name provided. We assume only one item/ name otel-config.yaml
		// - when configData is used
		// - when no config is passed (we use DefaultOtelCollectorConfig)
		commands = append(commands, common.ConfigVolumePath+"/"+otelConfigFileName)
		volMount := volume.GetVolumeMountWithSubPath(otelAgentVolumeName, common.ConfigVolumePath+"/"+otelConfigFileName, otelConfigFileName)
		managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.OtelAgent)
	}

	// Add config to otel-agent container command
	for id, container := range managers.PodTemplateSpec().Spec.Containers {
		if container.Name == "otel-agent" {
			for _, command := range commands {
				managers.PodTemplateSpec().Spec.Containers[id].Args = append(managers.PodTemplateSpec().Spec.Containers[id].Args,
					"--config="+command,
				)
			}

		}
	}

	// Add md5 hash annotation for configMap
	if o.customConfigAnnotationKey != "" && o.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(o.customConfigAnnotationKey, o.customConfigAnnotationValue)
	}

	// add ports
	for _, port := range o.ports {
		// bind container port to host port.
		port.HostPort = port.ContainerPort
		managers.Port().AddPortToContainer(apicommon.OtelAgent, port)
	}

	managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.OtelAgent}, &corev1.EnvVar{
		Name:  DDOtelCollectorCoreConfigEnabled,
		Value: "true",
	})

	return nil
}
