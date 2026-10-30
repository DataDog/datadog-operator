// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

// This file collects the names, ports, probe settings and environment variables used to render
// Observability Pipelines Worker workloads. Replica, resource and storage defaults live in the
// defaults package.

const (
	appName       = "observability-pipelines-worker"
	listenAddress = "0.0.0.0"
)

// The Worker workload defines these names; the validation package rejects user settings that collide with them.
const (
	ContainerName        = "worker"
	APIPortName          = "api"
	APIPort        int32 = 8686
	DataVolumeName       = "data"
	DataDirectory        = "/var/lib/observability-pipelines-worker"
)

const (
	probeInitialDelaySeconds       int32 = 15
	probeTimeoutSeconds            int32 = 15
	probePeriodSeconds             int32 = 10
	probeSuccessThreshold          int32 = 1
	livenessProbeFailureThreshold  int32 = 5
	readinessProbeFailureThreshold int32 = 3
)

// The graceful shutdown limit leaves a margin before the termination grace period ends.
const (
	minGracefulShutdownLimitSeconds int64 = 10
	gracefulShutdownMarginSeconds   int64 = 10
)

const (
	envDatadogAPIKey                = "DD_API_KEY"
	envDatadogSite                  = "DD_SITE"
	envPipelineID                   = "DD_OP_PIPELINE_ID"
	envDataDirectory                = "DD_OP_DATA_DIR"
	envAPIEnabled                   = "DD_OP_API_ENABLED"
	envAPIAddress                   = "DD_OP_API_ADDRESS"
	envGracefulShutdownLimitSeconds = "DD_OP_GRACEFUL_SHUTDOWN_LIMIT_SECS"
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
