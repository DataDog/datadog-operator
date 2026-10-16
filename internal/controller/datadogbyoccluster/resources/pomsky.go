// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resources

// This file collects the names, environment variables and node configuration defaults used to render
// Pomsky workloads. Most of them mirror the CloudPrem Helm chart; keep them aligned when either changes.
// Values sized from the cluster spec are computed in config_map.go, and replica and resource defaults
// live in the defaults package.

const (
	IndexerComponentName           = "indexer"
	SearcherComponentName          = "searcher"
	MetastoreComponentName         = "metastore"
	ControlPlaneComponentName      = "control-plane"
	JanitorComponentName           = "janitor"
	ReadOnlyMetastoreComponentName = "read-only-metastore"
	CompactorComponentName         = "compactor"
	PipelineComponentName          = "pipeline"
)

const (
	appName                  = "cloudprem"
	configChecksumAnnotation = "checksum/config"
	configVolumeName         = "config"
	dataVolumeName           = "data"
	defaultClusterDomain     = "cluster.local"
	bytesPerGiB              = 1024 * 1024 * 1024
)

const (
	quickwitIndexerServiceName           = "indexer"
	quickwitSearcherServiceName          = "searcher"
	quickwitMetastoreServiceName         = "metastore"
	quickwitControlPlaneServiceName      = "control_plane"
	quickwitJanitorServiceName           = "janitor"
	quickwitReadOnlyMetastoreServiceName = "metastore_read_replica"
	quickwitCompactorServiceName         = "compactor"
)

const (
	quickwitDirectory  = "/quickwit/"
	nodeConfigFileName = "node.yaml"
	nodeConfigPath     = quickwitDirectory + nodeConfigFileName
	defaultDataPath    = quickwitDirectory + "qwdata"
)

const (
	restPort      int32 = 7280
	grpcPort      int32 = 7281
	gossipPort    int32 = 7282
	cloudpremPort int32 = 7283
	healthPort    int32 = 7284
)

const (
	pomskyUserID                 int64 = 1005
	startupProbePath                   = "/health/readyz"
	startupProbeFailureThreshold int32 = 12
	startupProbePeriodSeconds    int32 = 5
	livenessProbePath                  = "/health/livez"
	livenessProbeTimeoutSeconds  int32 = 5
)

