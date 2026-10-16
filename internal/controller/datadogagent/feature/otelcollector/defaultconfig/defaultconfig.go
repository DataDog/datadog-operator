// package defaultconfig exposes the otel-agent default config
package defaultconfig

import (
	"fmt"
	"strings"
)

var DefaultOtelCollectorConfig = `
receivers:
  prometheus:
    config:
      scrape_configs:
        - job_name: "otelcol"
          scrape_interval: 60s
          static_configs:
            - targets: ["0.0.0.0:8888"]
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318
exporters:
  datadog:
    api:
      key: ${env:DD_API_KEY}
      site: ${env:DD_SITE}
    sending_queue:
      batch:
       flush_timeout: 10s
processors:
  infraattributes:
    cardinality: 2
  filter/drop-prometheus-internal-metrics:
    metrics:
      exclude:
        match_type: regexp
        metric_names:
          - ^scrape_.*$
          - ^up$
          - ^promhttp_metric_handler_errors_total$
connectors:
  datadog/connector:
    traces:
      compute_top_level_by_span_kind: true
      peer_tags_aggregation: true
      compute_stats_by_span_kind: true
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [infraattributes]
      exporters: [datadog, datadog/connector]
    metrics:
      receivers: [otlp, datadog/connector]
      processors: [infraattributes]
      exporters: [datadog]
    metrics/prometheus:
      receivers: [prometheus]
      processors: [filter/drop-prometheus-internal-metrics, infraattributes]
      exporters: [datadog]
    logs:
      receivers: [otlp]
      processors: [infraattributes]
      exporters: [datadog]`

var DefaultOtelCollectorConfigInGateway = func(ddaName string) string {
	return fmt.Sprintf(`
receivers:
  otlp:
    protocols:
      grpc:
          endpoint: 0.0.0.0:4317
      http:
          endpoint: 0.0.0.0:4318
exporters:
  otlphttp:
    endpoint: http://%s-otel-agent-gateway:4318
    tls:
      insecure: true
    sending_queue:
      batch:
       flush_timeout: 10s
processors:
  infraattributes:
    cardinality: 2
connectors:
  datadog/connector:
    traces:
      compute_top_level_by_span_kind: true
      peer_tags_aggregation: true
      compute_stats_by_span_kind: true
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [infraattributes]
      exporters: [otlphttp, datadog/connector]
    metrics:
      receivers: [otlp, datadog/connector]
      processors: [infraattributes]
      exporters: [otlphttp]
    logs:
      receivers: [otlp]
      processors: [infraattributes]
      exporters: [otlphttp]`, ddaName)
}

const infraAttributesProcessor = `
  infraattributes:
    cardinality: 2`

// k8sAttributesProcessor tags telemetry from the Kubernetes API, for collectors without the
// core Agent tagger. It only watches pods of the node the collector runs on.
const k8sAttributesProcessor = `
  k8s_attributes:
    auth_type: serviceAccount
    passthrough: false
    filter:
      node_from_env_var: K8S_NODE_NAME
    extract:
      metadata:
        - k8s.namespace.name
        - k8s.pod.name
        - k8s.pod.uid
        - k8s.node.name
        - k8s.deployment.name
        - k8s.replicaset.name
        - k8s.statefulset.name
        - k8s.daemonset.name
        - k8s.cronjob.name
        - k8s.job.name
        - k8s.container.name
        - container.id
        - container.image.name
        - container.image.tag
    pod_association:
      - sources:
          - from: resource_attribute
            name: k8s.pod.uid
      - sources:
          - from: resource_attribute
            name: k8s.pod.ip
      - sources:
          - from: connection`

// WithoutCoreAgent adapts a default config to run without the core Agent: the infraattributes
// processor relies on the core Agent tagger, so it is replaced by the k8s_attributes processor.
func WithoutCoreAgent(config string) string {
	config = strings.Replace(config, infraAttributesProcessor, k8sAttributesProcessor, 1)
	return strings.ReplaceAll(config, "infraattributes]", "k8s_attributes]")
}
