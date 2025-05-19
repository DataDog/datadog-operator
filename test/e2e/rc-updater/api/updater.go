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
	"strings"
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

type ErrorDetail struct {
	Title string `json:"title"`
}

type ErrorResponse struct {
	Errors []ErrorDetail `json:"errors"`
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

func extractIDFromError(title string) string {
	// Assuming the ID is always after the last ": " in the title
	parts := strings.Split(title, ": ")
	if len(parts) > 1 {
		return parts[len(parts)-1] // Return the part after the last ": "
	}
	return ""
}

func (c *Client) ApplyConfig(configRequest ConfigurationRequest) (*ConfigurationResponse, error) {
	var resp *http.Response
	var err error

	// Create
	resp, err = c.makeRequest(configRequest, "POST", "")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusBadRequest {
		return nil, fmt.Errorf("failed to create config, status code: %d, response: %s", resp.StatusCode, string(respBody))
	}

	if resp.StatusCode == http.StatusBadRequest {
		var errorResponse ErrorResponse
		jsonErr := json.Unmarshal(respBody, &errorResponse)
		if jsonErr != nil {
			fmt.Println("Error unmarshalling JSON:", jsonErr) // nolint: forbidigo
			return nil, jsonErr
		}
		if strings.HasPrefix(errorResponse.Errors[0].Title, "configuration already exists") {
			// Extract the ID from the error message
			existingID := extractIDFromError(errorResponse.Errors[0].Title)
			fmt.Println("using the policy that already exists:", existingID) // nolint: forbidigo
			resp, err = c.makeRequest(configRequest, "PATCH", existingID)
			if err != nil {
				return nil, err
			}
			defer resp.Body.Close()

			respBody, err = io.ReadAll(resp.Body)
			if err != nil {
				return nil, fmt.Errorf("error reading response: %w", err)
			}
		} else {
			return nil, fmt.Errorf("failed to create config, status code: %d, response: %s", resp.StatusCode, string(respBody))
		}
	}

	var configResp ConfigurationResponse
	err = json.Unmarshal(respBody, &configResp)
	if err != nil {
		return nil, err
	}

	return &configResp, nil

}

func (c *Client) makeRequest(configRequest ConfigurationRequest, method string, existingPolicyID string) (*http.Response, error) {
	var err error
	var req *http.Request
	url := "https://app.datadoghq.com/api/unstable/remote_config/products/configurator/configurations"
	reqBody, err := json.Marshal(configRequest)
	if err != nil {
		return nil, fmt.Errorf("error marshalling JSON: %w", err)
	}

	// create new config
	if method == "POST" {
		req, err = http.NewRequest(method, url, bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
	} else {
		url = fmt.Sprintf("%s/%s", url, existingPolicyID)
		req, err = http.NewRequest(method, url, bytes.NewBuffer(reqBody))
		if err != nil {
			return nil, fmt.Errorf("error creating request: %w", err)
		}
	}

	req.Header.Add("Accept", "application/json")
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Origin", "https://app.datadoghq.com")
	req.Header.Add("Dd-Api-Key", c.apiKey)
	req.Header.Add("Dd-Application-Key", c.appKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error sending request: %w", err)
	}
	return resp, nil
}

func (c *Client) DeleteConfig(configID string) error {
	url := fmt.Sprintf("https://app.datadoghq.com/api/unstable/remote_config/products/configurator/configurations/%s", configID)

	req, err := http.NewRequest("DELETE", url, nil) // nolint: noctx
	if err != nil {
		return fmt.Errorf("error creating request: %w", err)
	}

	req.Header.Set("Dd-Api-Key", c.apiKey)
	req.Header.Set("Dd-Application-Key", c.appKey)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("error sending request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("failed to delete configuration, status code: %d", resp.StatusCode)
	}

	return nil
}
