// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplaneconfiguration

import (
	"fmt"
	"strings"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *controlPlaneConfigurationFeature) buildControlPlaneConfigurationConfigMap(provider string, configMapName string) (*corev1.ConfigMap, error) {
	var configMap *corev1.ConfigMap
	if provider == kubernetes.DefaultProvider {
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: f.owner.GetNamespace(),
			},
			Data: map[string]string{
				"foo.yaml": "bar",
			},
		}
	} else if provider == kubernetes.OpenshiftRHCOSType {
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
  - prometheus_url: "https://%host%:%port%/metrics"
    bearer_token_auth: true`,

				"kube_controller_manager.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "kube-controller-manager"
      namespace: "openshift-kube-controller-manager"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%host%:%port%/metrics"
    ssl_verify: false
    bearer_token_auth: true`,

				"kube_scheduler.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "scheduler"
      namespace: "openshift-kube-scheduler"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%host%:%port%/metrics"
    ssl_verify: false
    bearer_token_auth: true`,

				"etcd.yaml": `advanced_ad_identifiers:
  - kube_endpoints:
      name: "etcd"
      namespace: "openshift-etcd"
cluster_check: true
init_config: {}
instances:
  - prometheus_url: "https://%host%:%port%/metrics"
    tls_ca_cert: "/etc/etcd-certs/etcd-client-ca.crt"
    tls_cert: "/etc/etcd-certs/etcd-client.crt"
    tls_private_key: "/etc/etcd-certs/etcd-client.key"`,
			},
		}
	} else {
		configMap = &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      configMapName,
				Namespace: f.owner.GetNamespace(),
			},
			Data: map[string]string{
				"test.yaml": "test",
			},
		}
	}

	fmt.Printf("ConfigMap YAML for provider %s:\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: %s\n  namespace: %s\ndata:\n%s\n",
		provider,
		configMap.Name,
		configMap.Namespace,
		formatConfigMapData(configMap.Data))
	return configMap, nil
}

func formatConfigMapData(data map[string]string) string {
	var result string
	for key, value := range data {
		result += fmt.Sprintf("  %s: |-\n    %s\n", key, strings.ReplaceAll(value, "\n", "\n    "))
	}
	return result
}
