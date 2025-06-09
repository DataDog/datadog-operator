// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplaneconfiguration

import (
	"fmt"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (f *controlPlaneConfigurationFeature) buildControlPlaneConfigurationConfigMap() (*corev1.ConfigMap, error) {
	configMap := buildDefaultConfigMap(f.owner.GetNamespace(), f.configMapName, controlPlaneConfigurationConfig(f.provider))
	fmt.Printf("ConfigMap YAML for provider %s:\napiVersion: v1\nkind: ConfigMap\nmetadata:\n  name: %s\n  namespace: %s\ndata:\n  %s: |\n    %s\n",
		f.provider,
		configMap.Name,
		configMap.Namespace,
		defaultControlPlaneConfigurationConfFileName,
		configMap.Data[defaultControlPlaneConfigurationConfFileName])
	return configMap, nil
}

func buildDefaultConfigMap(namespace, cmName string, content string) *corev1.ConfigMap {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      cmName,
			Namespace: namespace,
		},
		Data: map[string]string{
			defaultControlPlaneConfigurationConfFileName: content,
		},
	}
	return configMap
}

func controlPlaneConfigurationConfig(provider string) string {
	// _, providerValue := kubernetes.GetProviderLabelKeyValue(provider)
	fmt.Println("providerValue", provider)
	if provider == kubernetes.DefaultProvider {
		return fmt.Sprintf(`foo: bar`)
	} else if provider == kubernetes.OpenshiftRHCOSType {
		return getOpenShiftConfigMap()
	}
	return "test"
}

func getOpenShiftConfigMap() string {
	return fmt.Sprintf(`kube_apiserver_metrics.yaml: |-
  advanced_ad_identifiers:
    - kube_endpoints:
        name: "kubernetes"
        namespace: "default"
  cluster_check: true
  init_config: {}
  instances:
    - prometheus_url: "https://%%host%%:%%port%%/metrics"
      bearer_token_auth: true

kube_controller_manager.yaml: |-
  advanced_ad_identifiers:
    - kube_endpoints:
        name: "kube-controller-manager"
        namespace: "openshift-kube-controller-manager"
  cluster_check: true
  init_config: {}
  instances:
    - prometheus_url: "https://%%host%%:%%port%%/metrics"
      ssl_verify: false
      bearer_token_auth: true

kube_scheduler.yaml: |-
  advanced_ad_identifiers:
    - kube_endpoints:
        name: "scheduler"
        namespace: "openshift-kube-scheduler"
  cluster_check: true
  init_config: {}
  instances:
    - prometheus_url: "https://%%host%%:%%port%%/metrics"
      ssl_verify: false
      bearer_token_auth: true

etcd.yaml: |-
  advanced_ad_identifiers:
    - kube_endpoints:
        name: "etcd"
        namespace: "openshift-etcd"
  cluster_check: true
  init_config: {}
  instances:
    - prometheus_url: "https://%%host%%:%%port%%/metrics"
      tls_ca_cert: "/etc/etcd-certs/etcd-client-ca.crt"
      tls_cert: "/etc/etcd-certs/etcd-client.crt"
      tls_private_key: "/etc/etcd-certs/etcd-client.key"`)
}
