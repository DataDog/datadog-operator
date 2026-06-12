// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package agent

import (
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-operator/pkg/untaint"
)

// podToleratesAgentNotReadyStartup reports whether tolerations tolerate the
// agent-not-ready startup taint, using the same rules as the scheduler/kubelet
// (corev1.Toleration.ToleratesTaint). Comparison-operator tolerations (Lt/Gt) are
// ignored unless the cluster enables that feature (we pass false).
func podToleratesAgentNotReadyStartup(logger logr.Logger, tolerations []corev1.Toleration) bool {
	taint := untaint.AgentNotReadyTaint()
	for i := range tolerations {
		if tolerations[i].ToleratesTaint(logger, &taint, false) {
			return true
		}
	}
	return false
}

// EnsureAgentNotReadyStartupToleration appends the agent-not-ready Equal toleration
// when not already tolerated per Kubernetes toleration matching.
func EnsureAgentNotReadyStartupToleration(logger logr.Logger, spec *corev1.PodSpec) {
	if podToleratesAgentNotReadyStartup(logger, spec.Tolerations) {
		return
	}
	spec.Tolerations = append(spec.Tolerations, untaint.AgentNotReadyEqualToleration())
}
