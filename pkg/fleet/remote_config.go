// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// installerConfig is the fleet installer configuration received via RC.
type installerConfig struct {
	ID         string                     `json:"id"`
	Operations []fleetManagementOperation `json:"operations"`
}

// Operation is the type of fleet management operation to perform on a Kubernetes resource.
type Operation string

const (
	OperationCreate Operation = "create"
	OperationUpdate Operation = "update"
	OperationDelete Operation = "delete"
)

// fleetManagementOperation is a single fleet operation for config management of a Kubernetes resource.
// Config is a JSON merge patch (no strategic merge patch).
type fleetManagementOperation struct {
	Operation        Operation               `json:"operation"`
	GroupVersionKind schema.GroupVersionKind `json:"group_version_kind"`
	Config           json.RawMessage         `json:"config"`
}

// remoteAPIRequest is a task sent to the fleet daemon via RC.
type remoteAPIRequest struct {
	ID            string           `json:"id"`
	Package       string           `json:"package_name"`
	TraceID       string           `json:"trace_id"`
	ParentSpanID  string           `json:"parent_span_id"`
	ExpectedState expectedState    `json:"expected_state"`
	Method        string           `json:"method"`
	Params        experimentParams `json:"params"`
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

// experimentParams holds the parsed params for experiment methods.
type experimentParams struct {
	Version        string               `json:"version"`
	NamespacedName types.NamespacedName `json:"namespaced_name"`
}

// handleInstallerConfigUpdate returns an RC subscription callback that parses
// UPDATER_AGENT payloads and forwards them as a map[configID]installerConfig to h.
func handleInstallerConfigUpdate(ctx context.Context, h func(map[string]installerConfig) error) func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
	return func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
		logger := ctrl.LoggerFrom(ctx)
		configs := make(map[string]installerConfig, len(updates))
		for cfgPath, raw := range updates {
			logger.V(1).Info("Received INSTALLER_CONFIG payload", "cfgPath", cfgPath, "rawPayload", string(raw.Config))

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
func handleUpdaterTaskUpdate(ctx context.Context, h func(remoteAPIRequest) error) func(map[string]state.RawConfig, func(string, state.ApplyStatus)) {
	seen := make(map[string]struct{})
	return func(updates map[string]state.RawConfig, applyStatus func(string, state.ApplyStatus)) {
		logger := ctrl.LoggerFrom(ctx)
		for cfgPath, raw := range updates {
			logger.V(1).Info("Received UPDATER_TASK payload", "cfgPath", cfgPath, "rawPayload", string(raw.Config))

			var req remoteAPIRequest
			if err := json.Unmarshal(raw.Config, &req); err != nil {
				logger.Error(err, "Failed to unmarshal remote API request")
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateError, Error: fmt.Sprintf("failed to unmarshal remote API request: %v", err)})
				continue
			}

			if _, ok := seen[req.ID]; ok {
				// Already executed; acknowledge without re-running.
				logger.Info("Remote API request already executed", "id", req.ID)
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
				continue
			}

			// Signal received and parsed; notify the backend before applying.
			applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateUnacknowledged})

			if err := h(req); err != nil {
				logger.Error(err, "Failed to handle remote API request")
				applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateError, Error: err.Error()})
				continue
			}

			seen[req.ID] = struct{}{}
			applyStatus(cfgPath, state.ApplyStatus{State: state.ApplyStateAcknowledged})
		}
	}
}
