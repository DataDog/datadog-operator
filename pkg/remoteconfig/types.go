// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"github.com/go-logr/logr"
	kubeclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/config/remote/client"
	"github.com/DataDog/datadog-agent/pkg/config/remote/service"
)

// RemoteConfigUpdater TODO
type RemoteConfigUpdater struct {
	kubeClient kubeclient.Client // this does NOT cause the issue
	// rcClient *client.Client // this causes the issue
	// rcService *service.Service // this causes the issue
	serviceConf RcServiceConfiguration // this does NOT cause the issue
	logger      logr.Logger            // this does NOT cause the issue
}

// RcServiceConfiguration TODO
type RcServiceConfiguration struct {
	cfg         model.Config // this does NOT cause the issue
	apiKey      string
	baseRawURL  string
	hostname    string
	clusterName string
	// telemetryReporter service.RcTelemetryReporter // this causes the issue
	agentVersion  string
	rcDatabaseDir string
}

// DatadogAgentRemoteConfig contains the struct used to update DatadogAgent object from RemoteConfig
type DatadogAgentRemoteConfig struct {
	ID            string                       `json:"id,omitempty"`
	Name          string                       `json:"name,omitempty"`
	CoreAgent     *CoreAgentFeaturesConfig     `json:"config,omitempty"`
	SystemProbe   *SystemProbeFeaturesConfig   `json:"system_probe,omitempty"`
	SecurityAgent *SecurityAgentFeaturesConfig `json:"security_agent,omitempty"`
}

// CoreAgentFeaturesConfig TODO
type CoreAgentFeaturesConfig struct {
	SBOM *SbomConfig `json:"sbom"`
}

// SystemProbeFeaturesConfig TODO
type SystemProbeFeaturesConfig struct {
	CWS *FeatureEnabledConfig `json:"runtime_security_config"`
	USM *FeatureEnabledConfig `json:"service_monitoring_config"`
}

// SecurityAgentFeaturesConfig TODO
type SecurityAgentFeaturesConfig struct {
	CSPM *FeatureEnabledConfig `json:"compliance_config"`
}

// SbomConfig TODO
type SbomConfig struct {
	Enabled        *bool                 `json:"enabled"`
	Host           *FeatureEnabledConfig `json:"host"`
	ContainerImage *FeatureEnabledConfig `json:"container_image"`
}

// FeatureEnabledConfig TODO
type FeatureEnabledConfig struct {
	Enabled *bool `json:"enabled"`
}
