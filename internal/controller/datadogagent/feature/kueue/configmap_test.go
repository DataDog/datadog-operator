// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package kueue

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
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
      name: "kueue-controller-manager-metrics-service"
      namespace: "kueue-system"
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
      name: "my-kueue-metrics"
      namespace: "my-kueue"
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

// Test_kueueCheckConfigServiceIdentifiers checks that valid Kubernetes names that YAML
// would otherwise resolve to non-string scalars are read back unchanged.
func Test_kueueCheckConfigServiceIdentifiers(t *testing.T) {
	for _, name := range []string{"null", "true", "123"} {
		t.Run(name, func(t *testing.T) {
			var conf struct {
				AdvancedADIdentifiers []struct {
					KubeEndpoints struct {
						Name      string `yaml:"name"`
						Namespace string `yaml:"namespace"`
					} `yaml:"kube_endpoints"`
				} `yaml:"advanced_ad_identifiers"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(kueueCheckConfig(name, name, true)), &conf))
			require.Len(t, conf.AdvancedADIdentifiers, 1)
			assert.Equal(t, name, conf.AdvancedADIdentifiers[0].KubeEndpoints.Name)
			assert.Equal(t, name, conf.AdvancedADIdentifiers[0].KubeEndpoints.Namespace)
		})
	}
}
