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

const (
	// OTLPHTTPExporterName is the current type of the OTel Collector OTLP/HTTP exporter,
	// introduced in OTel Collector v0.144.0 (first bundled in Agent 7.76.0).
	OTLPHTTPExporterName = "otlp_http"
	// DeprecatedOTLPHTTPExporterName is the deprecated alias of OTLPHTTPExporterName,
	// required by Agents bundling an OTel Collector older than v0.144.0.
	DeprecatedOTLPHTTPExporterName = "otlphttp"
)

// DefaultOtelCollectorConfigInGateway returns the default otel-agent config used when the
// OTel Agent Gateway is enabled. otlpHTTPExporter is the type of the OTLP/HTTP exporter
// that forwards telemetry to the gateway (OTLPHTTPExporterName or DeprecatedOTLPHTTPExporterName).
var DefaultOtelCollectorConfigInGateway = func(ddaName, otlpHTTPExporter string) string {
	return fmt.Sprintf(`
receivers:
  otlp:
    protocols:
      grpc:
          endpoint: 0.0.0.0:4317
      http:
          endpoint: 0.0.0.0:4318
exporters:
  %[2]s:
    endpoint: http://%[1]s-otel-agent-gateway:4318
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
      exporters: [%[2]s, datadog/connector]
    metrics:
      receivers: [otlp, datadog/connector]
      processors: [infraattributes]
      exporters: [%[2]s]
    logs:
      receivers: [otlp]
      processors: [infraattributes]
      exporters: [%[2]s]`, ddaName, otlpHTTPExporter)
}
