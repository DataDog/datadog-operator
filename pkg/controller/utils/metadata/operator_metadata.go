// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package metadata

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-operator/api/datadoghq/v1alpha1"
	"github.com/DataDog/datadog-operator/api/datadoghq/v2alpha1"
	"github.com/DataDog/datadog-operator/pkg/config"
)

const (
	userAgentHTTPHeaderKey = "User-Agent"
	defaultInterval        = 1 * time.Minute
	resourceCountsTTL      = 5 * time.Minute // Refresh resource counts every 5 minutes
)

type OperatorMetadataForwarder struct {
	*SharedMetadata

	mutex            sync.RWMutex
	OperatorMetadata OperatorMetadata
}

type OperatorMetadataPayload struct {
	Hostname  string           `json:"hostname"`
	Timestamp int64            `json:"timestamp"`
	ClusterID string           `json:"cluster_id"`
	Metadata  OperatorMetadata `json:"datadog_operator_metadata"`
}

type OperatorMetadata struct {
	OperatorVersion               string         `json:"operator_version"`
	KubernetesVersion             string         `json:"kubernetes_version"`
	InstallMethodTool             string         `json:"install_method_tool"`
	InstallMethodToolVersion      string         `json:"install_method_tool_version"`
	IsLeader                      bool           `json:"is_leader"`
	DatadogAgentEnabled           bool           `json:"datadogagent_enabled"`
	DatadogMonitorEnabled         bool           `json:"datadogmonitor_enabled"`
	DatadogDashboardEnabled       bool           `json:"datadogdashboard_enabled"`
	DatadogSLOEnabled             bool           `json:"datadogslo_enabled"`
	DatadogGenericResourceEnabled bool           `json:"datadoggenericresource_enabled"`
	DatadogAgentProfileEnabled    bool           `json:"datadogagentprofile_enabled"`
	DatadogAgentInternalEnabled   bool           `json:"datadogagentinternal_enabled"`
	LeaderElectionEnabled         bool           `json:"leader_election_enabled"`
	ExtendedDaemonSetEnabled      bool           `json:"extendeddaemonset_enabled"`
	RemoteConfigEnabled           bool           `json:"remote_config_enabled"`
	IntrospectionEnabled          bool           `json:"introspection_enabled"`
	ClusterID                     string         `json:"cluster_id"`
	ConfigDDURL                   string         `json:"config_dd_url"`
	ConfigDDSite                  string         `json:"config_site"`
	ResourceCounts                map[string]int `json:"resource_count"`
}

// NewOperatorMetadataForwarder creates a new instance of the operator metadata forwarder
func NewOperatorMetadataForwarder(logger logr.Logger, k8sClient client.Reader, kubernetesVersion, operatorVersion string, credsManager *config.CredentialManager) *OperatorMetadataForwarder {
	forwarderLogger := logger.WithName("operator")
	return &OperatorMetadataForwarder{
		SharedMetadata:   NewSharedMetadata(forwarderLogger, k8sClient, kubernetesVersion, operatorVersion, credsManager),
		OperatorMetadata: OperatorMetadata{},
	}
}

// Start starts the operator metadata forwarder
func (omf *OperatorMetadataForwarder) Start() {
	if omf.hostName == "" {
		omf.logger.Error(ErrEmptyHostName, "Could not set host name; not starting metadata forwarder")
		return
	}
	omf.updateResourceCounts()

	omf.logger.Info("Starting metadata forwarder")

	ticker := time.NewTicker(defaultInterval)
	go func() {
		for range ticker.C {
			if err := omf.sendMetadata(); err != nil {
				omf.logger.V(1).Info("Error while sending metadata", "error", err)
			}
		}
	}()

	countsTicker := time.NewTicker(resourceCountsTTL)
	go func() {
		for range countsTicker.C {
			omf.updateResourceCounts()
		}
	}()
}

func (omf *OperatorMetadataForwarder) sendMetadata() error {
	clusterUID, err := omf.GetOrCreateClusterUID()
	if err != nil {
		return fmt.Errorf("error getting cluster UID: %w", err)
	}
	payload := omf.GetPayload(clusterUID)
	req, err := omf.createRequest(payload)
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}
	resp, err := omf.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("error sending metadata request: %w", err)
	}

	defer resp.Body.Close()

	omf.logger.V(1).Info("Sent metadata", "status code", resp.StatusCode)
	return nil
}

func (omf *OperatorMetadataForwarder) GetPayload(clusterUID string) []byte {
	now := time.Now().Unix()

	omf.mutex.RLock()
	// Copy metadata while holding the lock to avoid data races
	operatorMetadata := omf.OperatorMetadata
	if omf.OperatorMetadata.ResourceCounts != nil {
		operatorMetadata.ResourceCounts = make(map[string]int, len(omf.OperatorMetadata.ResourceCounts))
		maps.Copy(operatorMetadata.ResourceCounts, omf.OperatorMetadata.ResourceCounts)
	}
	omf.mutex.RUnlock()

	operatorMetadata.ClusterID = clusterUID
	operatorMetadata.OperatorVersion = omf.operatorVersion
	operatorMetadata.KubernetesVersion = omf.kubernetesVersion

	payload := OperatorMetadataPayload{
		Hostname:  omf.hostName,
		Timestamp: now,
		ClusterID: clusterUID,
		Metadata:  operatorMetadata,
	}

	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		omf.logger.V(1).Info("Error marshaling payload to json", "error", err)
	}

	return jsonPayload
}

