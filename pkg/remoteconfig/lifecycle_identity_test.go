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

var validLifecycleIdentity = LifecycleIdentity{
	InstallationID: "123e4567-e89b-42d3-a456-426614174000",
	TargetHash:     "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
}

func TestLifecycleIdentityFromEnvironment(t *testing.T) {
	clearLifecycleIdentityEnvironment(t)

	identity, err := LifecycleIdentityFromEnvironment()
	require.NoError(t, err)
	assert.False(t, identity.Configured())

	t.Setenv(lifecycleEKSARNHashEnv, validLifecycleIdentity.TargetHash)
	_, err = LifecycleIdentityFromEnvironment()
	require.Error(t, err)
	require.NoError(t, os.Unsetenv(lifecycleEKSARNHashEnv))

	t.Setenv(lifecycleInstallationIDEnv, validLifecycleIdentity.InstallationID)
	_, err = LifecycleIdentityFromEnvironment()
	require.Error(t, err)

	t.Setenv(lifecycleEKSARNHashEnv, validLifecycleIdentity.TargetHash)
	identity, err = LifecycleIdentityFromEnvironment()
	require.NoError(t, err)
	assert.Equal(t, validLifecycleIdentity, identity)
}

func TestLifecycleIdentityValidation(t *testing.T) {
	tests := []LifecycleIdentity{
		{InstallationID: "123E4567-E89B-42D3-A456-426614174000"},
		{InstallationID: "00000000-0000-0000-0000-000000000000"},
		{InstallationID: " " + validLifecycleIdentity.InstallationID},
	}

	require.NoError(t, validLifecycleIdentity.Validate())
	for _, identity := range tests {
		require.Error(t, identity.Validate())
	}
}

func TestLifecycleIdentityUpdaterTags(t *testing.T) {
	assert.Nil(t, (LifecycleIdentity{}).UpdaterTags())
	assert.Equal(t, []string{
		"eks_installation_id:" + validLifecycleIdentity.InstallationID,
		"eks_arn_sha256:" + validLifecycleIdentity.TargetHash,
		lifecycleCapabilityTag,
	}, validLifecycleIdentity.UpdaterTags())
}

func TestLifecycleIdentityTargetID(t *testing.T) {
	assert.Equal(t, "aerukz4jvpg66ajdivtytk6n54asgrlhrgv433ybencwpcnlzxxq", validLifecycleIdentity.TargetID())
	assert.Len(t, validLifecycleIdentity.TargetID(), 52)
	assert.Empty(t, (LifecycleIdentity{TargetHash: "invalid"}).TargetID())
}

func clearLifecycleIdentityEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{lifecycleInstallationIDEnv, lifecycleEKSARNHashEnv} {
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
