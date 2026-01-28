// package defaultconfig exposes the otel-agent default config
package defaultconfig

import "fmt"

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
