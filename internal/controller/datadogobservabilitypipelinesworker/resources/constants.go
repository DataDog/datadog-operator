// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

const (
	workerContainerName = "worker"
	dataVolumeName      = "data"
	dataDirectory       = "/var/lib/observability-pipelines-worker"

	workerAPIPort int32 = 8686
)

// Names fit Kubernetes' 15-character limit for container ports.
var sourceAddressEnvNames = map[string]string{
	"otlp-grpc":      "DD_OP_SOURCE_OTEL_GRPC_ADDRESS",
	"otlp-http":      "DD_OP_SOURCE_OTEL_HTTP_ADDRESS",
	"datadog-agent":  "DD_OP_SOURCE_DATADOG_AGENT_ADDRESS",
	"splunk-tcp":     "DD_OP_SOURCE_SPLUNK_TCP_ADDRESS",
	"splunk-hec":     "DD_OP_SOURCE_SPLUNK_HEC_ADDRESS",
	"http-server":    "DD_OP_SOURCE_HTTP_SERVER_ADDRESS",
	"fluent":         "DD_OP_SOURCE_FLUENT_ADDRESS",
	"logstash":       "DD_OP_SOURCE_LOGSTASH_ADDRESS",
	"syslog":         "DD_OP_SOURCE_SYSLOG_ADDRESS",
	"socket":         "DD_OP_SOURCE_SOCKET_ADDRESS",
	"aws-firehose":   "DD_OP_SOURCE_AWS_DATA_FIREHOSE_ADDRESS",
	"sumo-logic":     "DD_OP_SOURCE_SUMO_LOGIC_ADDRESS",
	"prom-pushgw":    "DD_OP_SOURCE_PROMETHEUS_PUSHGATEWAY_ADDRESS",
	"prom-rem-write": "DD_OP_SOURCE_PROMETHEUS_REMOTE_WRITE_ADDRESS",
}
