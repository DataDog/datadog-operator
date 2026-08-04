// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package untaint defines the agent-not-ready startup taint shared by the untaint
// controller and node Agent DaemonSet scheduling.
package untaint

import corev1 "k8s.io/api/core/v1"

const (
	// AgentNotReadyTaintKey is the key for the startup taint applied to nodes
	// until the Datadog Agent is ready (or removed by timeout policy).
	AgentNotReadyTaintKey = "agent.datadoghq.com/not-ready"
	// AgentNotReadyTaintValue is the value paired with AgentNotReadyTaintKey.
	AgentNotReadyTaintValue = "presence"
)

// AgentNotReadyTaint returns the full taint definition.
func AgentNotReadyTaint() corev1.Taint {
	return corev1.Taint{
		Key:    AgentNotReadyTaintKey,
		Value:  AgentNotReadyTaintValue,
		Effect: corev1.TaintEffectNoSchedule,
	}
}

// AgentNotReadyEqualToleration returns a toleration that matches AgentNotReadyTaint.
func AgentNotReadyEqualToleration() corev1.Toleration {
	return corev1.Toleration{
		Key:      AgentNotReadyTaintKey,
		Operator: corev1.TolerationOpEqual,
		Value:    AgentNotReadyTaintValue,
		Effect:   corev1.TaintEffectNoSchedule,
	}
}

// IsAgentNotReadyTaint reports whether t is the agent-not-ready startup taint.
func IsAgentNotReadyTaint(t corev1.Taint) bool {
	return t.Key == AgentNotReadyTaintKey && t.Value == AgentNotReadyTaintValue && t.Effect == corev1.TaintEffectNoSchedule
}
