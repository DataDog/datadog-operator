// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

const (
	// Volume names
	pluginDirVolumeName       = "plugin-dir"
	storageDirVolumeName      = "storage-dir"
	registrationDirVolumeName = "registration-dir"
	mountpointDirVolumeName   = "mountpoint-dir"
	apmSocketVolumeName       = "apm-socket"
	dsdSocketVolumeName       = "dsd-socket"

	// Volume mount paths
	pluginDirMountPath  = "/csi"
	storageDirMountPath = "/var/lib/datadog-csi-driver"
	mountpointDirPath   = "/var/lib/kubelet/pods"
	registrationDirPath = "/var/lib/kubelet/plugins_registry"
	registrarMountPath  = "/registration"

	// Host path templates (used with fmt.Sprintf and the CSI driver name)
	kubeletPluginsDirFmt = "/var/lib/kubelet/plugins/%s"
	kubeletStorageDirFmt = "/var/lib/kubelet/plugins/%s/storage"
	csiSocketPathFmt     = "/var/lib/kubelet/plugins/%s/csi.sock"

	// CSI socket path inside the container
	csiSocketAddress = "/csi/csi.sock"

	// Environment variable names
	envNodeID        = "NODE_ID"
	envDDAPMEnabled  = "DD_APM_ENABLED"
	envAddress       = "ADDRESS"
	envDriverRegSock = "DRIVER_REG_SOCK_PATH"

	// Container port
	csiDriverPort = int32(5000)

	// Pod labels
	appLabelKey                     = "app"
	admissionControllerEnabledLabel = "admission.datadoghq.com/enabled"

	// finalizerName is the finalizer for CSIDriver object cleanup
	finalizerName = "finalizer.datadoghq.com/csi-driver"
)
