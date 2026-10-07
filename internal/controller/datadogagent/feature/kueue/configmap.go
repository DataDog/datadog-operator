// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kueue

import (
	"fmt"
	"strconv"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-operator/internal/controller/datadogagent/object/configmap"
)

func (f *kueueFeature) buildKueueCheckConfigMap() (*corev1.ConfigMap, error) {
	if f.customConfig != nil && f.customConfig.ConfigMap != nil {
		return nil, nil
	}
	if f.customConfig != nil && f.customConfig.ConfigData != nil {
		return configmap.BuildConfigMapConfigData(f.owner.GetNamespace(), f.customConfig.ConfigData, f.configMapName, kueueConfFileName)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      f.configMapName,
			Namespace: f.owner.GetNamespace(),
		},
		Data: map[string]string{
			kueueConfFileName: kueueCheckConfig(f.serviceName, f.serviceNamespace, f.collectWorkloadEvents),
		},
	}, nil
}

// The Cluster Agent turns the kube_endpoints template into one endpoints check per
// Kueue controller pod and dispatches it to the node Agent running on that pod's node.
// Every replica has to be scraped: in HA setups, counters and histograms are only
// exported by the leader.
// Kueue always serves metrics over HTTPS with a self-signed certificate and authorizes
// the bearer token of the caller against the `/metrics` non-resource URL.
func kueueCheckConfig(serviceName, serviceNamespace string, collectWorkloadEvents bool) string {
	return fmt.Sprintf(`---
advanced_ad_identifiers:
  - kube_endpoints:
      name: %s
      namespace: %s
cluster_check: true
init_config:
instances:
  - openmetrics_endpoint: %s
    tls_verify: false
    auth_token:
      reader:
        type: file
        path: /var/run/secrets/kubernetes.io/serviceaccount/token
      writer:
        type: header
        name: Authorization
        value: Bearer <TOKEN>
        placeholder: <TOKEN>
    collect_workload_events: %s
`, serviceName, serviceNamespace, kueueOpenMetricsEndpoint, strconv.FormatBool(collectWorkloadEvents))
}
