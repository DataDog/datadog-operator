// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package pause

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestNew(t *testing.T) {
	cmd := New(genericclioptions.NewTestIOStreamsDiscard())

	enable, _, err := cmd.Find([]string{"enable"})
	require.NoError(t, err)
	disable, _, err := cmd.Find([]string{"disable"})
	require.NoError(t, err)
	assert.NotNil(t, enable.Flags().Lookup("wait"))
	assert.NotNil(t, disable.Flags().Lookup("wait"), "disable waits for Active to be True again")
}

// The selection is checked before any connection to the cluster.
func TestRequiresSelection(t *testing.T) {
	for _, action := range []string{"enable", "disable"} {
		t.Run(action, func(t *testing.T) {
			streams := genericclioptions.NewTestIOStreamsDiscard()
			cmd := New(streams)
			cmd.SetArgs([]string{action})
			cmd.SetOut(streams.Out)
			cmd.SetErr(streams.ErrOut)
			assert.ErrorContains(t, cmd.Execute(), "select DatadogPodAutoscalers")
		})
	}
}
