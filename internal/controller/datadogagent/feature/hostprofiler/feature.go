package hostprofiler

import (
	"errors"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
)

var errHostPIDDisabledManually = errors.New("Host PID is required for host profiler")

type hostProfilerFeature struct {
	hostProfilerEnabled     bool
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
	if featureutils.HasFeatureEnableAnnotation(dda, featureutils.EnableHostProfilerAnnotation) {
		o.logger.Info("DEPRECATION WARNING: annotation 'agent.datadoghq.com/host-profiler-enabled' is deprecated; use 'spec.features.hostProfiler.enabled' instead")
	}

	o.hostProfilerEnabled = featureutils.IsHostProfilerEnabled(dda, ddaSpec)

	var reqComp feature.RequiredComponents
	if o.hostProfilerEnabled {
		reqComp = feature.RequiredComponents{
			Agent: feature.RequiredComponent{
				IsRequired: ptr.To(true),
				Containers: []apicommon.AgentContainerName{
					apicommon.CoreAgentContainerName,
					apicommon.HostProfiler,
				},
			},
		}
	}
	return reqComp
}

func (o *hostProfilerFeature) ManageDependencies(managers feature.ResourceManagers, provider string) error {
	if o.hostPIDDisabledManually {
		return errHostPIDDisabledManually
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
	managers.PodTemplateSpec().Spec.HostPID = *ptr.To(true)

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

	// (todo: mackjmr): remove this once IPC port is enabled by default. Enabling this port is required to fetch the API key from
	// core agent when secrets backend is used.
	agentIpcPortEnvVar := &corev1.EnvVar{
		Name:  common.DDAgentIpcPort,
		Value: "5009",
	}
	agentIpcConfigRefreshIntervalEnvVar := &corev1.EnvVar{
		Name:  common.DDAgentIpcConfigRefreshInterval,
		Value: "60",
	}
	for _, container := range []apicommon.AgentContainerName{apicommon.CoreAgentContainerName, apicommon.HostProfiler} {
		managers.EnvVar().AddEnvVarToContainer(container, agentIpcPortEnvVar)
		managers.EnvVar().AddEnvVarToContainer(container, agentIpcConfigRefreshIntervalEnvVar)
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
