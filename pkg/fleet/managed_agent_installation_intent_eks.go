// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

type eksManagedAgentInstallationIntent struct {
	Version                 string                                      `json:"version"`
	InstallationID          string                                      `json:"installationID"`
	EKSARNSHA256            string                                      `json:"eksARNSHA256"`
	OperationID             string                                      `json:"operationID"`
	DesiredState            managedAgentInstallationDesiredState        `json:"desiredState"`
	AcknowledgedOperationID string                                      `json:"acknowledgedOperationID,omitempty"`
	Bootstrap               managedAgentInstallationBootstrap           `json:"bootstrap"`
	DatadogAgent            *datadogAgentManagedAgentInstallationConfig `json:"datadogAgent,omitempty"`
}

func decodeEKSManagedAgentInstallationIntent(raw []byte, identity ManagedAgentInstallationIdentity) (managedAgentInstallationIntent, json.RawMessage, string, error) {
	if len(raw) == 0 {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS managed Agent installation intent is missing %q", managedAgentInstallationIntentDataKey)
	}
	if len(raw) > managedAgentInstallationMaxIntentSize {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS managed Agent installation intent exceeds %d bytes", managedAgentInstallationMaxIntentSize)
	}

	var eksIntent eksManagedAgentInstallationIntent
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&eksIntent); err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("decode EKS managed Agent installation intent: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("decode EKS managed Agent installation intent: trailing JSON content")
	}
	switch eksIntent.Version {
	case managedAgentInstallationVersionV1:
		if eksIntent.DatadogAgent != nil {
			return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS managed Agent installation version %q does not support DatadogAgent config", eksIntent.Version)
		}
	case managedAgentInstallationVersionV2:
	default:
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("unsupported EKS managed Agent installation version %q", eksIntent.Version)
	}
	intentIdentity := NewEKSManagedAgentInstallationIdentity(eksIntent.InstallationID, eksIntent.EKSARNSHA256)
	if err := intentIdentity.Validate(); err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("invalid EKS managed Agent installation identity: %w", err)
	}
	if intentIdentity.InstallationID() != identity.InstallationID() {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS managed Agent installation ID does not match the local installation")
	}
	targetID := intentIdentity.TargetID()
	if targetID != identity.TargetID() {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS managed Agent installation ARN hash does not match the local installation")
	}
	if err := validateCanonicalUUID("operation_id", eksIntent.OperationID); err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}
	if eksIntent.AcknowledgedOperationID != "" {
		if err := validateCanonicalUUID("acknowledged_operation_id", eksIntent.AcknowledgedOperationID); err != nil {
			return managedAgentInstallationIntent{}, nil, "", err
		}
		if eksIntent.DesiredState == managedAgentInstallationDesiredStateInstalled && eksIntent.AcknowledgedOperationID != eksIntent.OperationID {
			return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS managed Agent installation acknowledgement must match the active install operation")
		}
	}
	if err := validateManagedAgentInstallationBootstrap(eksIntent.Bootstrap); err != nil {
		return managedAgentInstallationIntent{}, nil, "", err
	}

	var normalizedConfig json.RawMessage
	switch eksIntent.DesiredState {
	case managedAgentInstallationDesiredStateInstalled:
		config, configErr := managedAgentInstallationConfig(eksIntent.Bootstrap, eksIntent.DatadogAgent)
		if configErr != nil {
			return managedAgentInstallationIntent{}, nil, "", configErr
		}
		normalizedConfig = config
	case managedAgentInstallationDesiredStateAbsent:
		if eksIntent.DatadogAgent != nil {
			return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("EKS managed Agent uninstall intent must not contain DatadogAgent config")
		}
		normalizedConfig = json.RawMessage(`{}`)
	default:
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("unsupported EKS managed Agent installation desired state %q", eksIntent.DesiredState)
	}

	intent := managedAgentInstallationIntent{
		Version:                 eksIntent.Version,
		Provider:                ManagedAgentInstallationProviderEKS,
		InstallationID:          eksIntent.InstallationID,
		TargetID:                targetID,
		OperationID:             eksIntent.OperationID,
		DesiredState:            eksIntent.DesiredState,
		AcknowledgedOperationID: eksIntent.AcknowledgedOperationID,
		Bootstrap:               eksIntent.Bootstrap,
	}
	normalized := normalizedManagedAgentInstallationIntent{
		Version:        intent.Version,
		Provider:       intent.Provider,
		InstallationID: intent.InstallationID,
		TargetID:       intent.TargetID,
		OperationID:    intent.OperationID,
		DesiredState:   intent.DesiredState,
		Bootstrap:      intent.Bootstrap,
	}
	if intent.Version == managedAgentInstallationVersionV2 {
		normalized.DatadogAgent = eksIntent.DatadogAgent
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return managedAgentInstallationIntent{}, nil, "", fmt.Errorf("encode normalized EKS managed Agent installation intent: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return intent, normalizedConfig, hex.EncodeToString(digest[:]), nil
}
