// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

type genericManagedAgentInstallationIntent struct {
	Version                 string                               `json:"version"`
	InstallationID          string                               `json:"installationID"`
	ResourceKeySHA256       string                               `json:"resourceKeySHA256"`
	OperationID             string                               `json:"operationID"`
	DesiredState            managedAgentInstallationDesiredState `json:"desiredState"`
	AcknowledgedOperationID string                               `json:"acknowledgedOperationID,omitempty"`
	Bootstrap               managedAgentInstallationBootstrap    `json:"bootstrap"`
	AgentConfig             json.RawMessage                      `json:"agentConfig,omitempty"`
	AgentProfiles           json.RawMessage                      `json:"agentProfiles,omitempty"`
}

func decodeGenericManagedAgentInstallationIntent(raw []byte, identity ManagedAgentInstallationIdentity) (managedAgentInstallationIntent, json.RawMessage, string, error) {
	if len(raw) == 0 {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("generic managed Agent installation intent is missing %q", managedAgentInstallationIntentDataKey)
	}
	if len(raw) > managedAgentInstallationMaxIntentSize {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("generic managed Agent installation intent exceeds %d bytes", managedAgentInstallationMaxIntentSize)
	}

	var genericIntent genericManagedAgentInstallationIntent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&genericIntent); err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("decode generic managed Agent installation intent: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("decode generic managed Agent installation intent: trailing JSON content")
	}
	if genericIntent.Version != managedAgentInstallationVersion {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("unsupported generic managed Agent installation version %q", genericIntent.Version)
	}
	intentIdentity := NewGenericManagedAgentInstallationIdentity(genericIntent.InstallationID, genericIntent.ResourceKeySHA256)
	if err := intentIdentity.Validate(); err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("invalid generic managed Agent installation identity: %w", err)
	}
	if intentIdentity.InstallationID() != identity.InstallationID() {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("generic managed Agent installation ID does not match the local installation")
	}
	targetID := intentIdentity.TargetID()
	if targetID != identity.TargetID() {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("generic managed Agent installation resource key hash does not match the local installation")
	}
	if err := validateCanonicalUUID("operation_id", genericIntent.OperationID); err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}
	if genericIntent.AcknowledgedOperationID != "" {
		if err := validateCanonicalUUID("acknowledged_operation_id", genericIntent.AcknowledgedOperationID); err != nil {
			return managedAgentInstallationIntent{}, nil, "", err
		}
		if genericIntent.DesiredState == managedAgentInstallationDesiredStateInstalled && genericIntent.AcknowledgedOperationID != genericIntent.OperationID {
			return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("generic managed Agent installation acknowledgement must match the active install operation")
		}
	}
	if err := validateManagedAgentInstallationBootstrap(genericIntent.Bootstrap); err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}

	return finalizeManagedAgentInstallationIntent(
		ManagedAgentInstallationProviderGeneric,
		genericIntent.Version,
		genericIntent.InstallationID,
		targetID,
		genericIntent.OperationID,
		genericIntent.AcknowledgedOperationID,
		genericIntent.DesiredState,
		genericIntent.Bootstrap,
		genericIntent.AgentConfig,
		genericIntent.AgentProfiles,
	)
}
