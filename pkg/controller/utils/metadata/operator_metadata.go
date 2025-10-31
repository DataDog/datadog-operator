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
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/version"
)

const (
	userAgentHTTPHeaderKey = "User-Agent"
	defaultInterval        = 1 * time.Minute
)

type OperatorMetadataForwarder struct {
	*SharedMetadata

	// Operator-specific fields
	payloadHeader    http.Header
	OperatorMetadata OperatorMetadata
}

type OperatorMetadataPayload struct {
	Hostname    string           `json:"hostname"`
	Timestamp   int64            `json:"timestamp"`
	ClusterID   string           `json:"cluster_id"`
	ClusterName string           `json:"clustername"`
	Metadata    OperatorMetadata `json:"datadog_operator_metadata"`
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
	DatadogAgentInternalEnabled   bool   `json:"datadogagentinternal_enabled"`
	LeaderElectionEnabled         bool   `json:"leader_election_enabled"`
	ExtendedDaemonSetEnabled      bool   `json:"extendeddaemonset_enabled"`
	RemoteConfigEnabled           bool   `json:"remote_config_enabled"`
	IntrospectionEnabled          bool   `json:"introspection_enabled"`
	ClusterID                     string `json:"cluster_id"`
	ClusterName                   string `json:"cluster_name"`
	ConfigDDURL                   string `json:"config_dd_url"`
	ConfigDDSite                  string `json:"config_site"`
	ResourceCounts                string `json:"resource_count"`
}

// NewOperatorMetadataForwarder creates a new instance of the operator metadata forwarder
func NewOperatorMetadataForwarder(logger logr.Logger, k8sClient client.Reader, kubernetesVersion string, operatorVersion string) *OperatorMetadataForwarder {
	return &OperatorMetadataForwarder{
		SharedMetadata:   NewSharedMetadata(logger, k8sClient, kubernetesVersion, operatorVersion),
		OperatorMetadata: OperatorMetadata{},
	}
}

// Start starts the operator metadata forwarder
func (omf *OperatorMetadataForwarder) Start() {
	err := omf.setCredentials()
	if err != nil {
		omf.logger.Error(err, "Could not set credentials; not starting operator metadata forwarder")
		return
	}

	if omf.hostName == "" {
		omf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting operator metadata forwarder")
		return
	}

	omf.payloadHeader = omf.getHeaders()

	omf.logger.Info("Starting operator metadata forwarder")

	ticker := time.NewTicker(defaultInterval)
	go func() {
		for range ticker.C {
			if err := omf.sendMetadata(); err != nil {
				omf.logger.Error(err, "Error while sending operator metadata")
			}
		}
	}()
}

func (omf *OperatorMetadataForwarder) sendMetadata() error {
	clusterUID, err := omf.GetOrCreateClusterUID()
	if err != nil {
		omf.logger.Error(err, "Failed to get cluster UID")
		return err
	}
	payload := omf.GetPayload(clusterUID)

	omf.logger.Info("Operator metadata payload", "payload", string(payload))

	omf.logger.V(1).Info("Sending operator metadata to URL", "url", omf.requestURL)

	reader := bytes.NewReader(payload)
	req, err := http.NewRequestWithContext(context.TODO(), "POST", omf.requestURL, reader)
	if err != nil {
		omf.logger.Error(err, "Error creating request", "url", omf.requestURL, "reader", reader)
		return err
	}
	req.Header = omf.payloadHeader

	resp, err := omf.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending operator metadata request: %w", err)
	}

	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read operator metadata response body: %w", err)
	}

	omf.logger.V(1).Info("Read operator metadata response", "status code", resp.StatusCode, "body", string(body))
	return nil
}

