package hostprofiler

const (
	hostProfilerVolumeName     = "host-profiler-config-volume"
	hostProfilerConfigFileName = "host-profiler-config.yaml"
	// DefaultHostProfilerConf default otel agent ConfigMap name
	defaultHostProfilerConf         = "host-profiler-config"
	DDAgentIpcPort                  = "DD_AGENT_IPC_PORT"
	DDAgentIpcConfigRefreshInterval = "DD_AGENT_IPC_CONFIG_REFRESH_INTERVAL"
)
