// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadogcsidriver

const (
	// defaultCSIDriverImageRegistry is the registry for the CSI driver image
	defaultCSIDriverImageRegistry = "gcr.io/datadoghq"
	// defaultCSIDriverImageName is the default CSI driver container image name
	defaultCSIDriverImageName = "csi-driver"
	// defaultCSIDriverImageTag is the default CSI driver container image tag
	defaultCSIDriverImageTag = "1.2.1"

	// defaultRegistrarImageRegistry is the registry for the CSI node driver registrar image
	defaultRegistrarImageRegistry = "registry.k8s.io/sig-storage"
	// defaultRegistrarImageName is the default CSI node driver registrar image name
	defaultRegistrarImageName = "csi-node-driver-registrar"
	// defaultRegistrarImageTag is the default CSI node driver registrar image tag
	defaultRegistrarImageTag = "v2.0.1"

	// defaultAPMSocketPath is the default host path to the APM socket
	defaultAPMSocketPath = "/var/run/datadog/apm.socket"
	// defaultDSDSocketPath is the default host path to the DogStatsD socket
	defaultDSDSocketPath = "/var/run/datadog/dsd.socket"
)
