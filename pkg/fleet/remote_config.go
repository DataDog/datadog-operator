// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// installerConfig is the fleet installer configuration received via RC.
type installerConfig struct {
	ID             string                         `json:"id"`
	FileOperations []installerConfigFileOperation `json:"file_operations"`
	Operations     []fleetManagementOperation     `json:"operations"`
}

// installerConfigFileOperation is a single file operation in an installerConfig.
type installerConfigFileOperation struct {
	FileOperationType string          `json:"file_op"`
	FilePath          string          `json:"file_path"`
	Patch             json.RawMessage `json:"patch"`
}

// remoteAPIRequest is a task sent to the fleet daemon via RC.
type remoteAPIRequest struct {
	ID            string          `json:"id"`
	Package       string          `json:"package_name"`
	TraceID       string          `json:"trace_id"`
	ParentSpanID  string          `json:"parent_span_id"`
	ExpectedState expectedState   `json:"expected_state"`
	Method        string          `json:"method"`
	Params        json.RawMessage `json:"params"`
}

// expectedState describes the package state expected before executing the request.
type expectedState struct {
	InstallerVersion string `json:"installer_version"`
	Stable           string `json:"stable"`
	Experiment       string `json:"experiment"`
	StableConfig     string `json:"stable_config"`
	ExperimentConfig string `json:"experiment_config"`
	ClientID         string `json:"client_id"`
}

// handleInstallerConfigUpdate returns an RC subscription callback that parses
// UPDATER_AGENT payloads and forwards them as a map[configID]installerConfig to h.
func handleInstallerConfigUpdate(h func(map[string]installerConfig) error) func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
	return func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
		configs := make(map[string]installerConfig, len(updates))
		for cfgPath, raw := range updates {
			var cfg installerConfig
			if err := json.Unmarshal(raw.Config, &cfg); err != nil {
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateError, Error: fmt.Sprintf("failed to unmarshal installer config: %v", err)})
				return
			}
			configs[cfgPath] = cfg
		}

		if err := h(configs); err != nil {
			for cfgPath := range updates {
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
			}
			return
		}

		for cfgPath := range updates {
			applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}

// handleUpdaterTaskUpdate returns an RC subscription callback that parses a single
// UPDATER_TASK payload and forwards it as a remoteAPIRequest to h.
// Requests that have already been executed (tracked by seen IDs) are ignored.
func handleUpdaterTaskUpdate(h func(remoteAPIRequest) error) func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
	seen := make(map[string]struct{})
	return func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
		for cfgPath, raw := range updates {
			var req remoteAPIRequest
			if err := json.Unmarshal(raw.Config, &req); err != nil {
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateError, Error: fmt.Sprintf("failed to unmarshal remote API request: %v", err)})
				continue
			}

			if _, ok := seen[req.ID]; ok {
				// Already executed; acknowledge without re-running.
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
				continue
			}

			if err := h(req); err != nil {
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				continue
			}

			seen[req.ID] = struct{}{}
			applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}
