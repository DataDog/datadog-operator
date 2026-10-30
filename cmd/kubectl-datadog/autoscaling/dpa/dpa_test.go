// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package dpa

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestCommandTree(t *testing.T) {
	root := New(genericclioptions.NewTestIOStreamsDiscard())

	find := func(args ...string) *cobra.Command {
		t.Helper()
		cmd, _, err := root.Find(args)
		require.NoError(t, err)
		require.Equal(t, args[len(args)-1], cmd.Name())
		return cmd
	}

	mutating := map[string][]string{
		"pause enable":           {"pause", "enable"},
		"pause disable":          {"pause", "disable"},
		"force-fallback enable":  {"force-fallback", "enable"},
		"force-fallback disable": {"force-fallback", "disable"},
		"force-replicas set":     {"force-replicas", "set"},
		"force-replicas unset":   {"force-replicas", "unset"},
		"force-resources set":    {"force-resources", "set"},
		"force-resources unset":  {"force-resources", "unset"},
	}
	for name, path := range mutating {
		cmd := find(path...)
		// Same names as evict-legacy-nodes.
		for _, flag := range []string{"all", "all-namespaces", "label-selector", "yes", "dry-run", "namespace", "context"} {
			assert.NotNil(t, cmd.Flags().Lookup(flag), "%s --%s", name, flag)
		}
	}

	// Commands whose annotation is reported in the status wait for it.
	for _, path := range [][]string{{"pause", "enable"}, {"pause", "disable"}, {"force-fallback", "enable"}, {"force-replicas", "set"}, {"force-resources", "unset"}} {
		assert.NotNil(t, find(path...).Flags().Lookup("wait"), "%v --wait", path)
	}
	assert.Nil(t, find("force-fallback", "disable").Flags().Lookup("wait"))
	assert.Nil(t, find("force-fallback", "disable").Flags().Lookup("timeout"))
	assert.NotNil(t, find("force-resources", "set").Flags().Lookup("container"))
}
