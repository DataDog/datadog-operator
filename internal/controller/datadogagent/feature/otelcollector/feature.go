package otelcollector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/go-logr/logr"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelcollector/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/store"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/images"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

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
	customConfig       *v2alpha1.CustomConfig
	owner              metav1.Object
	configMapName      string
	ports              []*corev1.ContainerPort
	coreAgentConfig    coreAgentConfig
	createRBAC         bool
	serviceAccountName string

	customConfigAnnotationKey   string
	customConfigAnnotationValue string

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
	o.owner = dda
	if ddaSpec.Features.OtelCollector.Conf != nil {
		o.customConfig = ddaSpec.Features.OtelCollector.Conf
	}
	o.configMapName = constants.GetConfName(dda, o.customConfig, defaultOTelAgentConf)

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

	o.createRBAC = apiutils.BoolValue(dda.Spec.Features.OtelCollector.CreateRbac)
	o.serviceAccountName = constants.GetAgentServiceAccount(dda.Name, &dda.Spec)

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

// getEffectiveConfig returns the effective OpenTelemetry configuration
// It handles both custom config and default config cases
func (o *otelCollectorFeature) getEffectiveConfig(store store.StoreClient) (string, error) {
	// if custom config is provided via ConfigData
	if o.customConfig != nil && o.customConfig.ConfigData != nil {
		return *o.customConfig.ConfigData, nil
	}

	// if custom config is provided via ConfigMap
	if o.customConfig != nil && o.customConfig.ConfigMap != nil {
		ns := o.owner.GetNamespace()
		name := o.customConfig.ConfigMap.Name

		obj, ok := store.Get(kubernetes.ConfigMapKind, ns, name)
		if !ok {
			return "", fmt.Errorf("unable to get ConfigMap %s/%s from the store", ns, name)
		}

		cm, ok := obj.(*corev1.ConfigMap)
		if !ok {
			return "", fmt.Errorf("configMap %s/%s is not a corev1.ConfigMap", ns, name)
		}

		configData, ok := cm.Data[otelConfigFileName]
		if !ok {
			return "", fmt.Errorf("configMap %s/%s does not contain otel-config.yaml", ns, name)
		}

		return configData, nil
	}

	// use default config and override ports if needed
	configData := defaultconfig.DefaultOtelCollectorConfig
	for _, port := range o.ports {
		if port.Name == "otel-grpc" {
			configData = strings.Replace(configData, "4317", strconv.Itoa(int(port.ContainerPort)), 1)
		}
		if port.Name == "otel-http" {
			configData = strings.Replace(configData, "4318", strconv.Itoa(int(port.ContainerPort)), 1)
		}
	}

	return configData, nil
}

func (o *otelCollectorFeature) buildOTelAgentCoreConfigMap(configData *string) (*corev1.ConfigMap, error) {
	if configData == nil {
		return nil, fmt.Errorf("otelCollector configData is nil")
	}
	if *configData == "" {
		return nil, fmt.Errorf("otelCollector configData is empty")
	}

	// if custom config is not provided, build one with the configData
	if o.customConfig == nil {
		o.customConfig = &v2alpha1.CustomConfig{ConfigData: configData}
	}

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

// isK8sattributesRBACRequired checks if the OTel configuration has k8sattributes processor(s) enabled
// and whether any of them are configured with passthrough mode
func (o *otelCollectorFeature) isK8sattributesRBACRequired(configData string) (bool, error) {
	// otelConfig represents the OpenTelemetry Collector configuration structure
	type otelConfig struct {
		Processors map[string]struct {
			Passthrough bool `yaml:"passthrough"`
		} `yaml:"processors"`
	}

	var config otelConfig
	if err := yaml.Unmarshal([]byte(configData), &config); err != nil {
		o.logger.Error(err, "failed to parse OpenTelemetry configuration")
		return false, err
	}

	required := false
	for processorName, processorConfig := range config.Processors {
		if strings.HasPrefix(processorName, "k8sattributes") {
			// if any k8sattributes processor is not in passthrough mode, we need RBAC
			if !processorConfig.Passthrough {
				required = true
				break
			}
		}
	}

	return required, nil
}

func (o *otelCollectorFeature) ManageDependencies(managers feature.ResourceManagers) error {
	configData, err := o.getEffectiveConfig(managers.Store())
	if err != nil {
		return err
	}

	// if custom config is not provided via external ConfigMap, we need to create a configMap
	if !(o.customConfig != nil && o.customConfig.ConfigMap != nil) {
		configMap, err := o.buildOTelAgentCoreConfigMap(&configData)
		if err != nil {
			return err
		}

		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, configMap); err != nil {
			return err
		}
	}

	// Manage RBAC permission
	if o.createRBAC {
		rbacRequired, err := o.isK8sattributesRBACRequired(configData)
		if err != nil {
			return err
		}
		if rbacRequired {
			managers.RBACManager().AddClusterPolicyRules(o.owner.GetNamespace(), getRBACResourceName(o.owner), o.serviceAccountName, getK8sAttributesRBACPolicyRules())
		}
	}

	return nil
}

func (o *otelCollectorFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

func (o *otelCollectorFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
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

	// [investigation needed]: When the user provides a custom config map, the file name *must be* otel-config.yaml. If we choose to allow
	// any file name, we would need to update both the volume mount here, as well as the otel-agent container command. I haven't seen this
	// done for other containers, which is why I think it's acceptable to force users to use the `otel-config.yaml` name.
	volMount := volume.GetVolumeMountWithSubPath(otelAgentVolumeName, common.ConfigVolumePath+"/"+otelConfigFileName, otelConfigFileName)
	managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.OtelAgent)

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

func (o *otelCollectorFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
