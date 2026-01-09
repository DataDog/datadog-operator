package hostprofiler

import (
	"errors"
	"fmt"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/api/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/hostprofiler/defaultconfig"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/controller/utils/comparison"
	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

var errHostPIDDisabledManually = errors.New("Host PID is required for host profiler")

type hostProfilerFeature struct {
	owner                       metav1.Object
	customConfig                *v2alpha1.CustomConfig
	customConfigAnnotationKey   string
	customConfigAnnotationValue string
	configMapName               string
	hostProfilerEnabled         bool

	hostPIDDisabledManually bool

	logger logr.Logger
}

func init() {
	err := feature.Register(feature.HostProfilerIDType, buildHostProfilerFeature)
	if err != nil {
		panic(err)
	}
}

func buildHostProfilerFeature(options *feature.Options) feature.Feature {

	hostProfilerFeat := &hostProfilerFeature{}

	if options != nil {
		hostProfilerFeat.logger = options.Logger
	}

	return hostProfilerFeat
}

func (o *hostProfilerFeature) ID() feature.IDType {
	return feature.HostProfilerIDType
}

func (o *hostProfilerFeature) Configure(dda metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	o.hostProfilerEnabled = featureutils.HasHostProfilerAnnotation(dda)
	o.logger.Info("The host profiler feature is experimental and subject to change")

	// If a user disabled HostPID manually, error out rather than enabling it for them.
	if nodeAgent, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		if nodeAgent.HostPID != nil && apiutils.BoolValue(nodeAgent.HostPID) == false {
			o.logger.Error(errHostPIDDisabledManually, "Host PID is required to run the host profiler. Please enable host PID or disable the host profiler")
			o.hostPIDDisabledManually = true
			return feature.RequiredComponents{}
		}
	}

	o.owner = dda
	if value, ok := featureutils.HasHostProfilerConfigAnnotion(dda, featureutils.HostProfilerConfigDataAnnotion); ok {
		o.customConfig = &v2alpha1.CustomConfig{
			ConfigData: apiutils.NewStringPointer(value),
		}

	}

	if value, ok := featureutils.HasHostProfilerConfigAnnotion(dda, featureutils.HostProfilerConfigMapNameAnnotion); ok {
		o.customConfig = &v2alpha1.CustomConfig{
			ConfigMap: &v2alpha1.ConfigMapConfig{
				Name: value,
			},
		}
	}
	o.configMapName = constants.GetConfName(dda, o.customConfig, defaultHostProfilerConf)

	var reqComp feature.RequiredComponents
	if o.hostProfilerEnabled {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: apiutils.NewBoolPointer(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
					apicommon.HostProfiler,
				},
			},
		}

	}
	return reqComp
}

func (o *hostProfilerFeature) buildHostProfilerCoreConfigMap() (*corev1.ConfigMap, error) {
	if o.customConfig != nil && o.customConfig.ConfigData != nil {
		cm, err := configmap.BuildConfigMapConfigData(o.owner.GetNamespace(), o.customConfig.ConfigData, o.configMapName, hostProfilerConfigFileName)
		if err != nil {
			return nil, err
		}

		// Add md5 hash annotation for configMap
		o.customConfigAnnotationKey = object.GetChecksumAnnotationKey(feature.HostProfilerIDType)
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

func (o *hostProfilerFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	if o.hostPIDDisabledManually {
		return errHostPIDDisabledManually
	}

	// check if an otel collector config was provided. If not, use default.
	if o.customConfig == nil {
		o.customConfig = &v2alpha1.CustomConfig{}
	}

	if o.customConfig.ConfigData == nil && o.customConfig.ConfigMap == nil {
		var defaultConfig = defaultconfig.DefaultHostProfilerConfig
		o.customConfig.ConfigData = &defaultConfig
	}

	// create configMap if customConfig is provided
	configMap, err := o.buildHostProfilerCoreConfigMap()
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

func (o *hostProfilerFeature) ManageClusterAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *hostProfilerFeature) ManageNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	if o.hostPIDDisabledManually {
		return errHostPIDDisabledManually
	}

	// Host PID
	managers.PodTemplateSpec().Spec.HostPID = *apiutils.NewBoolPointer(true)

	// Tracingfs volume
	volumeTracingfs := corev1.Volume{
		Name: "tracingfs",
		VolumeSource: corev1.VolumeSource{
			HostPath: &corev1.HostPathVolumeSource{
				Path: "/sys/kernel/tracing",
			},
		},
	}
	managers.Volume().AddVolume(&volumeTracingfs)

	tracingfsMount := corev1.VolumeMount{
		Name:      "tracingfs",
		MountPath: "/sys/kernel/tracing",
		ReadOnly:  true,
	}
	managers.VolumeMount().AddVolumeMountToContainer(&tracingfsMount, apicommon.HostProfiler)

	// Config volume
	var vol corev1.Volume
	if o.customConfig != nil && o.customConfig.ConfigMap != nil {
		// Custom config is referenced via ConfigMap
		vol = volume.GetVolumeFromConfigMap(
			o.customConfig.ConfigMap,
			o.configMapName,
			hostProfilerVolumeName,
		)
	} else {
		// Otherwise, configMap was created in ManageDependencies (whether from CustomConfig.ConfigData or using defaults, so mount default volume)
		vol = volume.GetBasicVolume(o.configMapName, hostProfilerVolumeName)
	}

	// create volume
	managers.Volume().AddVolume(&vol)
	commands := []string{}
	if o.customConfig != nil && o.customConfig.ConfigMap != nil && len(o.customConfig.ConfigMap.Items) > 0 {
		for _, item := range o.customConfig.ConfigMap.Items {
			commands = append(commands, common.ConfigVolumePath+"/otel/"+item.Path)
		}
		volMount := corev1.VolumeMount{
			Name:      hostProfilerVolumeName,
			MountPath: common.ConfigVolumePath + "/otel/",
		}
		managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.HostProfiler)
	} else {
		// This part in used in three paths:
		// - no conf.ConfigMap.Items provided, but conf.ConfigMap.Name provided. We assume only one item/ name host-profiler-config.yaml
		// - when configData is used
		// - when no config is passed (we use DefaultOtelCollectorConfig)
		commands = append(commands, common.ConfigVolumePath+"/"+hostProfilerConfigFileName)
		volMount := volume.GetVolumeMountWithSubPath(hostProfilerVolumeName, common.ConfigVolumePath+"/"+hostProfilerConfigFileName, hostProfilerConfigFileName)
		managers.VolumeMount().AddVolumeMountToContainer(&volMount, apicommon.HostProfiler)
	}

	// Add config to host-profiler container command
	for id, container := range managers.PodTemplateSpec().Spec.Containers {
		if container.Name == "host-profiler" {
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
	return nil
}

func (o *hostProfilerFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *hostProfilerFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers, provider string) error {
	return nil
}

func (o *hostProfilerFeature) ManageOtelAgentGateway(managers feature.PodTemplateManagers, provider string) error {
	return nil
}
