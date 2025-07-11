// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

const (
	openshiftConfigMapName = "datadog-controlplane-configuration-openshift"
	defaultConfigMapName   = "datadog-controlplane-configuration-default"
	eksConfigMapName       = "datadog-controlplane-configuration-eks"
	otherConfigMapName     = "datadog-controlplane-configuration-unknown"

	controlPlaneMonitoringVolumeName      = "controlplane-config"
	controlPlaneMonitoringVolumeMountPath = "/etc/datadog-agent/conf.d"
	emptyDirVolumeName                    = "agent-conf-d-writable"

	etcdCertsVolumeName      = "etcd-client-certs"
	etcdCertsVolumeMountPath = "/etc/etcd-certs"
	etcdCertsSecretName      = "etcd-metric-client"
)
