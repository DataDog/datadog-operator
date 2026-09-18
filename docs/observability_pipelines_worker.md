# Observability Pipelines Worker ports

Configure worker ports using `spec.ports` on a `DatadogObservabilityPipelinesWorker`,
or `spec.components.pipeline.ports` on a `DatadogBYOCCluster`. At least one port
must be specified, and each entry requires an explicit port number.

The OP Controller exposes these ports on the worker container and Service. For
the names listed below, it also generates the source's address environment
variable with the value `0.0.0.0:<port>`.

| Port name | Address environment variable |
| --- | --- |
| `otlp-grpc` | `DD_OP_SOURCE_OTEL_GRPC_ADDRESS` |
| `otlp-http` | `DD_OP_SOURCE_OTEL_HTTP_ADDRESS` |
| `datadog-agent` | `DD_OP_SOURCE_DATADOG_AGENT_ADDRESS` |
| `splunk-tcp` | `DD_OP_SOURCE_SPLUNK_TCP_ADDRESS` |
| `splunk-hec` | `DD_OP_SOURCE_SPLUNK_HEC_ADDRESS` |
| `http-server` | `DD_OP_SOURCE_HTTP_SERVER_ADDRESS` |
| `fluent` | `DD_OP_SOURCE_FLUENT_ADDRESS` |
| `logstash` | `DD_OP_SOURCE_LOGSTASH_ADDRESS` |
| `syslog` | `DD_OP_SOURCE_SYSLOG_ADDRESS` |
| `socket` | `DD_OP_SOURCE_SOCKET_ADDRESS` |
| `aws-firehose` | `DD_OP_SOURCE_AWS_DATA_FIREHOSE_ADDRESS` |
| `sumo-logic` | `DD_OP_SOURCE_SUMO_LOGIC_ADDRESS` |
| `prom-pushgw` | `DD_OP_SOURCE_PROMETHEUS_PUSHGATEWAY_ADDRESS` |
| `prom-rem-write` | `DD_OP_SOURCE_PROMETHEUS_REMOTE_WRITE_ADDRESS` |

The abbreviated names fit Kubernetes' 15-character limit for container port
names. Other names expose a port without generating an environment variable;
configure any custom source address keys through `env` or `envFrom`.

For example, the following BYOC configuration exposes the OTLP source on custom
ports and generates both address variables:

```yaml
spec:
  components:
    pipeline:
      pipelineID: 11111111-2222-3333-4444-555555555555
      ports:
        - name: otlp-grpc
          port: 14317
        - name: otlp-http
          port: 14318
```

Port declarations do not create sources in the remote pipeline configuration.
Configure the corresponding source in the pipeline identified by `pipelineID`.
The OP OpenTelemetry source requires both its gRPC and HTTP addresses; declare
both ports explicitly. Neither OTLP port implicitly adds the other, and `otlp`
is not a recognized name for environment generation.

The `protocol` field controls the container and Service protocol; it defaults to
TCP. For UDP-capable sources such as `syslog` or `socket`, set `protocol: UDP`
and configure the same transport in the remote pipeline.

## Override generated addresses

The controller merges generated addresses with explicit `env` entries. Explicit
values, including `valueFrom` entries and empty values, override generated
addresses. No additional ConfigMap is created.

Kubernetes gives `env` precedence over `envFrom`. A generated address therefore
takes precedence over the same variable supplied through `envFrom`. To override
it with a ConfigMap or Secret value, use an explicit `env` entry with `valueFrom`.

For a BYOC cluster, configure overrides through `spec.components.pipeline.env`;
the corresponding field is `spec.env` on a standalone worker. Keep any
overridden listening port consistent with the
declared port. For example, use `[::]:14317` to change the binding address for
an `otlp-grpc` port declared as `14317`.

Changing ports updates the StatefulSet's environment and container ports,
triggering the normal StatefulSet rollout.

See the [standalone worker sample](../config/samples/datadoghq_v1alpha1_datadogobservabilitypipelinesworker.yaml)
and [BYOC sample](../config/samples/datadoghq_v1alpha1_datadogbyoccluster.yaml).
