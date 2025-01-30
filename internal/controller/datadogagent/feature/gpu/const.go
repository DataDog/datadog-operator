// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

const (
	nvidiaDevicesMountPath  = "/var/run/nvidia-container-devices/all"
 	nvidiaDevicesVolumeName = "nvidia-devices"
	devNullPath             = "/dev/null" // used to mount the NVIDIADevicesHostPath to /dev/null in the container, it's just used as a "signal" to the nvidia runtime to use the nvidia devices

	// defaultGPURuntimeClass default runtime class for GPU pods
	defaultGPURuntimeClass = "nvidia"
)
