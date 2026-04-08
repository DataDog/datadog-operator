// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

const (
	// csiDsName is the default name of the CSI driver DaemonSet
	csiDsName = "datadog-csi-driver-node-server"
	// csiDriverName is the default name of the CSIDriver Kubernetes object
	csiDriverName = "k8s.csi.datadoghq.com"
	// defaultCSIDriverImageName is the default CSI driver container image name
	defaultCSIDriverImageName = "csi-driver"
	// defaultRegistrarImageName is the default CSI node driver registrar image name
	defaultRegistrarImageName = "csi-node-driver-registrar"
	// defaultAPMSocketPath is the default host path to the APM socket
	defaultAPMSocketPath = "/var/run/datadog/apm.socket"
	// defaultDSDSocketPath is the default host path to the DogStatsD socket
	defaultDSDSocketPath = "/var/run/datadog/dsd.socket"

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
