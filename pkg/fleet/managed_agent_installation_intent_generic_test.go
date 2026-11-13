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
	return marshalGenericIntent(t, intent)
}

// genericIntentJSONWith mutates the valid generic intent: each override
// replaces the base value, and a nil value removes the key.
func genericIntentJSONWith(t *testing.T, overrides map[string]any) []byte {
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
	for key, value := range overrides {
		if value == nil {
			delete(intent, key)
			continue
		}
		intent[key] = value
	}
	return marshalGenericIntent(t, intent)
}

func marshalGenericIntent(t *testing.T, intent map[string]any) []byte {
	t.Helper()
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

func TestDecodeGenericManagedAgentInstallationIntentAbsent(t *testing.T) {
	raw := genericIntentJSONWith(t, map[string]any{
		"desiredState":            "absent",
		"acknowledgedOperationID": testAddonUninstallOperationID,
	})
	intent, config, _, err := decodeManagedAgentInstallationIntent(
		raw, validGenericManagedAgentInstallationIdentity,
	)
	require.NoError(t, err)
	assert.Equal(t, managedAgentInstallationDesiredStateAbsent, intent.DesiredState)
	assert.Equal(t, json.RawMessage(`{}`), config)
}

func TestDecodeGenericManagedAgentInstallationIntentErrors(t *testing.T) {
	identity := validGenericManagedAgentInstallationIdentity
	tests := []struct {
		name    string
		raw     func(t *testing.T) []byte
		wantErr string
	}{
		{
			name:    "missing intent",
			raw:     func(t *testing.T) []byte { return []byte{} },
			wantErr: "is missing",
		},
		{
			name: "oversized intent",
			raw: func(t *testing.T) []byte {
				return append([]byte{'a'}, make([]byte, managedAgentInstallationMaxIntentSize)...)
			},
			wantErr: "exceeds",
		},
		{
			name: "trailing JSON content",
			raw: func(t *testing.T) []byte {
				return append(genericInstallIntentJSON(t), []byte(" {}")...)
			},
			wantErr: "trailing JSON content",
		},
		{
			name: "unsupported version",
			raw: func(t *testing.T) []byte {
				return genericIntentJSONWith(t, map[string]any{"version": "v2"})
			},
			wantErr: "unsupported generic managed Agent installation version",
		},
		{
			name: "invalid operation ID",
			raw: func(t *testing.T) []byte {
				return genericIntentJSONWith(t, map[string]any{"operationID": "not-a-uuid"})
			},
			wantErr: "operation_id must be a canonical non-zero UUID",
		},
		{
			name: "invalid acknowledged operation ID",
			raw: func(t *testing.T) []byte {
				return genericIntentJSONWith(t, map[string]any{
					"acknowledgedOperationID": "not-a-uuid",
				})
			},
			wantErr: "acknowledged_operation_id must be a canonical non-zero UUID",
		},
		{
			name: "acknowledgement does not match the install operation",
			raw: func(t *testing.T) []byte {
				return genericIntentJSONWith(t, map[string]any{
					"acknowledgedOperationID": testAddonUninstallOperationID,
				})
			},
			wantErr: "acknowledgement must match the active install operation",
		},
		{
			name: "invalid bootstrap cluster name",
			raw: func(t *testing.T) []byte {
				return genericIntentJSONWith(t, map[string]any{
					"bootstrap": map[string]any{"clusterName": "", "site": "datadoghq.com"},
				})
			},
			wantErr: "bootstrap cluster name is invalid",
		},
		{
			name: "invalid bootstrap site",
			raw: func(t *testing.T) []byte {
				return genericIntentJSONWith(t, map[string]any{
					"bootstrap": map[string]any{"clusterName": "test-cluster", "site": "not a site"},
				})
			},
			wantErr: "bootstrap site is invalid",
		},
		{
			name: "unsupported desired state",
			raw: func(t *testing.T) []byte {
				return genericIntentJSONWith(t, map[string]any{"desiredState": "pending"})
			},
			wantErr: "unsupported generic managed Agent installation desired state",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, _, _, err := decodeManagedAgentInstallationIntent(test.raw(t), identity)
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
}
