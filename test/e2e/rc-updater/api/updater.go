// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type ConfigurationRequest struct {
	Data ConfigurationData `json:"data"`
}

type ConfigurationData struct {
	Type       string             `json:"type"`
	Attributes ConfigurationAttrs `json:"attributes"`
}

type ConfigurationAttrs struct {
	Name       string              `json:"name"`
	Scope      string              `json:"scope"`
	Parameters ConfigurationParams `json:"parameters"`
	Enabled    bool                `json:"enabled"`
}

type ConfigurationParams struct {
	CloudWorkloadSecurity             bool `json:"csm_cloud_workload_security"`
	CloudSecurityPostureManagement    bool `json:"csm_cloud_security_posture_management"`
	HostsVulnerabilityManagement      bool `json:"csm_hosts_vulnerability_management"`
	ContainersVulnerabilityManagement bool `json:"csm_containers_vulnerability_management"`
	UniversalServiceMonitoring        bool `json:"universal_service_monitoring"`
}

type ConfigurationResponse struct {
	Data ResponseData `json:"data"`
}

type ResponseData struct {
	ID         string        `json:"id"`
	Type       string        `json:"type"`
	Attributes ResponseAttrs `json:"attributes"`
}

type ResponseAttrs struct {
	Creator    string          `json:"creator"`
	Updater    string          `json:"updater"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
	Name       string          `json:"name"`
	Parameters ResponseParams  `json:"parameters"`
	Scope      string          `json:"scope"`
	Status     string          `json:"status"`
	Content    ResponseContent `json:"content"`
}

type ResponseParams struct {
	CloudWorkloadSecurity             bool `json:"csm_cloud_workload_security"`
	CloudSecurityPostureManagement    bool `json:"csm_cloud_security_posture_management"`
	HostsVulnerabilityManagement      bool `json:"csm_hosts_vulnerability_management"`
	ContainersVulnerabilityManagement bool `json:"csm_containers_vulnerability_management"`
	UniversalServiceMonitoring        bool `json:"universal_service_monitoring"`
}

type ResponseContent struct {
	Config        ConfigContent        `json:"config"`
	SecurityAgent SecurityAgentContent `json:"security_agent"`
	SystemProbe   SystemProbeContent   `json:"system_probe"`
}

type ConfigContent struct {
	SBOM SBOMContent `json:"sbom"`
}

type SBOMContent struct {
	Enabled        bool                  `json:"enabled"`
	Host           HostContent           `json:"host"`
	ContainerImage ContainerImageContent `json:"container_image"`
}

type HostContent struct {
	Enabled bool `json:"enabled"`
}

type ContainerImageContent struct {
	Enabled bool `json:"enabled"`
}

type SecurityAgentContent struct {
	RuntimeSecurityConfig RuntimeSecurityConfig `json:"runtime_security_config"`
	ComplianceConfig      ComplianceConfig      `json:"compliance_config"`
}

type RuntimeSecurityConfig struct {
	Enabled bool `json:"enabled"`
}

type ComplianceConfig struct {
	Enabled bool `json:"enabled"`
}

type SystemProbeContent struct {
	RuntimeSecurityConfig   RuntimeSecurityConfig   `json:"runtime_security_config"`
	ServiceMonitoringConfig ServiceMonitoringConfig `json:"service_monitoring_config"`
}

type ServiceMonitoringConfig struct {
	Enabled bool `json:"enabled"`
}

func (c *Client) ApplyConfig(configRequest ConfigurationRequest) (*ConfigurationResponse, error) {
	url := "https://app.datadoghq.com/api/unstable/remote_config/products/configurator/configurations"
	reqBody, err := json.Marshal(configRequest)
	if err != nil {
		return nil, fmt.Errorf("error marshalling JSON: %v", err)
	}

	// Create a new HTTP request
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Add("accept", "application/json")
	req.Header.Add("content-type", "application/json")
	req.Header.Add("origin", "https://app.datadoghq.com")
	req.Header.Add("DD-API-KEY", c.apiKey)
	req.Header.Add("DD-APPLICATION-KEY", c.appKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("failed to create config, status code: %d, response: %s", resp.StatusCode, string(respBody))
	}

	var configResp ConfigurationResponse
	err = json.Unmarshal(respBody, &configResp)
	if err != nil {
		return nil, err
	}

	return &configResp, nil
}

func (c *Client) DeleteConfig(configID string) error {
	url := fmt.Sprintf("https://app.datadoghq.com/api/unstable/remote_config/products/configurator/configurations/%s", configID)

	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("error creating request: %v", err)
	}

	req.Header.Set("DD-API-KEY", c.apiKey)
	req.Header.Set("DD-APPLICATION-KEY", c.appKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete configuration, status code: %d", resp.StatusCode)
	}

	return nil
}
