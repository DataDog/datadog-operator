// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package gpu

// DDEnableGPUProbeEnvVar is the name of the system-probe gpu_monitoring module enablement knob
const DDEnableGPUProbeEnvVar = "DD_GPU_MONITORING_ENABLED"

// DDEnableGPUMonitoringCheckEnvVar is the name of the gpu core-check config enablement knob
const DDEnableGPUMonitoringCheckEnvVar = "DD_GPU_ENABLED"

// DDEnableNVMLDetectionEnvVar is deprecated and will be removed in a future release
const DDEnableNVMLDetectionEnvVar = "DD_ENABLE_NVML_DETECTION"
const NVIDIAVisibleDevicesEnvVar = "NVIDIA_VISIBLE_DEVICES"