func (omf *OperatorMetadataForwarder) GetPayload(clusterUID string) []byte {
	now := time.Now().Unix()

	omf.OperatorMetadata.ClusterID = clusterUID
	omf.OperatorMetadata.ClusterName = omf.clusterName
	omf.OperatorMetadata.OperatorVersion = omf.operatorVersion
	omf.OperatorMetadata.KubernetesVersion = omf.kubernetesVersion
	omf.OperatorMetadata.ResourceCounts = omf.getResourceCounts()

	payload := OperatorMetadataPayload{
		Hostname:    omf.hostName,
		Timestamp:   now,
		ClusterID:   clusterUID,
		ClusterName: omf.clusterName,
		Metadata:    omf.OperatorMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		omf.logger.Error(err, "Error marshaling payload to json")
	}

	return jsonPayload
}

// setupFromOperator delegates to SharedMetadata setupFromOperator method
func (omf *OperatorMetadataForwarder) setupFromOperator() error {
	return omf.SharedMetadata.setupFromOperator()
}

// setupFromDDA delegates to SharedMetadata setupFromDDA method
func (omf *OperatorMetadataForwarder) setupFromDDA(dda *v2alpha1.DatadogAgent) error {
	return omf.SharedMetadata.setupFromDDA(dda)
}

// setCredentials attempts to set up credentials and cluster name from the operator configuration first.
// If cluster name is empty (even when credentials are successfully retrieved from operator),
// it falls back to setting up from DatadogAgent to ensure we have a valid cluster name.
func (omf *OperatorMetadataForwarder) setCredentials() error {
	err := omf.setupFromOperator()
	if err == nil && omf.clusterName != "" {
		return nil
	}

	dda, err := omf.SharedMetadata.getDatadogAgent()
	if err != nil {
		return err
	}

	return omf.setupFromDDA(dda)
}

func (omf *OperatorMetadataForwarder) getHeaders() http.Header {
	headers := omf.GetBaseHeaders()
	headers.Set(userAgentHTTPHeaderKey, fmt.Sprintf("Datadog Operator/%s", version.GetVersion()))
	return headers
}

// getResourceCounts counts all Datadog custom resources in the cluster and returns as JSON string
func (omf *OperatorMetadataForwarder) getResourceCounts() string {
	counts := make(map[string]int)

	// If k8sClient is nil (e.g., in tests), return empty JSON
	if omf.k8sClient == nil {
		return "{}"
	}

	ddaList := &v2alpha1.DatadogAgentList{}
	if err := omf.k8sClient.List(context.TODO(), ddaList); err == nil {
		counts["datadogagent"] = len(ddaList.Items)
	}

	ddaiList := &v1alpha1.DatadogAgentInternalList{}
	if err := omf.k8sClient.List(context.TODO(), ddaiList); err == nil {
		counts["datadogagentinternal"] = len(ddaiList.Items)
	}

	monitorList := &v1alpha1.DatadogMonitorList{}
	if err := omf.k8sClient.List(context.TODO(), monitorList); err == nil {
		counts["datadogmonitor"] = len(monitorList.Items)
	}

	dashboardList := &v1alpha1.DatadogDashboardList{}
	if err := omf.k8sClient.List(context.TODO(), dashboardList); err == nil {
		counts["datadogdashboard"] = len(dashboardList.Items)
	}

	sloList := &v1alpha1.DatadogSLOList{}
	if err := omf.k8sClient.List(context.TODO(), sloList); err == nil {
		counts["datadogslo"] = len(sloList.Items)
	}

	genericList := &v1alpha1.DatadogGenericResourceList{}
	if err := omf.k8sClient.List(context.TODO(), genericList); err == nil {
		counts["datadoggenericresource"] = len(genericList.Items)
	}

	profileList := &v1alpha1.DatadogAgentProfileList{}
	if err := omf.k8sClient.List(context.TODO(), profileList); err == nil {
		counts["datadogagentprofile"] = len(profileList.Items)
	}

	// Serialize to JSON string
	countsJSON, err := json.Marshal(counts)
	if err != nil {
		omf.logger.Error(err, "Error marshaling resource counts to JSON")
		return "{}"
	}

	return string(countsJSON)
}
