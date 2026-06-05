// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package untaint

import (
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
)

func TestIsAgentNotReadyTaint(t *testing.T) {
	assert.True(t, IsAgentNotReadyTaint(AgentNotReadyTaint()))
	assert.False(t, IsAgentNotReadyTaint(corev1.Taint{Key: AgentNotReadyTaintKey, Value: "other", Effect: corev1.TaintEffectNoSchedule}))
	assert.False(t, IsAgentNotReadyTaint(corev1.Taint{Key: "other", Value: AgentNotReadyTaintValue, Effect: corev1.TaintEffectNoSchedule}))
}

func TestAgentNotReadyEqualToleration_matchesTaint(t *testing.T) {
	tol := AgentNotReadyEqualToleration()
	taint := AgentNotReadyTaint()
	assert.Equal(t, taint.Key, tol.Key)
	assert.Equal(t, taint.Value, tol.Value)
	assert.Equal(t, taint.Effect, tol.Effect)
	assert.Equal(t, corev1.TolerationOpEqual, tol.Operator)
}
