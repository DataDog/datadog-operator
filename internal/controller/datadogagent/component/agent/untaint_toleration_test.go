// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
)

func TestEnsureAgentNotReadyStartupToleration_nilSpec(t *testing.T) {
	EnsureAgentNotReadyStartupToleration(nil)
}

func TestEnsureAgentNotReadyStartupToleration_idempotent(t *testing.T) {
	spec := &corev1.PodSpec{}
	want := agentNotReadyStartupToleration()

	EnsureAgentNotReadyStartupToleration(spec)
	require.Len(t, spec.Tolerations, 1)
	require.Equal(t, want, spec.Tolerations[0])

	EnsureAgentNotReadyStartupToleration(spec)
	require.Len(t, spec.Tolerations, 1)
}

func TestEnsureAgentNotReadyStartupToleration_skipIfUserEqual(t *testing.T) {
	want := agentNotReadyStartupToleration()
	spec := &corev1.PodSpec{Tolerations: []corev1.Toleration{want}}

	EnsureAgentNotReadyStartupToleration(spec)
	require.Len(t, spec.Tolerations, 1)
}

func TestEnsureAgentNotReadyStartupToleration_skipIfUserExists(t *testing.T) {
	spec := &corev1.PodSpec{
		Tolerations: []corev1.Toleration{
			{Key: agentNotReadyTaintKey, Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
		},
	}

	EnsureAgentNotReadyStartupToleration(spec)
	require.Len(t, spec.Tolerations, 1)
}

func TestEnsureAgentNotReadyStartupToleration_equalEmptyOperator(t *testing.T) {
	spec := &corev1.PodSpec{
		Tolerations: []corev1.Toleration{
			{Key: agentNotReadyTaintKey, Operator: "", Value: agentNotReadyTaintValue, Effect: corev1.TaintEffectNoSchedule},
		},
	}

	EnsureAgentNotReadyStartupToleration(spec)
	require.Len(t, spec.Tolerations, 1)
}