const (
	envKubernetesNamespace      = "KUBERNETES_NAMESPACE"
	envKubernetesComponent      = "KUBERNETES_COMPONENT"
	envKubernetesPodName        = "KUBERNETES_POD_NAME"
	envKubernetesNodeName       = "KUBERNETES_NODE_NAME"
	envKubernetesPodIP          = "KUBERNETES_POD_IP"
	envKubernetesLimitsCPU      = "KUBERNETES_LIMITS_CPU"
	envKubernetesLimitsMemory   = "KUBERNETES_LIMITS_MEMORY"
	envKubernetesRequestsCPU    = "KUBERNETES_REQUESTS_CPU"
	envKubernetesRequestsMemory = "KUBERNETES_REQUESTS_MEMORY"

	envQuickwitNumCPUs                      = "QW_NUM_CPUS"
	envQuickwitConfig                       = "QW_CONFIG"
	envQuickwitClusterID                    = "QW_CLUSTER_ID"
	envQuickwitNodeID                       = "QW_NODE_ID"
	envQuickwitAvailabilityZone             = "QW_AVAILABILITY_ZONE"
	envQuickwitPeerSeeds                    = "QW_PEER_SEEDS"
	envQuickwitAdvertiseAddress             = "QW_ADVERTISE_ADDRESS"
	envQuickwitClusterEndpoint              = "QW_CLUSTER_ENDPOINT"
	envQuickwitMetastoreURI                 = "QW_METASTORE_URI"
	envQuickwitReadOnlyMetastoreURI         = "QW_METASTORE_READ_REPLICA_URI"
	envQuickwitIngestDecommissionTimeout    = "QW_INGEST_DECOMMISSION_TIMEOUT"
	envQuickwitCompactorDecommissionTimeout = "QW_COMPACTOR_DECOMMISSION_TIMEOUT"
	envQuickwitStandaloneCompactors         = "QW_ENABLE_STANDALONE_COMPACTORS"
	envQuickwitDisableIngestV1              = "QW_DISABLE_INGEST_V1"
	envQuickwitDisableTelemetry             = "QW_DISABLE_TELEMETRY"
	envQuickwitLogFormat                    = "QW_LOG_FORMAT"
	envQuickwitRandomSplitPrefix            = "QW_RANDOM_SPLIT_PREFIX"
	envQuickwitOpenTelemetryExporter        = "QW_ENABLE_OPENTELEMETRY_OTLP_EXPORTER"

	envCloudPremDogstatsdHost     = "CP_DOGSTATSD_SERVER_HOST"
	envCloudPremDogstatsdPort     = "CP_DOGSTATSD_SERVER_PORT"
	envCloudPremReverseConnection = "CP_ENABLE_REVERSE_CONNECTION"
	envCloudPremMinShards         = "CP_MIN_SHARDS"

	envDatadogSite   = "DD_SITE"
	envDatadogAPIKey = "DD_API_KEY"
	envAWSRegion     = "AWS_REGION"
	envNoColor       = "NO_COLOR"

	envBYOCTelemetryEnabled           = "BYOC_TELEMETRY_ENABLED"
	envOTelResourceAttributes         = "OTEL_RESOURCE_ATTRIBUTES"
	envOTelExporterProtocol           = "OTEL_EXPORTER_OTLP_PROTOCOL"
	envOTelExporterLogsEndpoint       = "OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"
	envOTelExporterMetricsTemporality = "OTEL_EXPORTER_OTLP_METRICS_TEMPORALITY_PREFERENCE"
	envOTelExporterMetricsEndpoint    = "OTEL_EXPORTER_OTLP_METRICS_ENDPOINT"
	envOTelExporterTracesEndpoint     = "OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"
	envOTelTracesSampler              = "OTEL_TRACES_SAMPLER"
	envOTelTracesSamplerArg           = "OTEL_TRACES_SAMPLER_ARG"
	envImageName                      = "IMAGE_NAME"
	envImageTag                       = "IMAGE_TAG"
)

const (
	cloudPremMinShards          = "12"
	quickwitLogFormat           = "DDG"
	telemetryIntakePath         = "/api/unstable/byoc-telemetry-intake/v1/"
	telemetryExporterProtocol   = "http/protobuf"
	telemetryMetricsTemporality = "delta"
	telemetryTracesSampler      = "parentbased_traceidratio"
	telemetryTracesSamplerRatio = "0.2"
)

const quickwitUseReadOnlyMetastoreConfigKey = "use_metastore_read_replica"

// defaultNodeConfig returns the node configuration defaults that do not depend on the cluster spec.
func defaultNodeConfig() map[string]any {
	return map[string]any{
		"version":               0.8,
		"listen_address":        "0.0.0.0",
		"gossip_listen_port":    gossipPort,
		"cloudprem_listen_port": cloudpremPort,
		"data_dir":              defaultDataPath,
		"grpc":                  map[string]any{"keep_alive": map[string]any{"interval": "30s", "timeout": "10s"}},
		"health":                map[string]any{"listen_port": healthPort},
		"cloudprem": map[string]any{
			"mtls_header":              "X-Amzn-Mtls-Clientcert",
			"create_dd_logs_index":     true,
			"create_dd_metrics_index":  false,
			"create_dd_sketches_index": false,
			"create_dd_traces_index":   false,
		},
		"docs_clustering": []any{
			map[string]any{"fingerprint": []any{map[string]any{"kind": "structure"}}},
			map[string]any{"fingerprint": []any{map[string]any{"kind": "raw", "path": "source"}}},
			map[string]any{"fingerprint": []any{map[string]any{"kind": "raw", "path": "status"}}},
			map[string]any{"fingerprint": []any{map[string]any{"kind": "tokenized", "path": "message"}}},
		},
		quickwitIndexerServiceName:  map[string]any{"split_store_max_num_splits": 10000},
		quickwitSearcherServiceName: map[string]any{"aggregation_memory_limit": "500M"},
	}
}
