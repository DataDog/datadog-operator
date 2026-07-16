// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validManagedAgentInstallationIdentity = ManagedAgentInstallationIdentity{
	InstallationID: "123e4567-e89b-42d3-a456-426614174000",
	TargetHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
}

func TestManagedAgentInstallationIdentityFromEnvironment(t *testing.T) {
	clearManagedAgentInstallationIdentityEnvironment(t)

	identity, err := ManagedAgentInstallationIdentityFromEnvironment()
	require.NoError(t, err)
	assert.False(t, identity.Configured())

	t.Setenv(managedAgentInstallationEKSARNHashEnv, validManagedAgentInstallationIdentity.TargetHash)
	_, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.Error(t, err)
	require.NoError(t, os.Unsetenv(managedAgentInstallationEKSARNHashEnv))

	t.Setenv(managedAgentInstallationIDEnv, validManagedAgentInstallationIdentity.InstallationID)
	_, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.Error(t, err)

	t.Setenv(managedAgentInstallationEKSARNHashEnv, validManagedAgentInstallationIdentity.TargetHash)
	identity, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, validManagedAgentInstallationIdentity, identity)
}

func TestManagedAgentInstallationIdentityValidation(t *testing.T) {
	tests := []ManagedAgentInstallationIdentity{
		{InstallationID: "123E4567-E89B-42D3-A456-426614174000"},
		{InstallationID: "00000000-0000-0000-0000-000000000000"},
		{InstallationID: " " + validManagedAgentInstallationIdentity.InstallationID},
	}

	require.NoError(t, validManagedAgentInstallationIdentity.Validate())
	for _, identity := range tests {
		require.Error(t, identity.Validate())
	}
}

func TestManagedAgentInstallationIdentityUpdaterTags(t *testing.T) {
	assert.Nil(t, (ManagedAgentInstallationIdentity{}).UpdaterTags())
	assert.Equal(t, []string{
		"eks_installation_id:" + validManagedAgentInstallationIdentity.InstallationID,
		"eks_arn_sha256:" + validManagedAgentInstallationIdentity.TargetHash,
		managedAgentInstallationCapabilityTag,
	}, validManagedAgentInstallationIdentity.UpdaterTags())
}

func TestManagedAgentInstallationIdentityTargetID(t *testing.T) {
	assert.Equal(t, "aerukz4jvpg66ajdivtytk6n54asgrlhrgv433ybencwpcnlzxxq", validManagedAgentInstallationIdentity.TargetID())
	assert.Len(t, validManagedAgentInstallationIdentity.TargetID(), 52)
	assert.Empty(t, (ManagedAgentInstallationIdentity{TargetHash: "invalid"}).TargetID())
}

func clearManagedAgentInstallationIdentityEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{managedAgentInstallationIDEnv, managedAgentInstallationEKSARNHashEnv} {
		value, exists := os.LookupEnv(name)
		require.NoError(t, os.Unsetenv(name))
		t.Cleanup(func() {
			if exists {
				require.NoError(t, os.Setenv(name, value))
				return
			}
			require.NoError(t, os.Unsetenv(name))
		})
	}
}
