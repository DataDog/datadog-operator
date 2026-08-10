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
	gkeCOSNVIDIADriverLib64VolumeName = "gke-nvidia-driver-lib64"
	gkeCOSNVIDIADriverLib64HostPath   = "/home/kubernetes/bin/nvidia/lib64"
	gkeCOSNVIDIADriverLib64MountPath  = "/host/run/nvidia/driver/usr/lib/x86_64-linux-gnu"

	// defaultGPURuntimeClass default runtime class for GPU pods
	defaultGPURuntimeClass = "nvidia"

	// Kernel log collection for Xid errors. The journald tailer needs the host journal and
	// /etc/machine-id to resolve which journal to read.
	xidKernelLogsVolumeName = "gpu-xid-kernel-logs-config"
	xidKernelLogsConfigName = "journald.d"
	journalVolumeName       = "journald-log-path"
	journalHostPath         = "/var/log/journal"
	journalMountPath        = "/var/log/journal"
	machineIDVolumeName     = "gpu-machine-id"
	machineIDHostPath       = "/etc/machine-id"
	machineIDMountPath      = "/etc/machine-id"

	// xidKernelLogsConfigID is the journald `config_id`. The Agent's journald launcher keys its
	// tailers on `journald:<config_id>`, so any other kernel journald config MUST reuse this exact
	// value or the same messages get tailed twice. See the Helm chart companion change.
	xidKernelLogsConfigID = "kernel"
)
