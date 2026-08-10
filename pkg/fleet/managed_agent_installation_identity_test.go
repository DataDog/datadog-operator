// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package fleet

import (
	"errors"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const validManagedAgentInstallationTargetHash = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

var validManagedAgentInstallationIdentity = NewEKSManagedAgentInstallationIdentity(
	"123e4567-e89b-42d3-a456-426614174000",
	validManagedAgentInstallationTargetHash,
)

type testManagedAgentInstallationProviderIdentity struct {
	validationErr error
}

func (testManagedAgentInstallationProviderIdentity) Provider() ManagedAgentInstallationProvider {
	return "test"
}

func (testManagedAgentInstallationProviderIdentity) InstallationID() string {
	return "test-installation"
}

func (testManagedAgentInstallationProviderIdentity) TargetID() string {
	return "test-target"
}

func (i testManagedAgentInstallationProviderIdentity) Validate() error {
	return i.validationErr
}

func (testManagedAgentInstallationProviderIdentity) UpdaterTags() []string {
	return []string{"test-provider:enabled"}
}

func TestManagedAgentInstallationIdentityFromEnvironment(t *testing.T) {
	clearManagedAgentInstallationIdentityEnvironment(t)

	identity, err := ManagedAgentInstallationIdentityFromEnvironment()
	require.NoError(t, err)
	assert.False(t, identity.Configured())

	t.Setenv(eksManagedAgentInstallationARNHashEnv, validManagedAgentInstallationTargetHash)
	_, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.Error(t, err)
	require.NoError(t, os.Unsetenv(eksManagedAgentInstallationARNHashEnv))

	t.Setenv(eksManagedAgentInstallationIDEnv, validManagedAgentInstallationIdentity.InstallationID())
	_, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.Error(t, err)

	t.Setenv(eksManagedAgentInstallationARNHashEnv, validManagedAgentInstallationTargetHash)
	identity, err = ManagedAgentInstallationIdentityFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, validManagedAgentInstallationIdentity, identity)
}

func TestManagedAgentInstallationIdentityValidation(t *testing.T) {
	tests := []ManagedAgentInstallationIdentity{
		NewEKSManagedAgentInstallationIdentity("123E4567-E89B-42D3-A456-426614174000", validManagedAgentInstallationTargetHash),
		NewEKSManagedAgentInstallationIdentity("00000000-0000-0000-0000-000000000000", validManagedAgentInstallationTargetHash),
		NewEKSManagedAgentInstallationIdentity(" "+validManagedAgentInstallationIdentity.InstallationID(), validManagedAgentInstallationTargetHash),
		NewEKSManagedAgentInstallationIdentity(validManagedAgentInstallationIdentity.InstallationID(), "invalid"),
	}

	require.NoError(t, validManagedAgentInstallationIdentity.Validate())
	assert.Equal(t, ManagedAgentInstallationProviderEKS, validManagedAgentInstallationIdentity.Provider())
	for _, identity := range tests {
		require.Error(t, identity.Validate())
	}
}

func TestManagedAgentInstallationIdentityUpdaterTags(t *testing.T) {
	tags, err := (ManagedAgentInstallationIdentity{}).UpdaterTags()
	require.NoError(t, err)
	assert.Nil(t, tags)
	tags, err = validManagedAgentInstallationIdentity.UpdaterTags()
	require.NoError(t, err)
	assert.Equal(t, []string{
		"eks_installation_id:" + validManagedAgentInstallationIdentity.InstallationID(),
		"eks_arn_sha256:" + validManagedAgentInstallationTargetHash,
		eksManagedAgentInstallationCapabilityTag,
	}, tags)

	invalid := newManagedAgentInstallationIdentity(testManagedAgentInstallationProviderIdentity{validationErr: errors.New("invalid identity")})
	_, err = invalid.UpdaterTags()
	require.ErrorContains(t, err, "invalid identity")
}

func TestManagedAgentInstallationIdentityTargetID(t *testing.T) {
	assert.Equal(t, "aerukz4jvpg66ajdivtytk6n54asgrlhrgv433ybencwpcnlzxxq", validManagedAgentInstallationIdentity.TargetID())
	assert.Len(t, validManagedAgentInstallationIdentity.TargetID(), 52)
	assert.Empty(t, NewEKSManagedAgentInstallationIdentity(validManagedAgentInstallationIdentity.InstallationID(), "invalid").TargetID())
}

func TestManagedAgentInstallationIdentityProviderDelegation(t *testing.T) {
	identity := newManagedAgentInstallationIdentity(testManagedAgentInstallationProviderIdentity{})

	require.True(t, identity.Configured())
	assert.Equal(t, ManagedAgentInstallationProvider("test"), identity.Provider())
	assert.Equal(t, "test-installation", identity.InstallationID())
	assert.Equal(t, "test-target", identity.TargetID())
	require.NoError(t, identity.Validate())
}

func TestUnconfiguredManagedAgentInstallationIdentity(t *testing.T) {
	identity := ManagedAgentInstallationIdentity{}

	assert.False(t, identity.Configured())
	assert.Empty(t, identity.Provider())
	assert.Empty(t, identity.InstallationID())
	assert.Empty(t, identity.TargetID())
	require.NoError(t, identity.Validate())
}

func TestManagedAgentInstallationIdentityFromLoaders(t *testing.T) {
	identity := newManagedAgentInstallationIdentity(testManagedAgentInstallationProviderIdentity{})

	loaded, err := managedAgentInstallationIdentityFromLoaders([]managedAgentInstallationIdentityLoader{
		func() (ManagedAgentInstallationIdentity, error) { return ManagedAgentInstallationIdentity{}, nil },
		func() (ManagedAgentInstallationIdentity, error) { return identity, nil },
	})
	require.NoError(t, err)
	assert.Equal(t, identity, loaded)

	_, err = managedAgentInstallationIdentityFromLoaders([]managedAgentInstallationIdentityLoader{
		func() (ManagedAgentInstallationIdentity, error) { return identity, nil },
		func() (ManagedAgentInstallationIdentity, error) { return validManagedAgentInstallationIdentity, nil },
	})
	require.ErrorContains(t, err, "multiple managed Agent installation providers")
}

func clearManagedAgentInstallationIdentityEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{eksManagedAgentInstallationIDEnv, eksManagedAgentInstallationARNHashEnv} {
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
