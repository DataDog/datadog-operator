// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func (f *controlPlaneMonitoringFeature) buildControlPlaneMonitoringConfigMap(provider string, configMapName string) (*corev1.ConfigMap, error) {
	var configMap *corev1.ConfigMap
	switch provider {
	case kubernetes.OpenShiftProviderLabel:
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: f.owner.GetNamespace(),
			},
			Data: map[string]string{
				"kube_apiserver_metrics.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kubernetes"
      namespace: "default"
      resolve: "ip"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/metrics"
    bearer_token_auth: true`,

				"kube_controller_manager.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kube-controller-manager"
      namespace: "openshift-kube-controller-manager"
      resolve: "ip"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/metrics"
    ssl_verify: false
    bearer_token_auth: true`,

				"kube_scheduler.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "scheduler"
      namespace: "openshift-kube-scheduler"
      resolve: "ip"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/metrics"
    ssl_verify: false
    bearer_token_auth: true`,

				"etcd.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "etcd"
      namespace: "openshift-etcd"
      resolve: "ip"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/metrics"
    ssl_verify: false
    tls_cert: "/etc/etcd-certs/tls.crt"
    tls_private_key: "/etc/etcd-certs/tls.key"`,
			},
		}
	case kubernetes.EKSProviderLabel:
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: f.owner.GetNamespace(),
			},
			Data: map[string]string{
				"kube_apiserver_metrics.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kubernetes"
      namespace: "default"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/metrics"
    bearer_token_auth: true`,

				"kube_controller_manager.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kubernetes"
      namespace: "default"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/apis/metrics.eks.amazonaws.com/v1/kcm/container/metrics"
    extra_headers:
        accept: "*/*"
    bearer_token_auth: true
    tls_ca_cert: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"`,

				"kube_scheduler.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kubernetes"
      namespace: "default"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/apis/metrics.eks.amazonaws.com/v1/ksh/container/metrics"
    extra_headers:
        accept: "*/*"
    bearer_token_auth: true
    tls_ca_cert: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt"`,
			},
		}
	case kubernetes.TalosProvider:
		// apiserver, scheduler and controller-manager are cluster checks; etcd is
		// node-local and mounted into the node agent instead.
		//
		// The `kubernetes` Endpoints resolves to a control-plane address, so
		// %%host%% reaches the control plane from a check running elsewhere.
		// Scheduler and controller-manager need literal ports (%%port%% is 6443).
		// resolve: "ip" is required: that Endpoints has no targetRef, and the
		// Cluster Agent only dispatches pod-backed endpoints checks without it.
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: f.owner.GetNamespace(),
			},
			Data: map[string]string{
				"kube_apiserver_metrics.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kubernetes"
      namespace: "default"
      resolve: "ip"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:%%port%%/metrics"
    bearer_token_auth: true`,

				"kube_controller_manager.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kubernetes"
      namespace: "default"
      resolve: "ip"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:10257/metrics"
    ssl_verify: false
    bearer_token_auth: true`,

				"kube_scheduler.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kubernetes"
      namespace: "default"
      resolve: "ip"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:10259/metrics"
    ssl_verify: false
    bearer_token_auth: true`,

				// etcd has no pod or endpoint on Talos, so ad_identifiers names
				// kube-scheduler only to pin the check to a control-plane node. If
				// that AD name changes, the etcd check stops running silently.
				"etcd.yaml": `ad_identifiers:
  - kube-scheduler
init_config: {}
instances:
  - prometheus_url: "https://%%host%%:2379/metrics"
    tls_ca_cert: "` + talosEtcdCertsMountPath + `/ca.crt"
    tls_cert: "` + talosEtcdCertsMountPath + `/server.crt"
    tls_private_key: "` + talosEtcdCertsMountPath + `/server.key"`,
			},
		}
	default: // Default provider
		configMap = nil
	}
	return configMap, nil
}
