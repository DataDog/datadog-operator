package metadata

import (
	"encoding/json"
	"os"
	"time"
)

const (
	nodeNameEnvVar = "NODE_NAME"
)

type OperatorMetadataPayload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  OperatorMetadata `json:"datadog_operator_metadata"`
}

type OperatorMetadata struct {
	DatadogAgentEnabled        bool   `json:"datadogagent_enabled"`
	DatadogMonitorEnabled      bool   `json:"datadogmonitor_enabled"`
	DatadogDashboardEnabled    bool   `json:"datadogdashboard_enabled"`
	DatadogSLOEnabled          bool   `json:"datadogslo_enabled"`
	DatadogAgentProfileEnabled bool   `json:"datadogagentprofile_enabled"`
	ExtendedDaemonSetEnabled   bool   `json:"extendeddaemonset_enabled"`
	RemoteConfigEnabled        bool   `json:"remote_config_enabled"`
	IntrospectionEnabled       bool   `json:"introspection_enabled"`
	KubernetesVersion          string `json:"kubernetes_version"`
	OperatorVersion            string `json:"operator_version"`
	ConfigDDURL                string `json:"config_dd_url"`
	ConfigDDSite               string `json:"config_site"`
}

func (mdf *MetadataForwarder) createPayload() []byte {
	now := time.Now().Unix()

	// prioritize custom payload from env var
	if payloadFromEnvVar := os.Getenv("PAYLOAD"); payloadFromEnvVar != "" {
		mdf.logger.V(1).Info("Using custom payload from PAYLOAD env var", "payload", payloadFromEnvVar)
		return []byte(payloadFromEnvVar)
	}

	payload := OperatorMetadataPayload{
		Hostname:  os.Getenv(nodeNameEnvVar),
		Timestamp: now,
		Metadata:  mdf.OperatorMetadata,
	}
	mdf.logger.V(1).Info("Using payload", "payload", payload)

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		mdf.logger.Error(err, "Error marshaling payload to json")
	}

	return jsonPayload
}
