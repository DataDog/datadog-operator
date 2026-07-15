// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

const (
	nvidiaDevicesMountPath  = "/var/run/nvidia-container-devices/all"
	nvidiaDevicesVolumeName = "nvidia-devices"
	devNullPath             = "/dev/null" // used to mount the NVIDIADevicesHostPath to /dev/null in the container, it's just used as a "signal" to the nvidia runtime to use the nvidia devices

	// On GKE COS nodes the NVIDIA driver libraries live under
	// /home/kubernetes/bin/nvidia/lib64 on the host. They are mounted into the
	// path where the nvidia-container-runtime expects the driver libraries so
	// that the agent and system-probe can load them.
	gkeCOSNVIDIADriverLib64VolumeName         = "gke-nvidia-driver-lib64"
	gkeCOSNVIDIADriverLib64HostRootVolumeName = "gke-nvidia-driver-lib64-hostroot"
	gkeCOSNVIDIADriverLib64HostPath           = "/home/kubernetes/bin/nvidia/lib64"
	gkeCOSNVIDIADriverLib64MountPath          = "/host/run/nvidia/driver/usr/lib/x86_64-linux-gnu"
	gkeCOSNVIDIADriverLib64HostRootMountPath  = "/host/root/run/nvidia/driver/usr/lib/x86_64-linux-gnu"

	// defaultGPURuntimeClass default runtime class for GPU pods
	defaultGPURuntimeClass = "nvidia"
)
