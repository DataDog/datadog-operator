// package defaultconfig exposes the otel-agent default config
package defaultconfig

var DefaultOtelCollectorConfig = `
receivers:
  otlp:
    protocols:
      grpc:
          endpoint: 0.0.0.0:4317
      http:
          endpoint: 0.0.0.0:4318
exporters:
  debug:
    verbosity: detailed
  datadog:
    api:
      key: ${env:DD_API_KEY}
processors:
  batch:
    timeout: 10s
service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [batch]
      exporters: [datadog]
    metrics:
      receivers: [otlp]
      processors: [batch]
      exporters: [datadog]
    logs:
      receivers: [otlp]
      processors: [batch]
      exporters: [datadog]`
