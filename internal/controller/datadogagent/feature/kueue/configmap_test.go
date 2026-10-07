// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kueue

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func Test_kueueCheckConfig(t *testing.T) {
	tests := []struct {
		name                  string
		serviceName           string
		serviceNamespace      string
		collectWorkloadEvents bool
		want                  string
	}{
		{
			name:                  "defaults",
			serviceName:           defaultMetricsServiceName,
			serviceNamespace:      defaultMetricsServiceNamespace,
			collectWorkloadEvents: true,
			want: `---
advanced_ad_identifiers:
  - kube_endpoints:
      name: kueue-controller-manager-metrics-service
      namespace: kueue-system
cluster_check: true
init_config:
instances:
  - openmetrics_endpoint: https://%%host%%:%%port%%/metrics
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
    collect_workload_events: true
`,
		},
		{
			name:                  "custom service and workload events disabled",
			serviceName:           "my-kueue-metrics",
			serviceNamespace:      "my-kueue",
			collectWorkloadEvents: false,
			want: `---
advanced_ad_identifiers:
  - kube_endpoints:
      name: my-kueue-metrics
      namespace: my-kueue
cluster_check: true
init_config:
instances:
  - openmetrics_endpoint: https://%%host%%:%%port%%/metrics
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
    collect_workload_events: false
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, kueueCheckConfig(tt.serviceName, tt.serviceNamespace, tt.collectWorkloadEvents))
		})
	}
}
