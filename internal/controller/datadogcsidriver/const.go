// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

const (
	// AppLabelKey is the Kubernetes label key on CSI node-server pods.
	AppLabelKey = "app"
	// NodeServerDaemonSetAppValue is the label value identifying CSI node-server pods
	// (and the default DaemonSet name).
	NodeServerDaemonSetAppValue = "datadog-csi-driver-node-server"

	// csiDsName is the default name of the CSI driver DaemonSet
	csiDsName = NodeServerDaemonSetAppValue
	// csiDriverName is the default name of the CSIDriver Kubernetes object
	csiDriverName = "k8s.csi.datadoghq.com"
	// defaultCSIDriverImageName is the default CSI driver container image name
	defaultCSIDriverImageName = "csi-driver"
	// defaultRegistrarImageName is the default CSI node driver registrar image name
	defaultRegistrarImageName = "csi-node-driver-registrar"
	// defaultRegistrarImageRegistry is the default CSI node driver registrar image registry
	defaultRegistrarImageRegistry = "registry.k8s.io/sig-storage"
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

	// Host paths
	kubeletPluginsDir = "/var/lib/kubelet/plugins/datadog.csi/driver"
	kubeletStorageDir = "/var/lib/kubelet/plugins/datadog.csi/storage"
	csiSocketPath     = "/var/lib/kubelet/plugins/datadog.csi/driver/csi.sock"

	// CSI socket path inside the container
	csiSocketAddress = "/csi/csi.sock"

	// Environment variable names
	envNodeID        = "NODE_ID"
	envAddress       = "ADDRESS"
	envDriverRegSock = "DRIVER_REG_SOCK_PATH"

	// Container port
	csiDriverPort = int32(5000)

	// Pod labels
	admissionControllerEnabledLabel = "admission.datadoghq.com/enabled"

	// CSIDriver annotations
	apmEnabledAnnotationKey = "csi.datadoghq.com/apm-enabled"

	// finalizerName is the finalizer for CSIDriver object cleanup
	finalizerName = "finalizer.datadoghq.com/csi-driver"
)
