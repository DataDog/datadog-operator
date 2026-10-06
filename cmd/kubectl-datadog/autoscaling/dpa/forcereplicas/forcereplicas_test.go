// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package forcereplicas

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// The --replicas checks run before any connection to the cluster.
func TestSetValidatesReplicas(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr string
	}{
		{name: "missing", args: []string{"set", "web-dpa"}, wantErr: `required flag(s) "replicas" not set`},
		{name: "zero", args: []string{"set", "--replicas", "0", "web-dpa"}, wantErr: "--replicas must be at least 1"},
		{name: "negative", args: []string{"set", "--replicas", "-2", "web-dpa"}, wantErr: "--replicas must be at least 1"},
		{name: "not an integer", args: []string{"set", "--replicas", "abc", "web-dpa"}, wantErr: "invalid argument"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := New(genericclioptions.NewTestIOStreamsDiscard())
			cmd.SetArgs(tc.args)
			cmd.SetOut(genericclioptions.NewTestIOStreamsDiscard().Out)
			cmd.SetErr(genericclioptions.NewTestIOStreamsDiscard().ErrOut)
			assert.ErrorContains(t, cmd.Execute(), tc.wantErr)
		})
	}
}