// updateResourceCounts refreshes resource counts and stores them in OperatorMetadata.ResourceCounts
// falls back to the old value if the fetch fails
func (omf *OperatorMetadataForwarder) updateResourceCounts() {
	// If k8sClient is nil (e.g., in tests), return early
	if omf.k8sClient == nil {
		return
	}

	omf.mutex.Lock()
	defer omf.mutex.Unlock()

	// Only list resources that are enabled
	// For each resource type: if fetch succeeds, update count; if fails, keep old value
	if omf.OperatorMetadata.DatadogAgentEnabled {
		ddaList := &v2alpha1.DatadogAgentList{}
		if err := omf.k8sClient.List(context.TODO(), ddaList); err == nil {
			omf.OperatorMetadata.ResourceCounts["datadogagent"] = len(ddaList.Items)
		} else {
			omf.logger.V(1).Info("Failed to list DatadogAgents, keeping old value", "error", err, "old_count", omf.OperatorMetadata.ResourceCounts["datadogagent"])
		}
	}

	if omf.OperatorMetadata.DatadogAgentInternalEnabled {
		ddaiList := &v1alpha1.DatadogAgentInternalList{}
		if err := omf.k8sClient.List(context.TODO(), ddaiList); err == nil {
			omf.OperatorMetadata.ResourceCounts["datadogagentinternal"] = len(ddaiList.Items)
		} else {
			omf.logger.V(1).Info("Failed to list DatadogAgentInternals, keeping old value", "error", err, "old_count", omf.OperatorMetadata.ResourceCounts["datadogagentinternal"])
		}
	}

	if omf.OperatorMetadata.DatadogMonitorEnabled {
		monitorList := &v1alpha1.DatadogMonitorList{}
		if err := omf.k8sClient.List(context.TODO(), monitorList); err == nil {
			omf.OperatorMetadata.ResourceCounts["datadogmonitor"] = len(monitorList.Items)
		} else {
			omf.logger.V(1).Info("Failed to list DatadogMonitors, keeping old value", "error", err, "old_count", omf.OperatorMetadata.ResourceCounts["datadogmonitor"])
		}
	}

	if omf.OperatorMetadata.DatadogDashboardEnabled {
		dashboardList := &v1alpha1.DatadogDashboardList{}
		if err := omf.k8sClient.List(context.TODO(), dashboardList); err == nil {
			omf.OperatorMetadata.ResourceCounts["datadogdashboard"] = len(dashboardList.Items)
		} else {
			omf.logger.V(1).Info("Failed to list DatadogDashboards, keeping old value", "error", err, "old_count", omf.OperatorMetadata.ResourceCounts["datadogdashboard"])
		}
	}

	if omf.OperatorMetadata.DatadogSLOEnabled {
		sloList := &v1alpha1.DatadogSLOList{}
		if err := omf.k8sClient.List(context.TODO(), sloList); err == nil {
			omf.OperatorMetadata.ResourceCounts["datadogslo"] = len(sloList.Items)
		} else {
			omf.logger.V(1).Info("Failed to list DatadogSLOs, keeping old value", "error", err, "old_count", omf.OperatorMetadata.ResourceCounts["datadogslo"])
		}
	}

	if omf.OperatorMetadata.DatadogGenericResourceEnabled {
		genericList := &v1alpha1.DatadogGenericResourceList{}
		if err := omf.k8sClient.List(context.TODO(), genericList); err == nil {
			omf.OperatorMetadata.ResourceCounts["datadoggenericresource"] = len(genericList.Items)
		} else {
			omf.logger.V(1).Info("Failed to list DatadogGenericResources, keeping old value", "error", err, "old_count", omf.OperatorMetadata.ResourceCounts["datadoggenericresource"])
		}
	}

	if omf.OperatorMetadata.DatadogAgentProfileEnabled {
		profileList := &v1alpha1.DatadogAgentProfileList{}
		if err := omf.k8sClient.List(context.TODO(), profileList); err == nil {
			omf.OperatorMetadata.ResourceCounts["datadogagentprofile"] = len(profileList.Items)
		} else {
			omf.logger.V(1).Info("Failed to list DatadogAgentProfiles, keeping old value", "error", err, "old_count", omf.OperatorMetadata.ResourceCounts["datadogagentprofile"])
		}
	}
	omf.logger.V(1).Info("Updated resource counts", "counts", omf.OperatorMetadata.ResourceCounts)
}
