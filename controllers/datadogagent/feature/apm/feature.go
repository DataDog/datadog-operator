package apm

import (
	"path/filepath"

	apicommonv1 "github.com/DataDog/datadog-operator/apis/datadoghq/common/v1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/apis/datadoghq/v2alpha1"
	apiutils "github.com/DataDog/datadog-operator/apis/utils"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/feature"
	"github.com/DataDog/datadog-operator/controllers/datadogagent/object/volume"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
)

type apmFeature struct {
	enable          bool
	hostPortEnabled bool
	hostPortNumber  int32
	socketEnabled   bool
	socketPath      string
	socketDir       string
	logger          logr.Logger
}

func init() {
	err := feature.Register(feature.APMIDType, buildAPMFeature)
	if err != nil {
		panic(err)
	}
}

func buildAPMFeature(options *feature.Options) feature.Feature {
	apmFeature := &apmFeature{logger: options.Logger}

	return apmFeature
}

// Configure use to configure the internal of a Feature
// It should return `true` if the feature is enabled, else `false`.
func (f *apmFeature) Configure(dda *v2alpha1.DatadogAgent) feature.RequiredComponents {
	apmConfig := dda.Spec.Features.APM
	if apiutils.BoolValue(apmConfig.Enabled) {
		f.enable = true
	}

	if apiutils.BoolValue(apmConfig.HostPortConfig.Enabled) {
		f.hostPortEnabled = true

		// Port number will have been set by user explicitly or defaulted already
		f.hostPortNumber = *apmConfig.HostPortConfig.Port
	}

	if apiutils.BoolValue(apmConfig.UnixDomainSocketConfig.Enabled) {
		f.socketEnabled = true

		// UDS path will have been set by user explicitly or defaulted already
		f.socketPath = *apmConfig.UnixDomainSocketConfig.Path
		f.socketDir = filepath.Dir(f.socketPath)
	}

	return feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			Required:           &f.enable,
			RequiredContainers: []apicommonv1.AgentContainerName{apicommonv1.TraceAgentContainerName},
		},
	}
}

// ConfigureV1 use to configure the internal of a Feature from v1alpha1.DatadogAgent
// It should return `true` if the feature is enabled, else `false`.
func (f *apmFeature) ConfigureV1(dda *v1alpha1.DatadogAgent) feature.RequiredComponents {
	apmConfig := dda.Spec.Agent.Apm
	if apiutils.BoolValue(apmConfig.Enabled) {
		f.enable = true
	}

	if apmConfig.HostPort != nil {
		f.hostPortEnabled = true
		f.hostPortNumber = *apmConfig.HostPort
	} else {
		f.hostPortEnabled = false
	}

	if apiutils.BoolValue(apmConfig.UnixDomainSocket.Enabled) {
		f.socketEnabled = true
		if apmConfig.UnixDomainSocket.HostFilepath != nil {
			f.socketPath = *apmConfig.UnixDomainSocket.HostFilepath
		} else {
			f.socketPath = DefaultAPMSocketPath
		}
		f.socketDir = filepath.Dir(f.socketPath)
	}

	return feature.RequiredComponents{
		Agent: feature.RequiredComponent{
			Required:           &f.enable,
			RequiredContainers: []apicommonv1.AgentContainerName{apicommonv1.TraceAgentContainerName},
		},
	}
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageNodeAgent(managers feature.PodTemplateManagers) error {
	if f.enable {
		return f.enableAPM(managers)
	}
	return f.disableAPM(managers)
}

func (f *apmFeature) enableAPM(managers feature.PodTemplateManagers) error {
	// General Environment Variables
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.TraceAgentContainerName, &corev1.EnvVar{
		Name:  DDAPMEnabledEnvVar,
		Value: "true",
	})

	// Add Host Port
	if f.hostPortEnabled {
		managers.Port().AddPortToContainer(apicommonv1.TraceAgentContainerName, &corev1.ContainerPort{
			Name:          DefaultAPMPortName,
			HostPort:      f.hostPortNumber,
			ContainerPort: DefaultAPMPortNumber,
			Protocol:      corev1.ProtocolTCP,
		})
	}

	// Add UDS Support
	if f.socketEnabled {
		volume, volumeMount := volume.GetVolumes(APMSocketVolumeName, f.socketDir, f.socketDir)
		managers.Volume().AddVolumeToContainer(&volume, &volumeMount, apicommonv1.TraceAgentContainerName)

		managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
			Name:  DDAPMReceiverSocketEnvVar,
			Value: f.socketPath,
		})
	}

	return nil
}

func (f *apmFeature) disableAPM(managers feature.PodTemplateManagers) error {
	// Disable APM in core container
	managers.EnvVar().AddEnvVarToContainer(apicommonv1.CoreAgentContainerName, &corev1.EnvVar{
		Name:  DDAPMEnabledEnvVar,
		Value: "false",
	})

	return nil
}

// ManageDependencies allows a feature to manage its dependencies.
// Feature's dependencies should be added in the store.
func (f *apmFeature) ManageDependencies(managers feature.ResourceManagers) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageClusterAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageClusterCheckRunnerAgent allows a feature to configure the ClusterCheckRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageClusterCheckRunnerAgent(managers feature.PodTemplateManagers) error {
	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterCheckRunnerAgent's corev1.PodTemplateSpec
// It should do nothing if the feature doesn't need to configure it.
func (f *apmFeature) ManageClusterChecksRunner(managers feature.PodTemplateManagers) error {
	return nil
}
