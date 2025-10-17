// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/pkg/constants"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	userAgentHTTPHeaderKey = "User-Agent"

	defaultURLScheme     = "https"
	defaultURLHost       = "app.datadoghq.com"
	defaultURLHostPrefix = "app."
	defaultURLPath       = "api/v1/metadata"

	defaultInterval = 1 * time.Minute
)

type OperatorMetadataForwarder struct {
	*SharedMetadata

	// Operator-specific fields
	client           *http.Client
	requestURL       string
	hostName         string
	payloadHeader    http.Header
	OperatorMetadata OperatorMetadata
}

type OperatorMetadataPayload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	Metadata  OperatorMetadata `json:"datadog_operator_metadata"`
}

type OperatorMetadata struct {
	OperatorVersion               string `json:"operator_version"`
	KubernetesVersion             string `json:"kubernetes_version"`
	InstallMethodTool             string `json:"install_method_tool"`
	InstallMethodToolVersion      string `json:"install_method_tool_version"`
	IsLeader                      bool   `json:"is_leader"`
	DatadogAgentEnabled           bool   `json:"datadogagent_enabled"`
	DatadogMonitorEnabled         bool   `json:"datadogmonitor_enabled"`
	DatadogDashboardEnabled       bool   `json:"datadogdashboard_enabled"`
	DatadogSLOEnabled             bool   `json:"datadogslo_enabled"`
	DatadogGenericResourceEnabled bool   `json:"datadoggenericresource_enabled"`
	DatadogAgentProfileEnabled    bool   `json:"datadogagentprofile_enabled"`
	LeaderElectionEnabled         bool   `json:"leader_election_enabled"`
	ExtendedDaemonSetEnabled      bool   `json:"extendeddaemonset_enabled"`
	RemoteConfigEnabled           bool   `json:"remote_config_enabled"`
	IntrospectionEnabled          bool   `json:"introspection_enabled"`
	ClusterName                   string `json:"cluster_name"`
	ConfigDDURL                   string `json:"config_dd_url"`
	ConfigDDSite                  string `json:"config_site"`
}

// NewOperatorMetadataForwarder creates a new instance of the operator metadata forwarder
func NewOperatorMetadataForwarder(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string) *OperatorMetadataForwarder {
	return &OperatorMetadataForwarder{
		SharedMetadata: NewSharedMetadata(logger, k8sClient, kubernetesVersion, operatorVersion),
		hostName:       os.Getenv(constants.DDHostName),
		requestURL:     getURL(),
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		OperatorMetadata: OperatorMetadata{},
	}
}

// Start starts the operator metadata forwarder
func (omf *OperatorMetadataForwarder) Start() {
	err := omf.SetCredentials()
	if err != nil {
		omf.logger.Error(err, "Could not set credentials; not starting metadata forwarder")
		return
	}

	if omf.hostName == "" {
		omf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting metadata forwarder")
		return
	}

	omf.payloadHeader = omf.getHeaders()

	omf.logger.Info("Starting operator metadata forwarder")

	ticker := time.NewTicker(defaultInterval)
	go func() {
		for range ticker.C {
			if err := omf.sendMetadata(); err != nil {
				omf.logger.Error(err, "Error while sending metadata")
			}
		}
	}()
}

func (omf *OperatorMetadataForwarder) sendMetadata() error {
	payload := omf.GetPayload()

	omf.logger.Info("Metadata payload", "payload", string(payload))

	omf.logger.V(1).Info("Sending metadata to URL", "url", omf.requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", omf.requestURL, reader)
	if err != nil {
		omf.logger.Error(err, "Error creating request", "url", omf.requestURL, "reader", reader)
		return err
	}
	req.Header = omf.payloadHeader

	resp, err := omf.client.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response body: %w", err)
	}

	omf.logger.V(1).Info("Read response", "status code", resp.StatusCode, "body", string(body))
	return nil
}

func (omf *OperatorMetadataForwarder) GetPayload() []byte {
	now := time.Now().Unix()

	omf.OperatorMetadata.ClusterName = omf.clusterName
	omf.OperatorMetadata.OperatorVersion = omf.operatorVersion
	omf.OperatorMetadata.KubernetesVersion = omf.kubernetesVersion

	payload := OperatorMetadataPayload{
		Hostname:  omf.hostName,
		Timestamp: now,
		Metadata:  omf.OperatorMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		omf.logger.Error(err, "Error marshaling payload to json")
	}

	return jsonPayload
}

func (omf *OperatorMetadataForwarder) getHeaders() http.Header {
	headers := omf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}

func getURL() string {
	mdfURL := url.URL{
		Scheme: defaultURLScheme,
		Host:   defaultURLHost,
		Path:   defaultURLPath,
	}

	// check site env var
	// example: datadoghq.com
	if siteFromEnvVar := os.Getenv("DD_SITE"); siteFromEnvVar != "" {
		mdfURL.Host = defaultURLHostPrefix + siteFromEnvVar
	}
	// check url env var
	// example: https://app.datadoghq.com
	if urlFromEnvVar := os.Getenv("DD_URL"); urlFromEnvVar != "" {
		tempURL, err := url.Parse(urlFromEnvVar)
		if err == nil {
			mdfURL.Host = tempURL.Host
			mdfURL.Scheme = tempURL.Scheme
		}
	}

	return mdfURL.String()
}
