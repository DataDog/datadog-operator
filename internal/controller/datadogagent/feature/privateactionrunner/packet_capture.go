// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package privateactionrunner

import (
	corev1 "k8s.io/api/core/v1"

	apicommon "github.com/DataDog/datadog-operator/api/datadoghq/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/common"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature"
	featureutils "github.com/DataDog/datadog-operator/internal/controller/datadogagent/feature/utils"
	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/volume"
)

const (
	packetCaptureSocketVolumeName = "sysprobe-capture-socket-dir"
	packetCaptureSocketDirPath    = "/var/run/sysprobe-capture"
	packetCaptureSocketPath       = packetCaptureSocketDirPath + "/capture.sock"

	DDPacketCaptureEnabled = "DD_PACKET_CAPTURE_ENABLED"
	DDPacketCaptureSocket  = "DD_PACKET_CAPTURE_SOCKET"
)

// packetCaptureSystemProbeCapabilities is the subset of the default system-probe
// capabilities needed to load eBPF programs and open raw packet sockets. The
// agent component package cannot be imported here (import cycle).
var packetCaptureSystemProbeCapabilities = []corev1.Capability{
	"SYS_ADMIN",
	"SYS_RESOURCE",
	"NET_ADMIN",
	"NET_RAW",
	"IPC_LOCK",
}

// packetCaptureConfig exposes a system-probe socket that serves only the
// packet_capture module to the node Private Action Runner. The full
// system-probe socket is never mounted into the runner container.
type packetCaptureConfig struct {
	enabled bool
}

func packetCaptureConfigFromAnnotations(annotations map[string]string) (packetCaptureConfig, error) {
	enabled, err := strictBoolAnnotation(annotations, featureutils.EnablePrivateActionRunnerPacketCaptureAnnotation)
	if err != nil {
		return packetCaptureConfig{}, err
	}
	return packetCaptureConfig{enabled: enabled}, nil
}

func (config packetCaptureConfig) addSocket(managers feature.PodTemplateManagers) {
	if !config.enabled {
		return
	}

	// Capture socket directory: system-probe creates the socket, the runner only dials it.
	captureVol, captureVolMount := volume.GetVolumesEmptyDir(packetCaptureSocketVolumeName, packetCaptureSocketDirPath, false)
	managers.Volume().AddVolume(&captureVol)
	managers.VolumeMount().AddVolumeMountToContainer(&captureVolMount, apicommon.SystemProbeContainerName)

	_, captureVolMountReadOnly := volume.GetVolumesEmptyDir(packetCaptureSocketVolumeName, packetCaptureSocketDirPath, true)
	managers.VolumeMount().AddVolumeMountToContainer(&captureVolMountReadOnly, apicommon.PrivateActionRunnerContainerName)

	// system-probe still opens its main socket, so it needs a writable socket
	// directory even when no other system-probe feature is enabled. It is not
	// mounted into the runner container.
	sysProbeSocketVol, sysProbeSocketVolMount := volume.GetVolumesEmptyDir(common.SystemProbeSocketVolumeName, common.SystemProbeSocketVolumePath, false)
	managers.Volume().AddVolume(&sysProbeSocketVol)
	managers.VolumeMount().AddVolumeMountToContainer(&sysProbeSocketVolMount, apicommon.SystemProbeContainerName)

	managers.SecurityContext().AddCapabilitiesToContainer(packetCaptureSystemProbeCapabilities, apicommon.SystemProbeContainerName)

	socketEnvVar := &corev1.EnvVar{Name: DDPacketCaptureSocket, Value: packetCaptureSocketPath}
	managers.EnvVar().AddEnvVarToContainers(
		[]apicommon.AgentContainerName{apicommon.SystemProbeContainerName, apicommon.PrivateActionRunnerContainerName},
		socketEnvVar,
	)
	managers.EnvVar().AddEnvVarToContainer(apicommon.SystemProbeContainerName, &corev1.EnvVar{Name: DDPacketCaptureEnabled, Value: "true"})
}
