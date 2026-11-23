// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

const (
	openshiftConfigMapName = "datadog-controlplane-monitoring-openshift"
	defaultConfigMapName   = "datadog-controlplane-monitoring-default"
	eksConfigMapName       = "datadog-controlplane-monitoring-eks"
	talosConfigMapName     = "datadog-controlplane-monitoring-talos"

	kubeApiserverMetricsVolumeName  = "kube-apiserver-metrics-config"
	kubeControllerManagerVolumeName = "kube-controller-manager-config"
	kubeSchedulerVolumeName         = "kube-scheduler-config"
	etcdVolumeName                  = "etcd-config"

	kubeApiserverMetricsMountPath  = "/etc/datadog-agent/conf.d/kube_apiserver_metrics.d"
	kubeControllerManagerMountPath = "/etc/datadog-agent/conf.d/kube_controller_manager.d"
	kubeSchedulerMountPath         = "/etc/datadog-agent/conf.d/kube_scheduler.d"
	etcdMountPath                  = "/etc/datadog-agent/conf.d/etcd.d"

	etcdCertsVolumeName      = "etcd-client-certs"
	etcdCertsVolumeMountPath = "/etc/etcd-certs"
	etcdCertsSecretName      = "etcd-metric-client"
	etcdCertsSourceNamespace = "openshift-etcd-operator"

	disableEtcdAutoconfVolumeName      = "disable-etcd-autoconf"
	disableEtcdAutoconfVolumeMountPath = "/etc/datadog-agent/conf.d/etcd.d"

	// etcd client certs on Talos: host location, and where the etcd check reads them.
	talosEtcdCertsVolumeName = "etcd-certs"
	talosEtcdCertsHostPath   = "/system/secrets/etcd"
	talosEtcdCertsMountPath  = "/host/etc/kubernetes/pki/etcd"

	// Taint on Talos control-plane nodes.
	controlPlaneTaintKey = "node-role.kubernetes.io/control-plane"

	// Volumes that mask the Agent image's bundled autoconf for the checks that
	// run as cluster checks, so the node agent does not collect them a second
	// time when it runs on a control-plane node.
	disableKubeApiserverMetricsAutoconfVolumeName  = "disable-kube-apiserver-metrics-autoconf"
	disableKubeControllerManagerAutoconfVolumeName = "disable-kube-controller-manager-autoconf"
	disableKubeSchedulerAutoconfVolumeName         = "disable-kube-scheduler-autoconf"
)
