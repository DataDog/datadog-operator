// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package flightrecorder

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
)

func init() {
	err := feature.Register(feature.FlightRecorderIDType, buildFlightRecorderFeature)
	if err != nil {
		panic(err)
	}
}

func buildFlightRecorderFeature(options *feature.Options) feature.Feature {
	f := &flightRecorderFeature{}

	if options != nil {
		f.logger = options.Logger
	}

	return f
}

type flightRecorderFeature struct {
	logger logr.Logger

	enabled bool
}

// ID returns the ID of the Feature
func (f *flightRecorderFeature) ID() feature.IDType {
	return feature.FlightRecorderIDType
}

// Configure is used to configure the feature from a v2alpha1.DatadogAgent instance.
// FlightRecorder is enabled by setting DD_EXPERIMENTAL_FLIGHTRECORDER_ENABLED=true in
// spec.override.nodeAgent.env (component-level) or
// spec.override.nodeAgent.containers.agent.env (container-level).
func (f *flightRecorderFeature) Configure(_ metav1.Object, ddaSpec *v2alpha1.DatadogAgentSpec, _ *v2alpha1.RemoteConfigConfiguration) feature.RequiredComponents {
	if nodeAgentOverride, ok := ddaSpec.Override[v2alpha1.NodeAgentComponentName]; ok {
		for _, env := range nodeAgentOverride.Env {
			if env.Name == common.DDExperimentalFlightRecorderEnabled && env.Value == "true" {
				f.enabled = true
			}
		}
		if coreContainer, ok := nodeAgentOverride.Containers[apicommon.CoreAgentContainerName]; ok && coreContainer != nil {
			for _, env := range coreContainer.Env {
				if env.Name == common.DDExperimentalFlightRecorderEnabled && env.Value == "true" {
					f.enabled = true
				}
			}
		}
	}

	var reqComp feature.RequiredComponents

	if f.enabled {
		reqComp.Agent = feature.RequiredComponent{
			IsRequired: &f.enabled,
			Containers: []apicommon.AgentContainerName{apicommon.FlightRecorderContainerName},
		}
	}

	return reqComp
}

// ManageDependencies allows a feature to manage its dependencies.
func (f *flightRecorderFeature) ManageDependencies(_ feature.ResourceManagers, _ string) error {
	return nil
}

// ManageClusterAgent allows a feature to configure the ClusterAgent's corev1.PodTemplateSpec.
func (f *flightRecorderFeature) ManageClusterAgent(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

// ManageSingleContainerNodeAgent allows a feature to configure the Agent container for the Node Agent's corev1.PodTemplateSpec
// if SingleContainerStrategy is enabled and can be used with the configured feature set.
func (f *flightRecorderFeature) ManageSingleContainerNodeAgent(managers feature.PodTemplateManagers, provider string) error {
	return f.ManageNodeAgent(managers, provider)
}

// ManageNodeAgent allows a feature to configure the Node Agent's corev1.PodTemplateSpec.
func (f *flightRecorderFeature) ManageNodeAgent(managers feature.PodTemplateManagers, _ string) error {
	if !f.enabled {
		return nil
	}

	// Set env vars on the core agent and trace agent to enable flightrecorder
	// and configure the socket path. The trace-agent runs its own flightrecorder
	// component to record trace stats via the hook system.
	for _, container := range []apicommon.AgentContainerName{
		apicommon.CoreAgentContainerName,
		apicommon.TraceAgentContainerName,
	} {
		managers.EnvVar().AddEnvVarToContainer(container, &corev1.EnvVar{
			Name:  common.DDFlightRecorderEnabled,
			Value: "true",
		})
		managers.EnvVar().AddEnvVarToContainer(container, &corev1.EnvVar{
			Name:  common.DDFlightRecorderSocketPath,
			Value: flightRecorderSocketFile,
		})
	}

	// Shared socket volume (emptyDir) for Unix socket communication between agent and flightrecorder.
	socketVol := corev1.Volume{
		Name: flightRecorderSocketVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	socketVolMount := corev1.VolumeMount{
		Name:      flightRecorderSocketVolumeName,
		MountPath: flightRecorderSocketPath,
	}
	managers.Volume().AddVolume(&socketVol)
	managers.VolumeMount().AddVolumeMountToContainers(
		&socketVolMount,
		[]apicommon.AgentContainerName{
			apicommon.CoreAgentContainerName,
			apicommon.TraceAgentContainerName,
			apicommon.FlightRecorderContainerName,
		},
	)

	// Data volume for Vortex output files, mounted only on the flightrecorder container.
	dataVol := corev1.Volume{
		Name: flightRecorderDataVolumeName,
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	}
	dataVolMount := corev1.VolumeMount{
		Name:      flightRecorderDataVolumeName,
		MountPath: flightRecorderDataPath,
	}
	managers.Volume().AddVolume(&dataVol)
	managers.VolumeMount().AddVolumeMountToContainer(
		&dataVolMount,
		apicommon.FlightRecorderContainerName,
	)

	return nil
}

// ManageClusterChecksRunner allows a feature to configure the ClusterChecksRunner's corev1.PodTemplateSpec.
func (f *flightRecorderFeature) ManageClusterChecksRunner(_ feature.PodTemplateManagers, _ string) error {
	return nil
}

// ManageOtelAgentGateway allows a feature to configure the OTel Agent Gateway's corev1.PodTemplateSpec.
func (f *flightRecorderFeature) ManageOtelAgentGateway(_ feature.PodTemplateManagers, _ string) error {
	return nil
}
