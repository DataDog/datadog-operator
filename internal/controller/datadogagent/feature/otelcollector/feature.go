package otelcollector

import (
	"errors"
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
	"github.com/DataDog/datadog-operator/pkg/utils"
)

var errIncompatibleImageErr = errors.New("Incompatible OTel Agent image")

func init() {
	err := feature.Register(feature.OtelAgentIDType, buildOtelCollectorFeature)
	if err != nil {
		panic(err)
	}
}

func buildOtelCollectorFeature(options *feature.Options) feature.Feature {
	otelCollectorFeat := &otelCollectorFeature{}

	if options != nil {
		otelCollectorFeat.logger = options.Logger
	}

	return otelCollectorFeat
}

type otelCollectorFeature struct {
	customConfig    *v2alpha1.CustomConfig
	owner           metav1.Object
	configMapName   string
	ports           []*corev1.ContainerPort
	coreAgentConfig coreAgentConfig

	customConfigAnnotationKey   string
	customConfigAnnotationValue string

	forceEnableLocalService bool
	localServiceName        string

	incompatibleImage bool

	logger logr.Logger
}

type coreAgentConfig struct {
	extension_timeout *int
	extension_url     *string
	enabled           *bool
}

func (o *otelCollectorFeature) ID() feature.IDType {
	return feature.OtelAgentIDType
}

func (o *otelCollectorFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	var agentImageName string
	agentVersion := images.AgentLatestVersion
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.Image != nil {
			agentImageName = nodeAgent.Image.Name
			agentVersion = common.GetAgentVersionFromImage(*nodeAgent.Image)
		}
	}
	supportedVersion := utils.IsAboveMinVersion(agentVersion, "7.67.0-0", apiutils.NewBoolPointer(true))
	if !supportedVersion && agentImageName == "" {
		o.incompatibleImage = true
		o.logger.Error(errIncompatibleImageErr, "OTel Agent Standalone image requires agent version 7.67.0 or higher. Update the Agent version or use the agent image with -full tag instead.",
			"current_version", agentVersion)
		return feature.RequiredComponents{}
	}

	o.owner = dda
	if ddaSpec.Features.OtelCollector.Conf != nil {
		o.customConfig = ddaSpec.Features.OtelCollector.Conf
	}
	o.configMapName = constants.GetConfName(dda, o.customConfig, defaultOTelAgentConf)

	if ddaSpec.Global.LocalService != nil {
		o.forceEnableLocalService = apiutils.BoolValue(ddaSpec.Global.LocalService.ForceEnableLocalService)
	}
	o.localServiceName = constants.GetLocalAgentServiceName(dda.GetName(), ddaSpec)

	if ddaSpec.Features.OtelCollector.CoreConfig != nil {
		o.coreAgentConfig.enabled = ddaSpec.Features.OtelCollector.CoreConfig.Enabled
		o.coreAgentConfig.extension_timeout = ddaSpec.Features.OtelCollector.CoreConfig.ExtensionTimeout
		o.coreAgentConfig.extension_url = ddaSpec.Features.OtelCollector.CoreConfig.ExtensionURL
	}

	if len(ddaSpec.Features.OtelCollector.Ports) == 0 {
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
		o.ports = ddaSpec.Features.OtelCollector.Ports
	}

	var reqComp feature.RequiredComponents
	if apiutils.BoolValue(ddaSpec.Features.OtelCollector.Enabled) {
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

func (o *otelCollectorFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	if o.incompatibleImage {
		return errIncompatibleImageErr
	}
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

func (o *otelCollectorFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *otelCollectorFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if o.incompatibleImage {
		return errIncompatibleImageErr
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

	// (todo: mackjmr): remove this once IPC port is enabled by default. Enabling this port is required to fetch the API key from
	// core agent when secrets backend is used.
	agentIpcPortEnvVar := &corev1.EnvVar{
		Name:  DDAgentIpcPort,
		Value: "5009",
	}
	agentIpcConfigRefreshIntervalEnvVar := &corev1.EnvVar{
		Name:  DDAgentIpcConfigRefreshInterval,
		Value: "60",
	}
	// don't set env var if it was already set by user.
	mergeFunc := func(current, newEnv *corev1.EnvVar) (*corev1.EnvVar, error) {
		return current, nil
	}
	for _, container := range []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.OtelAgent} {
		managers.EnvVar().AddEnvVarToContainerWithMergeFunc(container, agentIpcPortEnvVar, mergeFunc)
		managers.EnvVar().AddEnvVarToContainerWithMergeFunc(container, agentIpcConfigRefreshIntervalEnvVar, mergeFunc)
	}

	var enableEnvVar *corev1.EnvVar
	if o.coreAgentConfig.enabled != nil {
		if *o.coreAgentConfig.enabled {
			// only need to set env var if true, as it will default to false.
			enableEnvVar = &corev1.EnvVar{
				Name:  DDOtelCollectorCoreConfigEnabled,
				Value: apiutils.BoolToString(o.coreAgentConfig.enabled),
			}
			managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.OtelAgent}, enableEnvVar)
		}
	} else {
		managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.OtelAgent}, &corev1.EnvVar{
			Name:  DDOtelCollectorCoreConfigEnabled,
			Value: "true",
		})
	}

	if o.coreAgentConfig.extension_timeout != nil {
		managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName}, &corev1.EnvVar{
			Name:  DDOtelCollectorCoreConfigExtensionTimeout,
			Value: strconv.Itoa(*o.coreAgentConfig.extension_timeout),
		})
	}
	if o.coreAgentConfig.extension_url != nil {
		managers.EnvVar().AddEnvVarToContainers([]apicommon.AgentContainerName{apicommon.CoreAgentContainerName}, &corev1.EnvVar{
			Name:  DDOtelCollectorCoreConfigExtensionURL,
			Value: *o.coreAgentConfig.extension_url,
		})
	}

	return nil
}

func (o *otelCollectorFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *otelCollectorFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *otelCollectorFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
