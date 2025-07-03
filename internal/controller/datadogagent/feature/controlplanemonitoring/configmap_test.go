// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package controlplanemonitoring

import (
	"reflect"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/pkg/kubernetes"
)

func Test_controlPlaneMonitoringFeature_buildControlPlaneMonitoringConfigMap(t *testing.T) {
	owner := &metav1.ObjectMeta{
		Name:      "test",
		Namespace: "foo",
	}

	type fields struct {
		enabled  bool
		owner    metav1.Object
		provider string
	}
	tests := []struct {
		name          string
		fields        fields
		configMapName string
		want          *corev1.ConfigMap
		wantErr       bool
	}{
		{
			name: "default provider",
			fields: fields{
				owner:    owner,
				provider: kubernetes.DefaultProvider,
				enabled:  true,
			},
			configMapName: defaultConfigMapName,
			want:          nil,
		},
		{
			name: "openshift provider",
			fields: fields{
				owner:    owner,
				provider: kubernetes.OpenshiftRHCOSType,
				enabled:  true,
			},
			configMapName: openshiftConfigMapName,
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      openshiftConfigMapName,
					Namespace: "foo",
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
			},
		},
		{
			name: "eks provider",
			fields: fields{
				owner:    owner,
				provider: kubernetes.EKSAMIType,
				enabled:  true,
			},
			configMapName: eksConfigMapName,
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      eksConfigMapName,
					Namespace: "foo",
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
			},
		},
		{
			name: "unknown provider",
			fields: fields{
				owner:    owner,
				provider: "unknown",
				enabled:  true,
			},
			configMapName: otherConfigMapName,
			want: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      otherConfigMapName,
					Namespace: "foo",
				},
				Data: map[string]string{
					"test.yaml": "test",
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &controlPlaneMonitoringFeature{
				owner:    tt.fields.owner,
				enabled:  tt.fields.enabled,
				provider: tt.fields.provider,
			}
			got, err := f.buildControlPlaneMonitoringConfigMap(tt.fields.provider, tt.configMapName)
			if (err != nil) != tt.wantErr {
				t.Errorf("controlPlaneMonitoringFeature.buildControlPlaneMonitoringConfigMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("controlPlaneMonitoringFeature.buildControlPlaneMonitoringConfigMap() = %#v,\nwant %#v", got, tt.want)
			}
		})
	}
}
