// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

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
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/otelagentgateway/defaultconfig"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func init() {
	err := feature.Register(feature.OtelAgentGatewayIDType, buildOtelAgentGatewayFeature)
	if err != nil {
		panic(err)
	}
}

type otelAgentGatewayFeature struct {
	owner                       metav1.Object
	logger                      logr.Logger
	ports                       []*corev1.ContainerPort
	localServiceName            string
	customConfig                *v2alpha1.CustomConfig
	configMapName               string
	customConfigAnnotationKey   string
	customConfigAnnotationValue string
}

func buildOtelAgentGatewayFeature(options *feature.Options) feature.Feature {
	feature := &otelAgentGatewayFeature{}
	if options != nil {
		feature.logger = options.Logger
	}
	return feature
}

// ID returns the ID of the Feature
func (f *otelAgentGatewayFeature) ID() feature.IDType {
	return feature.OtelAgentGatewayIDType
}

func (f *otelAgentGatewayFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) (reqComp feature.RequiredComponents) {
	if ddaSpec.Features.OtelAgentGateway == nil || !apiutils.BoolValue(ddaSpec.Features.OtelAgentGateway.Enabled) {
		return reqComp
	}

	f.owner = dda

	reqComp = feature.RequiredComponents{
		OtelAgentGateway: feature.RequiredComponent{
			IsRequired: apiutils.NewBoolPointer(true),
			Containers: []apicommon.AgentContainerName{apicommon.OtelAgent},
		},
	}

	f.localServiceName = constants.GetOTelAgentGatewayServiceName(dda.GetName())
	if len(ddaSpec.Features.OtelAgentGateway.Ports) == 0 {
		f.ports = []*corev1.ContainerPort{
			{
				Name:          "otel-grpc",
				ContainerPort: 4317,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "otel-http",
				ContainerPort: 4318,
				Protocol:      corev1.ProtocolTCP,
			},
		}
	} else {
		f.ports = ddaSpec.Features.OtelAgentGateway.Ports
	}

	if ddaSpec.Features.OtelAgentGateway.Conf != nil {
		f.customConfig = ddaSpec.Features.OtelAgentGateway.Conf
	}
	f.configMapName = constants.GetConfName(dda, f.customConfig, defaultOTelAgentGatewayConf)

	return reqComp
}

func (o *otelAgentGatewayFeature) buildOTelAgentCoreConfigMap() (*corev1.ConfigMap, error) {
	if o.customConfig != nil && o.customConfig.ConfigData != nil {
		cm, err := configmap.BuildConfigMapConfigData(o.owner.GetNamespace(), o.customConfig.ConfigData, o.configMapName, otelConfigFileName)
		if err != nil {
			return nil, err
		}

		// Add md5 hash annotation for configMap
		o.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.OtelAgentGatewayIDType)
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

func (f *otelAgentGatewayFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	// check if an otel collector config was provided. If not, use default.
	if f.customConfig == nil {
		f.customConfig = &v2alpha1.CustomConfig{}
	}

	grpcPort := 4317
	httpPort := 4318
	for _, port := range f.ports {
		if port.Name == "otel-grpc" {
			grpcPort = int(port.ContainerPort)
		}
		if port.Name == "otel-http" {
			httpPort = int(port.ContainerPort)
		}
	}

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

	if f.customConfig.ConfigData == nil && f.customConfig.ConfigMap == nil {
		var defaultConfig = defaultconfig.DefaultOtelAgentGatewayConfig
		if grpcPort != 4317 {
			defaultConfig = strings.Replace(defaultConfig, "4317", strconv.Itoa(grpcPort), 1)
		}
		if httpPort != 4318 {
			defaultConfig = strings.Replace(defaultConfig, "4318", strconv.Itoa(httpPort), 1)
		}
		f.customConfig.ConfigData = &defaultConfig
	}

	// create configMap if customConfig is provided
	configMap, err := f.buildOTelAgentCoreConfigMap()
	if err != nil {
		return err
	}

	if configMap != nil {
		if err := managers.Store().AddOrUpdate(kubernetes.ConfigMapKind, configMap); err != nil {
			return err
		}
	}

	internalTrafficPolicy := corev1.ServiceInternalTrafficPolicyLocal
	if err := managers.ServiceManager().AddService(
		f.localServiceName,
		f.owner.GetNamespace(),
		common.GetOtelAgentGatewayServiceSelector(f.owner),
		[]corev1.ServicePort{*otlpGrpcPort, *otlpHttpPort},
		&internalTrafficPolicy,
	); err != nil {
		return err
	}
	return nil
}

func (f *otelAgentGatewayFeature) ManageClusterAgent(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Cluster Agent
	return nil
}

func (f *otelAgentGatewayFeature) ManageSingleContainerNodeAgent(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Node Agent
	return nil
}

func (f *otelAgentGatewayFeature) ManageNodeAgent(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Node Agent
	return nil
}

func (f *otelAgentGatewayFeature) ManageClusterChecksRunner(feature.PodTemplateManagers, string) error {
	// OtelAgentGateway doesn't need to configure the Cluster Checks Runner
	return nil
}

func (f *otelAgentGatewayFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	var vol corev1.Volume
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		// Custom config is referenced via ConfigMap
		vol = volume.GetVolumeFromConfigMap(
			f.customConfig.ConfigMap,
			f.configMapName,
			otelAgentVolumeName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(f.configMapName, otelAgentVolumeName)
	}

	// create volume
	managers.Volume().AddVolume(&vol)

	if f.customConfig != nil && f.customConfig.ConfigMap != nil && len(f.customConfig.ConfigMap.Items) > 0 {
		volMount := corev1.VolumeMount{
			Name:      otelAgentVolumeName,
			MountPath: common.ConfigVolumePath + "/otel/",
		}
		managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.OtelAgent)
	} else {
		// This part is used in three paths:
		// - no conf.ConfigMap.Items provided, but conf.ConfigMap.Name provided. We assume only one item/ name otel-config.yaml
		// - when configData is used
		// - when no config is passed (we use DefaultOtelCollectorConfig)
		volMount := volume.GetVolumeMountWithSubPath(otelAgentVolumeName, common.ConfigVolumePath+"/"+otelConfigFileName, otelConfigFileName)
		managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.OtelAgent)
	}

	// Add md5 hash annotation for configMap
	if f.customConfigAnnotationKey != "" && f.customConfigAnnotationValue != "" {
		managers.Annotation().AddAnnotation(f.customConfigAnnotationKey, f.customConfigAnnotationValue)
	}

	// Add ports
	for _, port := range f.ports {
		managers.Port().AddPortToContainer(apicommon.OtelAgent, port)
	}
	return nil
}
