// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var validGenericManagedAgentInstallationIdentity = NewGenericManagedAgentInstallationIdentity(
	"123e4567-e89b-42d3-a456-426614174000",
	validManagedAgentInstallationTargetHash,
)

func TestGenericManagedAgentInstallationIdentityFromEnvironment(t *testing.T) {
	clearManagedAgentInstallationIdentityEnvironment(t)

	identity, err := ManagedAgentInstallationIdentityFromEnvironment()
	require.NoError(t, err)
	assert.False(t, identity.Configured())

	t.Setenv(genericManagedAgentInstallationKeyHashEnv, validManagedAgentInstallationTargetHash)
	_, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.Error(t, err)
	require.NoError(t, os.Unsetenv(genericManagedAgentInstallationKeyHashEnv))

	t.Setenv(genericManagedAgentInstallationIDEnv, validGenericManagedAgentInstallationIdentity.InstallationID())
	_, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.Error(t, err)

	t.Setenv(genericManagedAgentInstallationKeyHashEnv, validManagedAgentInstallationTargetHash)
	identity, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.NoError(t, err)
	assert.True(t, identity.Configured())
	assert.Equal(t, ManagedAgentInstallationProviderGeneric, identity.Provider())
	assert.Equal(t, validGenericManagedAgentInstallationIdentity.InstallationID(), identity.InstallationID())
	assert.Equal(t, validGenericManagedAgentInstallationIdentity.TargetID(), identity.TargetID())
}

func TestGenericAndEKSManagedAgentInstallationIdentitiesConflict(t *testing.T) {
	clearManagedAgentInstallationIdentityEnvironment(t)

	t.Setenv(genericManagedAgentInstallationIDEnv, validGenericManagedAgentInstallationIdentity.InstallationID())
	t.Setenv(genericManagedAgentInstallationKeyHashEnv, validManagedAgentInstallationTargetHash)
	t.Setenv(eksManagedAgentInstallationIDEnv, validManagedAgentInstallationIdentity.InstallationID())
	t.Setenv(eksManagedAgentInstallationARNHashEnv, validManagedAgentInstallationTargetHash)

	_, err := ManagedAgentInstallationIdentityFromEnvironment()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "multiple managed Agent installation providers are configured")
}

func TestGenericManagedAgentInstallationIdentityValidate(t *testing.T) {
	require.NoError(t, validGenericManagedAgentInstallationIdentity.Validate())

	invalid := NewGenericManagedAgentInstallationIdentity("not-a-uuid", validManagedAgentInstallationTargetHash)
	err := invalid.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "must be a canonical non-zero UUID")

	invalid = NewGenericManagedAgentInstallationIdentity("123e4567-e89b-42d3-a456-426614174000", "not-a-hash")
	err = invalid.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "lowercase SHA-256 digest")
}

func TestGenericManagedAgentInstallationIdentityUpdaterTags(t *testing.T) {
	tags, err := validGenericManagedAgentInstallationIdentity.UpdaterTags()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"managed_installation_id:123e4567-e89b-42d3-a456-426614174000",
		"resource_key_sha256:" + validManagedAgentInstallationTargetHash,
		"managed_agent_installation:generic-v1",
	}, tags)
}

func TestGenericManagedAgentInstallationIdentityTargetIDMatchesEKSDerivation(t *testing.T) {
	// The neutral identity must derive TargetID exactly like the EKS provider
	// from the same digest, so Fleet Admin can select on either tag set.
	assert.Equal(t,
		validGenericManagedAgentInstallationIdentity.TargetID(),
		validManagedAgentInstallationIdentity.TargetID(),
	)
}
