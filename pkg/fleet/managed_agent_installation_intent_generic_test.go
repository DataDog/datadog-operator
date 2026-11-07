// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func genericInstallIntentJSON(t *testing.T) []byte {
	t.Helper()
	intent := map[string]any{
		"version":           "v1",
		"installationID":    validGenericManagedAgentInstallationIdentity.InstallationID(),
		"resourceKeySHA256": validManagedAgentInstallationTargetHash,
		"operationID":       testAddonInstallOperationID,
		"desiredState":      "installed",
		"bootstrap": map[string]any{
			"clusterName": "test-cluster",
			"site":        "datadoghq.com",
		},
	}
	encoded, err := json.Marshal(intent)
	require.NoError(t, err)
	return encoded
}

func TestDecodeGenericManagedAgentInstallationIntent(t *testing.T) {
	intent, config, digest, err := decodeManagedAgentInstallationIntent(
		genericInstallIntentJSON(t), validGenericManagedAgentInstallationIdentity,
	)
	require.NoError(t, err)
	assert.Equal(t, ManagedAgentInstallationProviderGeneric, intent.Provider)
	assert.Equal(t, validGenericManagedAgentInstallationIdentity.InstallationID(), intent.InstallationID)
	assert.Equal(t, validGenericManagedAgentInstallationIdentity.TargetID(), intent.TargetID)
	assert.Equal(t, managedAgentInstallationDesiredStateInstalled, intent.DesiredState)
	assert.NotEmpty(t, digest)
	assert.NotEmpty(t, config)
}

func TestDecodeGenericManagedAgentInstallationIntentRejectsEKSField(t *testing.T) {
	// The generic decoder must not accept the EKS-named ARN hash field:
	// DisallowUnknownFields keeps the wire formats distinct.
	eksShaped := map[string]any{
		"version":        "v1",
		"installationID": validGenericManagedAgentInstallationIdentity.InstallationID(),
		"eksARNSHA256":   validManagedAgentInstallationTargetHash,
		"operationID":    testAddonInstallOperationID,
		"desiredState":   "installed",
		"bootstrap": map[string]any{
			"clusterName": "test-cluster",
			"site":        "datadoghq.com",
		},
	}
	encoded, err := json.Marshal(eksShaped)
	require.NoError(t, err)

	_, _, _, err = decodeManagedAgentInstallationIntent(encoded, validGenericManagedAgentInstallationIdentity)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestDecodeGenericManagedAgentInstallationIntentIdentityMismatch(t *testing.T) {
	other := NewGenericManagedAgentInstallationIdentity(
		"123e4567-e89b-42d3-a456-426614174999",
		validManagedAgentInstallationTargetHash,
	)
	_, _, _, err := decodeManagedAgentInstallationIntent(genericInstallIntentJSON(t), other)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "does not match the local installation")
}

func TestDecodeGenericManagedAgentInstallationIntentNotForEKSIdentity(t *testing.T) {
	// An operator configured with the EKS identity must refuse generic intents:
	// it decodes through the EKS decoder, which rejects the neutral field.
	_, _, _, err := decodeManagedAgentInstallationIntent(
		genericInstallIntentJSON(t), validManagedAgentInstallationIdentity,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown field "resourceKeySHA256"`)
}
