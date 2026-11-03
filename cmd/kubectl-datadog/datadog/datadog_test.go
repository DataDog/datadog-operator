// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package datadog

import (
	"testing"

	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/DataDog/datadog-operator/pkg/version"
)

func TestVersionFlag(t *testing.T) {
	originalVersion, originalCommit, originalBuildTime := version.Version, version.Commit, version.BuildTime
	t.Cleanup(func() {
		version.Version, version.Commit, version.BuildTime = originalVersion, originalCommit, originalBuildTime
	})
	version.Version = "1.32.0-rc.1"
	version.Commit = "0123456789abcdef0123456789abcdef01234567"
	version.BuildTime = "2026-11-03T08:00:00Z"

	streams, _, out, errOut := genericclioptions.NewTestIOStreams()
	cmd := NewCmd(streams)
	cmd.SetArgs([]string{"--version"})

	require.NoError(t, cmd.Execute())
	require.Equal(t, "Version: 1.32.0-rc.1\nGit Commit: 0123456789abcdef0123456789abcdef01234567\nBuild time: 2026-11-03T08:00:00Z\n", out.String())
	require.Empty(t, errOut.String())
}
