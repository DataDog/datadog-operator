// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

const (
	openshiftConfigMapName = "datadog-controlplane-monitoring-openshift"
	defaultConfigMapName   = "datadog-controlplane-monitoring-default"
	eksConfigMapName       = "datadog-controlplane-monitoring-eks"

	kubeApiserverVolumeName         = "kube-apiserver-config"
	kubeControllerManagerVolumeName = "kube-controller-manager-config"
	kubeSchedulerVolumeName         = "kube-scheduler-config"
	etcdVolumeName                  = "etcd-config"

	kubeApiserverMountPath         = "/etc/datadog-agent/conf.d/kube_apiserver_metrics.d"
	kubeControllerManagerMountPath = "/etc/datadog-agent/conf.d/kube_controller_manager.d"
	kubeSchedulerMountPath         = "/etc/datadog-agent/conf.d/kube_scheduler.d"
	etcdMountPath                  = "/etc/datadog-agent/conf.d/etcd.d"

	etcdCertsVolumeName      = "etcd-client-certs"
	etcdCertsVolumeMountPath = "/etc/etcd-certs"
	etcdCertsSecretName      = "etcd-metric-client"

	disableEtcdAutoconfVolumeName      = "disable-etcd-autoconf"
	disableEtcdAutoconfVolumeMountPath = "/etc/datadog-agent/conf.d/etcd.d"
)
