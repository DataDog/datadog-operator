// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package remoteconfig

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/go-logr/logr"
	"github.com/stretchr/testify/assert"
)

func TestFlareState(t *testing.T) {
	r := NewRemoteConfigUpdater(nil, logr.Discard())
	st := r.FlareState()
	assert.False(t, st.Started)
	assert.Contains(t, st.Products, string(state.ProductAgentTask))

	r.setRemoteLogLevel("debug")
	r.taskStatuses = map[string]state.ApplyStatus{
		"b": {State: state.ApplyStateError, Error: "boom"},
		"a": {State: state.ApplyStateAcknowledged},
	}
	st = r.FlareState()
	assert.Equal(t, "debug", st.RemoteLogLevel)
	assert.Equal(t, []FlareTaskState{{ID: "a", State: "acknowledged"}, {ID: "b", State: "error", Error: "boom"}}, st.AgentTasks)
}
