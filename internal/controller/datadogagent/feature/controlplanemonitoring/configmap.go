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
	default: // Default provider
		configMap = nil
	}
	return configMap, nil
}
