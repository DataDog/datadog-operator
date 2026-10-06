// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autoscaling

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/cli-runtime/pkg/genericclioptions"
)

func TestNewRegistersSubcommands(t *testing.T) {
	cmd := New(genericclioptions.NewTestIOStreamsDiscard())
	for _, name := range []string{"cluster", "dpa"} {
		sub, _, err := cmd.Find([]string{name})
		if assert.NoError(t, err, name) {
			assert.Equal(t, name, sub.Name())
		}
	}
}
