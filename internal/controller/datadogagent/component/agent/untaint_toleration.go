// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/pkg/untaint"
)

// podToleratesAgentNotReadyStartup returns true if the pod already tolerates the
// agent-not-ready NoSchedule taint (exact Equal match or broader Exists on the key).
func podToleratesAgentNotReadyStartup(tolerations []corev1.Toleration) bool {
	want := untaint.AgentNotReadyEqualToleration()
	for _, t := range tolerations {
		op := t.Operator
		if op == "" {
			op = corev1.TolerationOpEqual
		}
		switch op {
		case corev1.TolerationOpEqual:
			if t.Key == want.Key && t.Value == want.Value &&
				(t.Effect == want.Effect || t.Effect == "") {
				return true
			}
		case corev1.TolerationOpExists:
			if t.Key == want.Key && (t.Effect == want.Effect || t.Effect == "") {
				return true
			}
		}
	}
	return false
}

// EnsureAgentNotReadyStartupToleration appends the agent-not-ready Equal toleration
// when not already present or covered by an equivalent Exists toleration.
func EnsureAgentNotReadyStartupToleration(spec *corev1.PodSpec) {
	if spec == nil {
		return
	}
	if podToleratesAgentNotReadyStartup(spec.Tolerations) {
		return
	}
	spec.Tolerations = append(spec.Tolerations, untaint.AgentNotReadyEqualToleration())
}
